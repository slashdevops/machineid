package machineid

import (
	"context"
	"log/slog"
	"runtime/pprof"
	"sync"
)

// componentTask describes how to collect one hardware component.
// Exactly one of single or multi must be set.
type componentTask struct {
	single    func(ctx context.Context) (string, error)
	multi     func(ctx context.Context) ([]string, error)
	component string
	prefix    string
}

// componentResult holds the outcome of a single component collection.
type componentResult struct {
	err       error
	component string
	prefix    string
	value     string   // for single-value components
	values    []string // for multi-value components (MAC, disk)
	multi     bool     // true if this is a multi-value result
}

// pprofComponentLabel is the runtime/pprof label attached to every collector
// goroutine. Since Go 1.27, goroutine labels appear in tracebacks, so a hung
// system command shows which component it belongs to.
const pprofComponentLabel = "machineid.component"

// runComponentTasks runs every task concurrently and folds the results into
// identifiers in task order. Hardware queries are dominated by process
// start-up cost (system_profiler, wmic, PowerShell), so running them in
// parallel reduces total latency to that of the slowest single command.
//
// Results are written into a slice indexed by task position rather than sent
// over a channel, so identifiers and diag.Collected are deterministic no
// matter which command finishes first, and no goroutine can outlive the call.
func runComponentTasks(ctx context.Context, tasks []componentTask, diag *DiagnosticInfo, logger *slog.Logger) []string {
	if len(tasks) == 0 {
		return nil
	}

	results := make([]componentResult, len(tasks))

	var wg sync.WaitGroup
	for i, task := range tasks {
		wg.Go(func() {
			pprof.Do(ctx, pprof.Labels(pprofComponentLabel, task.component), func(ctx context.Context) {
				results[i] = runComponentTask(ctx, task)
			})
		})
	}
	wg.Wait()

	var identifiers []string
	for _, r := range results {
		if r.multi {
			identifiers = appendMultiResult(identifiers, r, diag, logger)
		} else {
			identifiers = appendSingleResult(identifiers, r, diag, logger)
		}
	}

	return identifiers
}

// runComponentTask executes one task and packages its outcome.
func runComponentTask(ctx context.Context, task componentTask) componentResult {
	r := componentResult{component: task.component, prefix: task.prefix}

	if task.multi != nil {
		r.multi = true
		r.values, r.err = task.multi(ctx)

		return r
	}

	r.value, r.err = task.single(ctx)

	return r
}

// appendSingleResult processes a single-value component result into identifiers.
func appendSingleResult(identifiers []string, r componentResult, diag *DiagnosticInfo, logger *slog.Logger) []string {
	return appendIdentifierIfValid(identifiers, func() (string, error) {
		return r.value, r.err
	}, r.prefix, diag, r.component, logger)
}

// appendMultiResult processes a multi-value component result into identifiers.
func appendMultiResult(identifiers []string, r componentResult, diag *DiagnosticInfo, logger *slog.Logger) []string {
	return appendIdentifiersIfValid(identifiers, func() ([]string, error) {
		return r.values, r.err
	}, r.prefix, diag, r.component, logger)
}
