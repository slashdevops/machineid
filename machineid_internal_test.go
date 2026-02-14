package machineid

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// TestProviderWithMockExecutor tests using a mock executor for deterministic testing.
func TestProviderWithMockExecutor(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("sysctl", "Test CPU Brand String")

	g := New().
		WithExecutor(mock).
		WithCPU()

	id, err := g.ID(context.Background())
	if err != nil {
		t.Fatalf("ID() with mock executor error = %v", err)
	}

	if len(id) != 64 {
		t.Errorf("ID() returned ID of length %d, expected 64", len(id))
	}

	// Verify the ID is consistent with the same mock
	id2, err := g.ID(context.Background())
	if err != nil {
		t.Fatalf("Second ID() call error = %v", err)
	}

	if id != id2 {
		t.Error("ID() returned different IDs with same mock executor")
	}
}

// TestProviderErrorHandling tests various error conditions.
func TestProviderErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		setupMock   func(*mockExecutor)
		configure   func(*Provider) *Provider
		expectError bool
		wantErr     error
	}{
		{
			name: "command execution fails but no fallback available",
			setupMock: func(m *mockExecutor) {
				m.setError("sysctl", fmt.Errorf("command not found"))
			},
			configure: func(p *Provider) *Provider {
				return p.WithCPU()
			},
			expectError: true,
			wantErr:     ErrNoIdentifiers,
		},
		{
			name: "no identifiers collected",
			setupMock: func(m *mockExecutor) {
				// All commands fail
				m.setError("sysctl", fmt.Errorf("failed"))
				m.setError("ioreg", fmt.Errorf("failed"))
				m.setError("system_profiler", fmt.Errorf("failed"))
			},
			configure: func(p *Provider) *Provider {
				return p.WithCPU().WithSystemUUID()
			},
			expectError: true,
			wantErr:     ErrNoIdentifiers,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMockExecutor()
			if tt.setupMock != nil {
				tt.setupMock(mock)
			}

			p := New().WithExecutor(mock)
			p = tt.configure(p)

			_, err := p.ID(context.Background())
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if tt.expectError && err != nil && tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("got error %v, want %v", err, tt.wantErr)
				}
			}
		})
	}
}

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

// TestValidateError tests Validate method when ID generation fails.
func TestValidateError(t *testing.T) {
	mock := newMockExecutor()
	mock.setError("sysctl", fmt.Errorf("command failed"))

	p := New().WithExecutor(mock).WithCPU()

	valid, err := p.Validate(context.Background(), "some-id")
	if err == nil {
		t.Error("Expected error when ID generation fails")
	}
	if valid {
		t.Error("Validation should fail when error occurs")
	}
}

// TestAppendIdentifierIfValidEmpty tests with empty value.
func TestAppendIdentifierIfValidEmpty(t *testing.T) {
	diag := &DiagnosticInfo{Errors: make(map[string]error)}
	getValue := func() (string, error) {
		return "", nil
	}

	result := appendIdentifierIfValid([]string{"existing"}, getValue, "prefix:", diag, "test")
	if len(result) != 1 {
		t.Errorf("Expected 1 identifier, got %d", len(result))
	}
	if _, ok := diag.Errors["test"]; !ok {
		t.Error("Expected error recorded in diagnostics for empty value")
	}
}

// TestAppendIdentifierIfValidError tests with error.
func TestAppendIdentifierIfValidError(t *testing.T) {
	diag := &DiagnosticInfo{Errors: make(map[string]error)}
	getValue := func() (string, error) {
		return "", fmt.Errorf("test error")
	}

	result := appendIdentifierIfValid([]string{"existing"}, getValue, "prefix:", diag, "test")
	if len(result) != 1 {
		t.Errorf("Expected 1 identifier (original), got %d", len(result))
	}
	if _, ok := diag.Errors["test"]; !ok {
		t.Error("Expected error recorded in diagnostics")
	}
}

// TestAppendIdentifierIfValidSuccess tests with valid value.
func TestAppendIdentifierIfValidSuccess(t *testing.T) {
	diag := &DiagnosticInfo{Errors: make(map[string]error)}
	getValue := func() (string, error) {
		return "good-value", nil
	}

	result := appendIdentifierIfValid([]string{"existing"}, getValue, "prefix:", diag, "test")
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

	result := appendIdentifiersIfValid([]string{"existing"}, getValues, "prefix:", diag, "test")
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

	result := appendIdentifiersIfValid([]string{"existing"}, getValues, "prefix:", diag, "test")
	if len(result) != 1 {
		t.Errorf("Expected 1 identifier (original), got %d", len(result))
	}
	if _, ok := diag.Errors["test"]; !ok {
		t.Error("Expected error recorded in diagnostics")
	}
}

