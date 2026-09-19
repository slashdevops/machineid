package machineid

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"runtime/pprof"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunComponentTasksEmpty(t *testing.T) {
	diag := &DiagnosticInfo{Errors: make(map[string]error)}
	if got := runComponentTasks(context.Background(), nil, diag, nil); got != nil {
		t.Errorf("Expected nil identifiers for no tasks, got %v", got)
	}
}

// TestRunComponentTasksDeterministicOrder verifies that identifiers and
// diag.Collected follow task declaration order even when tasks finish in
// reverse order.
func TestRunComponentTasksDeterministicOrder(t *testing.T) {
	tasks := []componentTask{
		{component: "a", prefix: "a:", single: func(context.Context) (string, error) {
			time.Sleep(30 * time.Millisecond)
			return "1", nil
		}},
		{component: "b", prefix: "b:", multi: func(context.Context) ([]string, error) {
			time.Sleep(10 * time.Millisecond)
			return []string{"2", "3"}, nil
		}},
		{component: "c", prefix: "c:", single: func(context.Context) (string, error) {
			return "4", nil
		}},
	}

	for range 5 {
		diag := &DiagnosticInfo{Errors: make(map[string]error)}
		got := runComponentTasks(context.Background(), tasks, diag, nil)

		want := []string{"a:1", "b:2", "b:3", "c:4"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("identifiers = %v, want %v", got, want)
		}
		if strings.Join(diag.Collected, ",") != "a,b,c" {
			t.Fatalf("Collected = %v, want [a b c]", diag.Collected)
		}
	}
}

func TestRunComponentTasksRecordsErrors(t *testing.T) {
	boom := errors.New("boom")
	tasks := []componentTask{
		{component: "ok", prefix: "ok:", single: func(context.Context) (string, error) { return "v", nil }},
		{component: "fail", prefix: "fail:", single: func(context.Context) (string, error) { return "", boom }},
		{component: "empty", prefix: "empty:", single: func(context.Context) (string, error) { return "", nil }},
		{component: "none", prefix: "none:", multi: func(context.Context) ([]string, error) { return nil, nil }},
	}

	diag := &DiagnosticInfo{Errors: make(map[string]error)}
	got := runComponentTasks(context.Background(), tasks, diag, nil)

	if len(got) != 1 || got[0] != "ok:v" {
		t.Errorf("identifiers = %v, want [ok:v]", got)
	}
	if !errors.Is(diag.Errors["fail"], boom) {
		t.Errorf("Errors[fail] = %v, want wrapped boom", diag.Errors["fail"])
	}
	if !errors.Is(diag.Errors["empty"], ErrEmptyValue) {
		t.Errorf("Errors[empty] = %v, want ErrEmptyValue", diag.Errors["empty"])
	}
	if !errors.Is(diag.Errors["none"], ErrNoValues) {
		t.Errorf("Errors[none] = %v, want ErrNoValues", diag.Errors["none"])
	}
	for _, c := range []string{"fail", "empty", "none"} {
		if _, ok := errors.AsType[*ComponentError](diag.Errors[c]); !ok {
			t.Errorf("Errors[%s] = %T, want *ComponentError", c, diag.Errors[c])
		}
	}
}

// TestRunComponentTasksRunsConcurrently verifies that tasks overlap in time.
func TestRunComponentTasksRunsConcurrently(t *testing.T) {
	const n = 4
	var inFlight, peak atomic.Int32

	var tasks []componentTask
	for i := range n {
		tasks = append(tasks, componentTask{component: fmt.Sprint(i), prefix: "p:",
			single: func(context.Context) (string, error) {
				cur := inFlight.Add(1)
				for {
					old := peak.Load()
					if cur <= old || peak.CompareAndSwap(old, cur) {
						break
					}
				}
				time.Sleep(20 * time.Millisecond)
				inFlight.Add(-1)
				return "x", nil
			}})
	}

	runComponentTasks(context.Background(), tasks, nil, nil)

	if peak.Load() < 2 {
		t.Errorf("peak concurrency = %d, expected tasks to overlap", peak.Load())
	}
}

// TestRunComponentTasksPropagatesContext verifies that the task context is
// derived from the caller's context and carries the pprof component label.
func TestRunComponentTasksPropagatesContext(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "marker")

	var sawValue, sawLabel bool
	tasks := []componentTask{{component: "cpu", prefix: "cpu:",
		single: func(ctx context.Context) (string, error) {
			sawValue = ctx.Value(key{}) == "marker"
			label, ok := pprof.Label(ctx, pprofComponentLabel)
			sawLabel = ok && label == "cpu"
			return "v", nil
		}}}

	runComponentTasks(ctx, tasks, nil, nil)

	if !sawValue {
		t.Error("task did not receive the caller's context")
	}
	if !sawLabel {
		t.Error("task context is missing the pprof component label")
	}
}

// TestRunComponentTasksNoGoroutineLeak uses the Go 1.27 goroutineleak
// profile to assert that no collector goroutine outlives the call, even
// when tasks block until the context is cancelled.
func TestRunComponentTasksNoGoroutineLeak(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	tasks := []componentTask{
		{component: "block", prefix: "b:", single: func(ctx context.Context) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		}},
		{component: "canceller", prefix: "c:", single: func(context.Context) (string, error) {
			cancel()
			return "v", nil
		}},
	}

	diag := &DiagnosticInfo{Errors: make(map[string]error)}
	runComponentTasks(ctx, tasks, diag, nil)

	if !errors.Is(diag.Errors["block"], context.Canceled) {
		t.Errorf("Errors[block] = %v, want context.Canceled", diag.Errors["block"])
	}

	prof := pprof.Lookup("goroutineleak")
	if prof == nil {
		t.Fatal("goroutineleak profile not available")
	}
	var buf bytes.Buffer
	if err := prof.WriteTo(&buf, 1); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if strings.Contains(buf.String(), "runComponentTask") {
		t.Errorf("leaked collector goroutine:\n%s", buf.String())
	}
}
