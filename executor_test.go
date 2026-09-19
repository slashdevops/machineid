package machineid

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// mockExecutor is a test double that implements CommandExecutor for testing.
// It is safe for concurrent use (required by Windows concurrent collection).
//
// Resolution order for each Execute call:
//  1. Args-specific error (setErrorForArgs)
//  2. Args-specific output (setOutputForArgs)
//  3. Command-name error (setError)
//  4. Command-name output (setOutput)
//  5. fallback error
//
// The args-specific map lets tests distinguish calls that share a command
// name but differ in arguments (e.g. `sysctl -n machdep.cpu.brand_string`
// vs `sysctl -n machdep.cpu.features`).
type mockExecutor struct {
	mu sync.RWMutex
	// outputs maps command name to expected output
	outputs map[string]string
	// errors maps command name to expected error
	errors map[string]error
	// outputsByArgs maps "name\x00arg1\x00arg2..." to expected output
	outputsByArgs map[string]string
	// errorsByArgs maps "name\x00arg1\x00arg2..." to expected error
	errorsByArgs map[string]error
	// callCount tracks how many times each command was called
	callCount map[string]int
}

// newMockExecutor creates a new mock executor for testing.
func newMockExecutor() *mockExecutor {
	return &mockExecutor{
		outputs:       make(map[string]string),
		errors:        make(map[string]error),
		outputsByArgs: make(map[string]string),
		errorsByArgs:  make(map[string]error),
		callCount:     make(map[string]int),
	}
}

// calls returns how many times the named command was executed.
func (m *mockExecutor) calls(command string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.callCount[command]
}

// argsKey builds the internal lookup key for an args-specific mock entry.
// Using NUL as separator avoids collisions with args that contain spaces.
func argsKey(name string, args []string) string {
	if len(args) == 0 {
		return name
	}
	return name + "\x00" + strings.Join(args, "\x00")
}

// Execute implements CommandExecutor interface.
func (m *mockExecutor) Execute(ctx context.Context, name string, args ...string) (string, error) {
	m.mu.Lock()
	m.callCount[name]++
	m.mu.Unlock()

	m.mu.RLock()
	defer m.mu.RUnlock()

	key := argsKey(name, args)

	// Args-specific entries take precedence over plain command-name entries.
	if err, exists := m.errorsByArgs[key]; exists {
		return "", err
	}
	if output, exists := m.outputsByArgs[key]; exists {
		return output, nil
	}

	if err, exists := m.errors[name]; exists {
		return "", err
	}

	if output, exists := m.outputs[name]; exists {
		return output, nil
	}

	return "", fmt.Errorf("command %q not configured in mock", name)
}

// setOutput configures the mock to return the given output for a command.
// Matches any invocation of the command regardless of arguments.
func (m *mockExecutor) setOutput(command, output string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.outputs[command] = output
}

// setError configures the mock to return an error for a command.
// Matches any invocation of the command regardless of arguments.
func (m *mockExecutor) setError(command string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors[command] = err
}

// setOutputForArgs configures the mock to return the given output only when
// the command is invoked with the exact args slice. Takes precedence over
// setOutput.
func (m *mockExecutor) setOutputForArgs(command string, args []string, output string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.outputsByArgs[argsKey(command, args)] = output
}

// setErrorForArgs configures the mock to return an error only when the
// command is invoked with the exact args slice. Takes precedence over
// setError.
func (m *mockExecutor) setErrorForArgs(command string, args []string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errorsByArgs[argsKey(command, args)] = err
}

// TestExecuteCancelledContext tests that an already-cancelled context is
// reported as a CommandError wrapping context.Canceled.
func TestExecuteCancelledContext(t *testing.T) {
	executor := &defaultCommandExecutor{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := executor.Execute(ctx, "echo", "test")
	if err == nil {
		t.Fatal("Expected error for cancelled context but got none")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled in chain, got %v", err)
	}
	cmdErr, ok := errors.AsType[*CommandError](err)
	if !ok {
		t.Fatalf("Expected CommandError, got %T", err)
	}
	if cmdErr.Command != "echo" {
		t.Errorf("Expected command 'echo', got %q", cmdErr.Command)
	}
}

// TestExecuteDeadlineExceeded tests that a slow command is killed at the
// timeout, that the error wraps context.DeadlineExceeded, and that WaitDelay
// keeps Output from blocking past the deadline.
func TestExecuteDeadlineExceeded(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX sleep binary")
	}

	executor := &defaultCommandExecutor{Timeout: 50 * time.Millisecond}
	start := time.Now()

	_, err := executor.Execute(context.Background(), "sleep", "5")
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected context.DeadlineExceeded in chain, got %v", err)
	}
	if elapsed > 3*time.Second {
		t.Errorf("Execute took %v; expected the timeout plus WaitDelay to bound it", elapsed)
	}
}

// TestExecuteStderrExcerpt tests that a failing command's stderr is attached
// to the CommandError.
func TestExecuteStderrExcerpt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}

	executor := &defaultCommandExecutor{}
	_, err := executor.Execute(context.Background(), "sh", "-c", "echo boom >&2; exit 3")

	cmdErr, ok := errors.AsType[*CommandError](err)
	if !ok {
		t.Fatalf("Expected CommandError, got %T: %v", err, err)
	}
	if cmdErr.Stderr != "boom" {
		t.Errorf("Expected stderr excerpt 'boom', got %q", cmdErr.Stderr)
	}
	if !strings.Contains(cmdErr.Error(), "boom") {
		t.Errorf("Expected Error() to include stderr, got %q", cmdErr.Error())
	}
}

