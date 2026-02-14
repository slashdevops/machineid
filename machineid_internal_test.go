package machineid

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
)

// TestHashIdentifiersEmpty tests hashing with empty identifiers.
func TestHashIdentifiersEmpty(t *testing.T) {
	result := hashIdentifiers([]string{}, "", Format64)
	if len(result) != 64 {
		t.Errorf("Expected 64-character hash, got %d", len(result))
	}
}

// TestHashIdentifiersSorting tests that identifiers are sorted before hashing.
func TestHashIdentifiersSorting(t *testing.T) {
	ids1 := []string{"cpu:intel", "uuid:123"}
	ids2 := []string{"uuid:123", "cpu:intel"}

	hash1 := hashIdentifiers(ids1, "test", Format64)
	hash2 := hashIdentifiers(ids2, "test", Format64)

	if hash1 != hash2 {
		t.Error("Hash should be same regardless of input order")
	}
}

// TestHashIdentifiersWithoutSalt tests hashing without salt.
func TestHashIdentifiersWithoutSalt(t *testing.T) {
	ids := []string{"test1", "test2"}

	withSalt := hashIdentifiers(ids, "mysalt", Format64)
	withoutSalt := hashIdentifiers(ids, "", Format64)

	if withSalt == withoutSalt {
		t.Error("Hash with salt should differ from hash without salt")
	}
}

// TestAppendIdentifierIfValidEmpty tests with empty value.
func TestAppendIdentifierIfValidEmpty(t *testing.T) {
	diag := &DiagnosticInfo{Errors: make(map[string]error)}
	getValue := func() (string, error) {
		return "", nil
	}

	result := appendIdentifierIfValid([]string{"existing"}, getValue, "prefix:", diag, "test", nil)
	if len(result) != 1 {
		t.Errorf("Expected 1 identifier, got %d", len(result))
	}
	diagErr, ok := diag.Errors["test"]
	if !ok {
		t.Fatal("Expected error recorded in diagnostics for empty value")
	}
	if !errors.Is(diagErr, ErrEmptyValue) {
		t.Errorf("Expected ErrEmptyValue in diagnostic, got %v", diagErr)
	}
	var compErr *ComponentError
	if !errors.As(diagErr, &compErr) {
		t.Fatal("Expected ComponentError in diagnostic")
	}
	if compErr.Component != "test" {
		t.Errorf("ComponentError.Component = %q, want %q", compErr.Component, "test")
	}
}

// TestAppendIdentifierIfValidError tests with error.
func TestAppendIdentifierIfValidError(t *testing.T) {
	diag := &DiagnosticInfo{Errors: make(map[string]error)}
	getValue := func() (string, error) {
		return "", fmt.Errorf("test error")
	}

	result := appendIdentifierIfValid([]string{"existing"}, getValue, "prefix:", diag, "test", nil)
	if len(result) != 1 {
		t.Errorf("Expected 1 identifier (original), got %d", len(result))
	}
	diagErr, ok := diag.Errors["test"]
	if !ok {
		t.Fatal("Expected error recorded in diagnostics")
	}
	var compErr *ComponentError
	if !errors.As(diagErr, &compErr) {
		t.Fatal("Expected ComponentError in diagnostic")
	}
	if compErr.Component != "test" {
		t.Errorf("ComponentError.Component = %q, want %q", compErr.Component, "test")
	}
}

// TestAppendIdentifierIfValidSuccess tests with valid value.
func TestAppendIdentifierIfValidSuccess(t *testing.T) {
	diag := &DiagnosticInfo{Errors: make(map[string]error)}
	getValue := func() (string, error) {
		return "good-value", nil
	}

	result := appendIdentifierIfValid([]string{"existing"}, getValue, "prefix:", diag, "test", nil)
	if len(result) != 2 {
		t.Errorf("Expected 2 identifiers, got %d", len(result))
	}
	if len(diag.Collected) != 1 || diag.Collected[0] != "test" {
		t.Errorf("Expected 'test' in collected, got %v", diag.Collected)
	}
}

