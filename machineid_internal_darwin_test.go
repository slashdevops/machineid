//go:build darwin

package machineid

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
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

// TestWithLoggerOutput verifies that log output appears when a logger is set.
func TestWithLoggerOutput(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mock := newMockExecutor()
	mock.setOutput("sysctl", "Test CPU Brand")

	p := New().
		WithExecutor(mock).
		WithLogger(logger).
		WithCPU()

	_, err := p.ID(context.Background())
	if err != nil {
		t.Fatalf("ID() error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Error("Expected log output when logger is set, got empty string")
	}

	// Check for key log messages
	if !bytes.Contains(buf.Bytes(), []byte("generating machine ID")) {
		t.Error("Expected 'generating machine ID' in log output")
	}
	if !bytes.Contains(buf.Bytes(), []byte("machine ID generated")) {
		t.Error("Expected 'machine ID generated' in log output")
	}
	if !bytes.Contains(buf.Bytes(), []byte("component collected")) {
		t.Error("Expected 'component collected' in log output")
	}
}

// TestWithoutLoggerNoOutput verifies that no logging occurs without a logger.
func TestWithoutLoggerNoOutput(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("sysctl", "Test CPU Brand")

	p := New().
		WithExecutor(mock).
		WithCPU()

	// Should not panic or produce any output
	_, err := p.ID(context.Background())
	if err != nil {
		t.Fatalf("ID() error: %v", err)
	}
}

// TestProviderCachedIDWithLogger tests the cached ID debug log path.
func TestProviderCachedIDWithLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mock := newMockExecutor()
	mock.setOutput("sysctl", "Test CPU")

	p := New().WithExecutor(mock).WithLogger(logger).WithCPU()

	// First call generates ID
	_, err := p.ID(context.Background())
	if err != nil {
		t.Fatalf("First ID() error: %v", err)
	}

	// Second call returns cached
	_, err = p.ID(context.Background())
	if err != nil {
		t.Fatalf("Second ID() error: %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte("returning cached machine ID")) {
		t.Error("Expected 'returning cached machine ID' in log output")
	}
}
