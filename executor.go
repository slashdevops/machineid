package machineid

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// defaultCommandExecutor implements CommandExecutor using actual system command execution.
type defaultCommandExecutor struct {
	TimeOut time.Duration
}

// Execute runs a system command with a timeout and returns the output.
// It uses context.WithTimeout to prevent commands from hanging indefinitely.
func (e *defaultCommandExecutor) Execute(ctx context.Context, name string, args ...string) (string, error) {
	// Set a timeout of 5 seconds for command execution
	var timeout time.Duration
	if e.TimeOut > 0 {
		timeout = e.TimeOut
	} else {
		timeout = 3 * time.Second
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

// executeCommand is a convenience wrapper that creates a context and calls Execute.
// This function is used by platform-specific collectors that need the Provider's executor.
func executeCommand(executor CommandExecutor, name string, args ...string) (string, error) {
	if executor == nil {
		executor = &defaultCommandExecutor{
			TimeOut: 3 * time.Second,
		}
	}

	return executor.Execute(context.Background(), name, args...)
}
