package machineid

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

// defaultCommandExecutor implements CommandExecutor using actual system command execution.
type defaultCommandExecutor struct {
	Timeout time.Duration
}

// Execute runs a system command with a timeout and returns the output.
// It uses context.WithTimeout to prevent commands from hanging indefinitely.
func (e *defaultCommandExecutor) Execute(ctx context.Context, name string, args ...string) (string, error) {
	timeout := e.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, name, args...)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("command %q failed: %w", name, err)
	}

	return strings.TrimSpace(string(output)), nil
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