// TestAppendIdentifiersIfValidEmpty tests with empty array.
func TestAppendIdentifiersIfValidEmpty(t *testing.T) {
	diag := &DiagnosticInfo{Errors: make(map[string]error)}
	getValues := func() ([]string, error) {
		return []string{}, nil
	}

	result := appendIdentifiersIfValid([]string{"existing"}, getValues, "prefix:", diag, "test", nil)
	if len(result) != 1 {
		t.Errorf("Expected 1 identifier, got %d", len(result))
	}
}

// TestAppendIdentifiersIfValidError tests with error.
func TestAppendIdentifiersIfValidError(t *testing.T) {
	diag := &DiagnosticInfo{Errors: make(map[string]error)}
	getValues := func() ([]string, error) {
		return nil, fmt.Errorf("test error")
	}

	result := appendIdentifiersIfValid([]string{"existing"}, getValues, "prefix:", diag, "test", nil)
	if len(result) != 1 {
		t.Errorf("Expected 1 identifier (original), got %d", len(result))
	}
	diagErr, ok := diag.Errors["test"]
	if !ok {
		t.Fatal("Expected error recorded in diagnostics")
	}
	var compErr *ComponentError
	if !errors.As(diagErr, &compErr) {
		t.Fatal("Expected ComponentError in diagnostic")
	}
	if compErr.Component != "test" {
		t.Errorf("ComponentError.Component = %q, want %q", compErr.Component, "test")
	}
}

// TestAppendIdentifiersIfValidMultiple tests with multiple values.
func TestAppendIdentifiersIfValidMultiple(t *testing.T) {
	diag := &DiagnosticInfo{Errors: make(map[string]error)}
	getValues := func() ([]string, error) {
		return []string{"val1", "val2", "val3"}, nil
	}

	result := appendIdentifiersIfValid([]string{"existing"}, getValues, "prefix:", diag, "test", nil)
	if len(result) != 4 {
		t.Errorf("Expected 4 identifiers, got %d", len(result))
	}

	// Check that prefix was added
	if result[1] != "prefix:val1" {
		t.Errorf("Expected 'prefix:val1', got '%s'", result[1])
	}

	// Check diagnostics
	if len(diag.Collected) != 1 || diag.Collected[0] != "test" {
		t.Errorf("Expected 'test' in collected, got %v", diag.Collected)
	}
}

// TestAppendIdentifierNilDiag tests that nil diagnostics don't panic.
func TestAppendIdentifierNilDiag(t *testing.T) {
	getValue := func() (string, error) {
		return "value", nil
	}

	result := appendIdentifierIfValid(nil, getValue, "prefix:", nil, "test", nil)
	if len(result) != 1 {
		t.Errorf("Expected 1 identifier, got %d", len(result))
	}
}

// TestAppendIdentifierIfValidWithLogger tests all logger paths in appendIdentifierIfValid.
func TestAppendIdentifierIfValidWithLogger(t *testing.T) {
	t.Run("error with logger", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		diag := &DiagnosticInfo{Errors: make(map[string]error)}

		result := appendIdentifierIfValid(nil, func() (string, error) {
			return "", fmt.Errorf("test error")
		}, "prefix:", diag, "test-comp", logger)

		if len(result) != 0 {
			t.Errorf("Expected 0 identifiers, got %d", len(result))
		}
		if !bytes.Contains(buf.Bytes(), []byte("component failed")) {
			t.Error("Expected 'component failed' in log output")
		}
	})

	t.Run("empty value with logger", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		diag := &DiagnosticInfo{Errors: make(map[string]error)}

		result := appendIdentifierIfValid(nil, func() (string, error) {
			return "", nil
		}, "prefix:", diag, "test-comp", logger)

		if len(result) != 0 {
			t.Errorf("Expected 0 identifiers, got %d", len(result))
		}
		if !bytes.Contains(buf.Bytes(), []byte("component returned empty value")) {
			t.Error("Expected 'component returned empty value' in log output")
		}
	})

	t.Run("success with logger", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		diag := &DiagnosticInfo{Errors: make(map[string]error)}

		result := appendIdentifierIfValid(nil, func() (string, error) {
			return "good-value", nil
		}, "prefix:", diag, "test-comp", logger)

		if len(result) != 1 {
			t.Errorf("Expected 1 identifier, got %d", len(result))
		}
		if !bytes.Contains(buf.Bytes(), []byte("component collected")) {
			t.Error("Expected 'component collected' in log output")
		}
		if !bytes.Contains(buf.Bytes(), []byte("component value")) {
			t.Error("Expected 'component value' in log output")
		}
	})
}

