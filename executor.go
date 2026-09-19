package machineid

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// waitDelay bounds how long Execute waits for a command's output pipes to
// close after the command has been killed on context cancellation. Without
// it, a grandchild process that inherited stdout keeps Output blocked
// indefinitely.
const waitDelay = time.Second

// maxStderr caps the stderr excerpt attached to a CommandError.
const maxStderr = 200

// defaultCommandExecutor implements CommandExecutor using actual system command execution.
type defaultCommandExecutor struct {
	Timeout time.Duration
}

// Execute runs a system command with a timeout and returns the output.
// It uses context.WithTimeout to prevent commands from hanging indefinitely.
// When the deadline or the parent context ends the command, the returned
// error wraps [context.DeadlineExceeded] or [context.Canceled] so callers can
// detect it with [errors.Is].
func (e *defaultCommandExecutor) Execute(ctx context.Context, name string, args ...string) (string, error) {
	timeout := e.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, name, args...)
	cmd.WaitDelay = waitDelay

	output, err := cmd.Output()
	if err != nil {
		cmdErr := &CommandError{Command: name, Err: err}
		if ctxErr := timeoutCtx.Err(); ctxErr != nil {
			cmdErr.Err = fmt.Errorf("%w: %w", ctxErr, err)
		}
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			cmdErr.Stderr = stderrExcerpt(exitErr.Stderr)
		}

		return "", cmdErr
	}

	return strings.TrimSpace(string(output)), nil
}

// stderrExcerpt returns the first non-empty line of stderr, truncated to maxStderr bytes.
func stderrExcerpt(stderr []byte) string {
	for line := range strings.SplitSeq(string(stderr), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) > maxStderr {
			return line[:maxStderr] + "..."
		}

		return line
	}

	return ""
}

// executeCommand is a convenience wrapper that calls Execute with the given context.
// This function is used by platform-specific collectors that need the Provider's executor.
func executeCommand(ctx context.Context, executor CommandExecutor, logger *slog.Logger, name string, args ...string) (string, error) {
	if executor == nil {
		executor = &defaultCommandExecutor{
			Timeout: defaultTimeout,
		}
	}

	if logger != nil {
		logger.Debug("executing command", "command", name, "args", args)
	}

	start := time.Now()
	result, err := executor.Execute(ctx, name, args...)
	duration := time.Since(start)

	if logger != nil {
		if err != nil {
			logger.Debug("command failed", "command", name, "duration", duration, "error", err)
		} else {
			logger.Debug("command completed", "command", name, "duration", duration)
		}
	}

	return result, err
}

// memoExecutor caches the result of every distinct command invocation for the
// lifetime of one collection pass. Collectors that share a data source (the
// macOS UUID, serial and CPU collectors all read
// `system_profiler SPHardwareDataType -json`) then spawn that process once.
// Concurrent callers of the same command wait for the single in-flight run.
// Failures are cached too, so a broken command is not retried by every
// collector that falls back through it.
type memoExecutor struct {
	inner   CommandExecutor
	logger  *slog.Logger
	entries map[string]*memoEntry
	mu      sync.Mutex
}

type memoEntry struct {
	err    error
	output string
	once   sync.Once
}

// newMemoExecutor wraps inner with a per-invocation result cache.
// A nil inner executor falls back to the default executor.
func newMemoExecutor(inner CommandExecutor, logger *slog.Logger) *memoExecutor {
	if inner == nil {
		inner = &defaultCommandExecutor{Timeout: defaultTimeout}
	}

	return &memoExecutor{
		inner:   inner,
		logger:  logger,
		entries: make(map[string]*memoEntry),
	}
}

// Execute implements CommandExecutor.
func (m *memoExecutor) Execute(ctx context.Context, name string, args ...string) (string, error) {
	key := name + "\x00" + strings.Join(args, "\x00")

	m.mu.Lock()
	entry, seen := m.entries[key]
	if !seen {
		entry = &memoEntry{}
		m.entries[key] = entry
	}
	m.mu.Unlock()

	if seen && m.logger != nil {
		m.logger.Debug("reusing command output", "command", name, "args", args)
	}

	entry.once.Do(func() {
		entry.output, entry.err = m.inner.Execute(ctx, name, args...)
	})

	return entry.output, entry.err
}
