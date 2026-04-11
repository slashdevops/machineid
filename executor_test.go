package machineid

import (
	"context"
	"fmt"
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