// TestAppendIdentifiersIfValidWithLogger tests all logger paths in appendIdentifiersIfValid.
func TestAppendIdentifiersIfValidWithLogger(t *testing.T) {
	t.Run("error with logger", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		diag := &DiagnosticInfo{Errors: make(map[string]error)}

		result := appendIdentifiersIfValid(nil, func() ([]string, error) {
			return nil, fmt.Errorf("test error")
		}, "prefix:", diag, "test-comp", logger)

		if len(result) != 0 {
			t.Errorf("Expected 0 identifiers, got %d", len(result))
		}
		if !bytes.Contains(buf.Bytes(), []byte("component failed")) {
			t.Error("Expected 'component failed' in log output")
		}
	})

	t.Run("empty values with logger", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		diag := &DiagnosticInfo{Errors: make(map[string]error)}

		result := appendIdentifiersIfValid(nil, func() ([]string, error) {
			return []string{}, nil
		}, "prefix:", diag, "test-comp", logger)

		if len(result) != 0 {
			t.Errorf("Expected 0 identifiers, got %d", len(result))
		}
		if !bytes.Contains(buf.Bytes(), []byte("component returned no values")) {
			t.Error("Expected 'component returned no values' in log output")
		}
	})

	t.Run("success with logger", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		diag := &DiagnosticInfo{Errors: make(map[string]error)}

		result := appendIdentifiersIfValid(nil, func() ([]string, error) {
			return []string{"val1", "val2"}, nil
		}, "prefix:", diag, "test-comp", logger)

		if len(result) != 2 {
			t.Errorf("Expected 2 identifiers, got %d", len(result))
		}
		if !bytes.Contains(buf.Bytes(), []byte("component collected")) {
			t.Error("Expected 'component collected' in log output")
		}
		if !bytes.Contains(buf.Bytes(), []byte("component values")) {
			t.Error("Expected 'component values' in log output")
		}
	})
}

// TestAppendIdentifiersIfValidEmptyWithDiag tests empty values record ErrNoValues in diagnostics.
func TestAppendIdentifiersIfValidEmptyWithDiag(t *testing.T) {
	diag := &DiagnosticInfo{Errors: make(map[string]error)}
	result := appendIdentifiersIfValid(nil, func() ([]string, error) {
		return []string{}, nil
	}, "prefix:", diag, "test", nil)

	if len(result) != 0 {
		t.Errorf("Expected 0 identifiers, got %d", len(result))
	}
	diagErr, ok := diag.Errors["test"]
	if !ok {
		t.Fatal("Expected error recorded in diagnostics for empty values")
	}
	if !errors.Is(diagErr, ErrNoValues) {
		t.Errorf("Expected ErrNoValues in diagnostic, got %v", diagErr)
	}
	var compErr *ComponentError
	if !errors.As(diagErr, &compErr) {
		t.Fatal("Expected ComponentError in diagnostic")
	}
	if compErr.Component != "test" {
		t.Errorf("ComponentError.Component = %q, want %q", compErr.Component, "test")
	}
}

// TestFormatHashInvalidLength tests formatHash with non-64-char input.
func TestFormatHashInvalidLength(t *testing.T) {
	short := "abc123"
	result := formatHash(short, Format64)
	if result != short {
		t.Errorf("Expected input returned unchanged for invalid length, got %q", result)
	}
}

// TestFormatHashDefaultCase tests formatHash with an unknown FormatMode.
func TestFormatHashDefaultCase(t *testing.T) {
	// Create a valid 64-char hex string
	hash := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	result := formatHash(hash, FormatMode(999))
	if result != hash {
		t.Errorf("Expected input returned unchanged for unknown format mode, got %q", result)
	}
}

// TestFormatHashAllModes tests formatHash produces correct lengths for all modes.
func TestFormatHashAllModes(t *testing.T) {
	hash := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"

	tests := []struct {
		mode       FormatMode
		wantLength int
	}{
		{Format32, 32},
		{Format64, 64},
		{Format128, 128},
		{Format256, 256},
	}

	for _, tt := range tests {
		result := formatHash(hash, tt.mode)
		if len(result) != tt.wantLength {
			t.Errorf("formatHash(mode=%d) length = %d, want %d", tt.mode, len(result), tt.wantLength)
		}
	}
}