func TestStderrExcerpt(t *testing.T) {
	long := strings.Repeat("x", maxStderr+10)
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"whitespace only", "  \n\n ", ""},
		{"first non-empty line", "\n  first \nsecond", "first"},
		{"truncated", long, long[:maxStderr] + "..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stderrExcerpt([]byte(tt.in)); got != tt.want {
				t.Errorf("stderrExcerpt(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// --- memoExecutor tests ---

func TestMemoExecutorRunsEachInvocationOnce(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutputForArgs("system_profiler", []string{"SPHardwareDataType", "-json"}, "{}")
	mock.setOutputForArgs("system_profiler", []string{"SPStorageDataType", "-json"}, "[]")
	mock.setError("ioreg", fmt.Errorf("boom"))

	memo := newMemoExecutor(mock, nil)
	ctx := context.Background()

	for range 3 {
		out, err := memo.Execute(ctx, "system_profiler", "SPHardwareDataType", "-json")
		if err != nil || out != "{}" {
			t.Fatalf("Execute = %q, %v", out, err)
		}
	}
	if _, err := memo.Execute(ctx, "system_profiler", "SPStorageDataType", "-json"); err != nil {
		t.Fatal(err)
	}
	// Failures are cached too.
	for range 2 {
		if _, err := memo.Execute(ctx, "ioreg"); err == nil {
			t.Fatal("Expected cached error")
		}
	}

	if got := mock.calls("system_profiler"); got != 2 {
		t.Errorf("Expected 2 distinct system_profiler runs, got %d", got)
	}
	if got := mock.calls("ioreg"); got != 1 {
		t.Errorf("Expected 1 ioreg run, got %d", got)
	}
}

func TestMemoExecutorConcurrentCallersShareOneRun(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("slow", "value")
	memo := newMemoExecutor(mock, nil)

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if out, err := memo.Execute(context.Background(), "slow", "arg"); err != nil || out != "value" {
				t.Errorf("Execute = %q, %v", out, err)
			}
		})
	}
	wg.Wait()

	if got := mock.calls("slow"); got != 1 {
		t.Errorf("Expected exactly 1 run for concurrent callers, got %d", got)
	}
}

func TestMemoExecutorNilInnerUsesDefault(t *testing.T) {
	memo := newMemoExecutor(nil, nil)
	if memo.inner == nil {
		t.Fatal("Expected default executor for nil inner")
	}
}

func TestMemoExecutorLogsReuse(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mock := newMockExecutor()
	mock.setOutput("cmd", "v")
	memo := newMemoExecutor(mock, logger)

	for range 2 {
		if _, err := memo.Execute(context.Background(), "cmd"); err != nil {
			t.Fatal(err)
		}
	}

	if !strings.Contains(buf.String(), "reusing command output") {
		t.Errorf("Expected reuse log, got %q", buf.String())
	}
}

// TestExecuteCommandWithNilExecutor tests executeCommand with nil executor.
func TestExecuteCommandWithNilExecutor(t *testing.T) {
	// This should use the default realExecutor
	_, err := executeCommand(context.Background(), nil, nil, "echo", "test")
	// We expect this to work or fail gracefully
	if err != nil {
		// That's fine, we just want to ensure no panic
		t.Logf("Command execution with nil executor: %v", err)
	}
}

// TestMockExecutorArgsAwareOverride verifies that setOutputForArgs takes
// precedence over setOutput and that different arg slices resolve
// independently — required for tests that exercise multiple sysctl
// subcommands.
func TestMockExecutorArgsAwareOverride(t *testing.T) {
	m := newMockExecutor()
	m.setOutput("sysctl", "generic-output")
	m.setOutputForArgs("sysctl", []string{"-n", "machdep.cpu.brand_string"}, "Apple M1 Pro")
	m.setOutputForArgs("sysctl", []string{"-n", "machdep.cpu.features"}, "")

	ctx := context.Background()

	brand, err := m.Execute(ctx, "sysctl", "-n", "machdep.cpu.brand_string")
	if err != nil {
		t.Fatalf("brand call: %v", err)
	}
	if brand != "Apple M1 Pro" {
		t.Errorf("brand = %q, want %q", brand, "Apple M1 Pro")
	}

	features, err := m.Execute(ctx, "sysctl", "-n", "machdep.cpu.features")
	if err != nil {
		t.Fatalf("features call: %v", err)
	}
	if features != "" {
		t.Errorf("features = %q, want empty", features)
	}

	// An un-overridden invocation should fall through to setOutput.
	generic, err := m.Execute(ctx, "sysctl", "-a")
	if err != nil {
		t.Fatalf("generic call: %v", err)
	}
	if generic != "generic-output" {
		t.Errorf("generic = %q, want %q", generic, "generic-output")
	}
}

// TestMockExecutorArgsAwareError verifies setErrorForArgs takes precedence.
func TestMockExecutorArgsAwareError(t *testing.T) {
	m := newMockExecutor()
	m.setOutput("sysctl", "generic")
	m.setErrorForArgs("sysctl", []string{"-n", "machdep.cpu.features"}, fmt.Errorf("nope"))

	ctx := context.Background()
	if _, err := m.Execute(ctx, "sysctl", "-n", "machdep.cpu.features"); err == nil {
		t.Error("Expected args-specific error")
	}
	out, err := m.Execute(ctx, "sysctl", "-n", "machdep.cpu.brand_string")
	if err != nil {
		t.Fatalf("generic call: %v", err)
	}
	if out != "generic" {
		t.Errorf("generic = %q, want %q", out, "generic")
	}
}