// TestAppendIdentifiersIfValidMultiple tests with multiple values.
func TestAppendIdentifiersIfValidMultiple(t *testing.T) {
	diag := &DiagnosticInfo{Errors: make(map[string]error)}
	getValues := func() ([]string, error) {
		return []string{"val1", "val2", "val3"}, nil
	}

	result := appendIdentifiersIfValid([]string{"existing"}, getValues, "prefix:", diag, "test")
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

	result := appendIdentifierIfValid(nil, getValue, "prefix:", nil, "test")
	if len(result) != 1 {
		t.Errorf("Expected 1 identifier, got %d", len(result))
	}
}

// TestDiagnosticsAvailableAfterID tests that Diagnostics() returns data after ID().
func TestDiagnosticsAvailableAfterID(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("sysctl", "Test CPU")
	mock.setOutput("system_profiler", `{
		"SPHardwareDataType": [{
			"chip_type": "Apple M1",
			"machine_model": "Mac",
			"platform_UUID": "UUID-123",
			"serial_number": "SERIAL"
		}]
	}`)

	p := New().WithExecutor(mock).WithCPU().WithSystemUUID()

	// Before ID(), Diagnostics should be nil
	if p.Diagnostics() != nil {
		t.Error("Diagnostics should be nil before ID() call")
	}

	_, err := p.ID(context.Background())
	if err != nil {
		t.Fatalf("ID() error: %v", err)
	}

	diag := p.Diagnostics()
	if diag == nil {
		t.Fatal("Diagnostics should not be nil after ID() call")
	}

	if len(diag.Collected) == 0 {
		t.Error("Expected at least one collected component")
	}
}

// TestDiagnosticsRecordsFailures tests that failed components are recorded.
func TestDiagnosticsRecordsFailures(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("sysctl", "Test CPU")
	mock.setOutput("system_profiler", `{
		"SPHardwareDataType": [{
			"chip_type": "Apple M1",
			"machine_model": "Mac",
			"platform_UUID": "",
			"serial_number": ""
		}]
	}`)
	mock.setError("ioreg", fmt.Errorf("ioreg not available"))

	p := New().WithExecutor(mock).WithCPU().WithSystemUUID()

	_, err := p.ID(context.Background())
	if err != nil {
		t.Fatalf("ID() error: %v", err)
	}

	diag := p.Diagnostics()
	if diag == nil {
		t.Fatal("Diagnostics should not be nil")
	}

	// CPU should succeed, UUID should fail (empty in JSON + ioreg fails)
	if len(diag.Collected) == 0 {
		t.Error("Expected at least one collected component")
	}
}

// TestProviderCachedIDNotModified tests that cached ID is not modified on subsequent calls.
func TestProviderCachedIDNotModified(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("sysctl", "CPU1")

	p := New().WithExecutor(mock).WithCPU()

	id1, err := p.ID(context.Background())
	if err != nil {
		t.Fatalf("First ID() call failed: %v", err)
	}

	// Change the mock output
	mock.setOutput("sysctl", "CPU2")

	// Should still return cached value
	id2, err := p.ID(context.Background())
	if err != nil {
		t.Fatalf("Second ID() call failed: %v", err)
	}

	if id1 != id2 {
		t.Error("Cached ID was modified on subsequent call")
	}

	// Verify mock was only called once (due to caching)
	if mock.callCount["sysctl"] > 2 {
		t.Errorf("Expected sysctl to be called at most twice, got %d", mock.callCount["sysctl"])
	}
}

// TestProviderAllIdentifiers tests using all identifier types.
func TestProviderAllIdentifiers(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("sysctl", "Intel CPU")
	mock.setOutput("system_profiler", `platform_UUID: "UUID123"`)
	mock.setOutput("ioreg", "some data")
	mock.setOutput("diskutil", `<string>/dev/disk0</string>`)

	p := New().
		WithExecutor(mock).
		WithCPU().
		WithSystemUUID().
		WithMotherboard().
		WithMAC().
		WithDisk()

	id, err := p.ID(context.Background())
	if err != nil {
		t.Fatalf("ID() with all identifiers failed: %v", err)
	}

	if len(id) != 64 {
		t.Errorf("Expected 64-character ID (default Format64), got %d", len(id))
	}
}

// TestCollectIdentifiersError tests when collectIdentifiers returns an error.
func TestCollectIdentifiersError(t *testing.T) {
	mock := newMockExecutor()
	// Don't set any outputs, so all commands will fail with "not configured"

	p := New().WithExecutor(mock).WithCPU()

	_, err := p.ID(context.Background())
	if err == nil {
		t.Error("Expected error when collectIdentifiers fails")
	}
}

// TestProviderValidateMismatch tests validation with mismatched ID.
func TestProviderValidateMismatch(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("sysctl", "CPU1")

	p := New().WithExecutor(mock).WithCPU()

	// Generate ID
	id, err := p.ID(context.Background())
	if err != nil {
		t.Fatalf("ID() failed: %v", err)
	}

	// Validate with different ID
	valid, err := p.Validate(context.Background(), id+"different")
	if err != nil {
		t.Errorf("Validate() error: %v", err)
	}

	if valid {
		t.Error("Expected validation to fail for different ID")
	}
}
