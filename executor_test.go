package machineid

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// mockExecutor is a test double that implements CommandExecutor for testing.
type mockExecutor struct {
	// outputs maps command name to expected output
	outputs map[string]string
	// errors maps command name to expected error
	errors map[string]error
	// callCount tracks how many times each command was called
	callCount map[string]int
}

// newMockExecutor creates a new mock executor for testing.
func newMockExecutor() *mockExecutor {
	return &mockExecutor{
		outputs:   make(map[string]string),
		errors:    make(map[string]error),
		callCount: make(map[string]int),
	}
}

// Execute implements CommandExecutor interface.
func (m *mockExecutor) Execute(ctx context.Context, name string, args ...string) (string, error) {
	m.callCount[name]++

	if err, exists := m.errors[name]; exists {
		return "", err
	}

	if output, exists := m.outputs[name]; exists {
		return output, nil
	}

	return "", fmt.Errorf("command %q not configured in mock", name)
}

// setOutput configures the mock to return the given output for a command.
func (m *mockExecutor) setOutput(command, output string) {
	m.outputs[command] = output
}

// setError configures the mock to return an error for a command.
func (m *mockExecutor) setError(command string, err error) {
	m.errors[command] = err
}

// TestExecuteTimeout tests that command execution respects timeout.
func TestExecuteTimeout(t *testing.T) {
	executor := &defaultCommandExecutor{}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	time.Sleep(2 * time.Millisecond) // Ensure timeout expires

	_, err := executor.Execute(ctx, "echo", "test")
	if err == nil {
		t.Error("Expected timeout error but got none")
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