// TestLogMethodsNilLogger tests that log methods don't panic with nil logger.
func TestLogMethodsNilLogger(t *testing.T) {
	p := New() // no logger set

	// These should not panic
	p.logDebug("test debug")
	p.logInfo("test info")
	p.logWarn("test warn")
}

// TestLogMethodsWithLogger tests that log methods produce output with logger set.
func TestLogMethodsWithLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	p := New().WithLogger(logger)

	p.logDebug("test debug msg")
	p.logInfo("test info msg")
	p.logWarn("test warn msg")

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("test debug msg")) {
		t.Error("Expected 'test debug msg' in log output")
	}
	if !bytes.Contains([]byte(output), []byte("test info msg")) {
		t.Error("Expected 'test info msg' in log output")
	}
	if !bytes.Contains([]byte(output), []byte("test warn msg")) {
		t.Error("Expected 'test warn msg' in log output")
	}
}

// TestEnabledComponents tests that enabledComponents returns correct names.
func TestEnabledComponents(t *testing.T) {
	p := New().WithCPU().WithSystemUUID().WithDisk()
	components := p.enabledComponents()

	if len(components) != 3 {
		t.Fatalf("Expected 3 components, got %d: %v", len(components), components)
	}

	want := []string{ComponentCPU, ComponentSystemUUID, ComponentDisk}
	for i, c := range components {
		if c != want[i] {
			t.Errorf("Component[%d] = %q, want %q", i, c, want[i])
		}
	}
}

// TestEnabledComponentsAll tests all components enabled.
func TestEnabledComponentsAll(t *testing.T) {
	p := New().WithCPU().WithMotherboard().WithSystemUUID().WithMAC().WithDisk()
	components := p.enabledComponents()

	if len(components) != 5 {
		t.Fatalf("Expected 5 components, got %d: %v", len(components), components)
	}
}

// TestEnabledComponentsNone tests no components enabled.
func TestEnabledComponentsNone(t *testing.T) {
	p := New()
	components := p.enabledComponents()

	if len(components) != 0 {
		t.Errorf("Expected 0 components, got %d: %v", len(components), components)
	}
}

// TestProviderWithLoggerNoIdentifiersWarning tests logWarn path when no identifiers collected.
func TestProviderWithLoggerNoIdentifiersWarning(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	p := New().WithLogger(logger)
	// No components enabled → ErrNoIdentifiers

	_, err := p.ID(context.Background())
	if !errors.Is(err, ErrNoIdentifiers) {
		t.Errorf("Expected ErrNoIdentifiers, got %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte("no hardware identifiers collected")) {
		t.Error("Expected 'no hardware identifiers collected' warning in log output")
	}
}

// TestExecuteCommandWithLogger tests executeCommand logger output paths.
func TestExecuteCommandWithLogger(t *testing.T) {
	t.Run("success with logger", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

		mock := newMockExecutor()
		mock.setOutput("testcmd", "output")

		result, err := executeCommand(context.Background(), mock, logger, "testcmd", "arg1")
		if err != nil {
			t.Fatalf("executeCommand error: %v", err)
		}
		if result != "output" {
			t.Errorf("Expected 'output', got %q", result)
		}
		if !bytes.Contains(buf.Bytes(), []byte("executing command")) {
			t.Error("Expected 'executing command' in log output")
		}
		if !bytes.Contains(buf.Bytes(), []byte("command completed")) {
			t.Error("Expected 'command completed' in log output")
		}
	})

	t.Run("error with logger", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

		mock := newMockExecutor()
		mock.setError("testcmd", fmt.Errorf("mock failure"))

		_, err := executeCommand(context.Background(), mock, logger, "testcmd")
		if err == nil {
			t.Fatal("Expected error")
		}
		if !bytes.Contains(buf.Bytes(), []byte("executing command")) {
			t.Error("Expected 'executing command' in log output")
		}
		if !bytes.Contains(buf.Bytes(), []byte("command failed")) {
			t.Error("Expected 'command failed' in log output")
		}
	})
}
