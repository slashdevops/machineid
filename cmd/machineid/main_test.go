package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/slashdevops/machineid"
)

func TestParseFormatMode(t *testing.T) {
	tests := []struct {
		input   int
		want    machineid.FormatMode
		wantErr bool
	}{
		{32, machineid.Format32, false},
		{64, machineid.Format64, false},
		{128, machineid.Format128, false},
		{256, machineid.Format256, false},
		{0, 0, true},
		{16, 0, true},
		{512, 0, true},
		{-1, 0, true},
	}

	for _, tt := range tests {
		got, err := parseFormatMode(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseFormatMode(%d) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("parseFormatMode(%d) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestFormatDiagnosticsNil(t *testing.T) {
	provider := machineid.New()
	// Before ID() call, Diagnostics() is nil
	result := formatDiagnostics(provider)
	if result != nil {
		t.Error("Expected nil for provider without diagnostics")
	}
}

func TestFormatDiagnosticsWithData(t *testing.T) {
	provider := machineid.New().WithCPU().WithSystemUUID()
	// Generate ID to populate diagnostics
	_, err := provider.ID(t.Context())
	if err != nil {
		t.Fatalf("ID() error: %v", err)
	}

	result := formatDiagnostics(provider)
	if result == nil {
		t.Fatal("Expected non-nil diagnostics")
	}

	if _, ok := result["collected"]; !ok {
		t.Error("Expected 'collected' key in diagnostics")
	}
}

func TestPrintDiagnosticsNil(t *testing.T) {
	provider := machineid.New()
	// Should not panic
	printDiagnostics(provider)
}

func TestPrintDiagnosticsWithData(t *testing.T) {
	provider := machineid.New().WithCPU().WithSystemUUID()
	_, err := provider.ID(t.Context())
	if err != nil {
		t.Fatalf("ID() error: %v", err)
	}
	// Should not panic
	printDiagnostics(provider)
}

func TestFormatDiagnosticsWithErrors(t *testing.T) {
	provider := machineid.New().WithCPU().WithDisk()
	_, _ = provider.ID(t.Context())

	result := formatDiagnostics(provider)
	if result == nil {
		t.Fatal("Expected non-nil diagnostics")
	}

	// At least collected should be present
	if _, ok := result["collected"]; !ok {
		t.Error("Expected 'collected' key in diagnostics")
	}
}

func TestPrintJSON(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printJSON(map[string]any{"key": "value"})

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("printJSON output is not valid JSON: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("Expected key=value, got %v", result["key"])
	}
}

func TestHandleValidateValid(t *testing.T) {
	provider := machineid.New().WithCPU().WithSystemUUID()
	id, err := provider.ID(t.Context())
	if err != nil {
		t.Fatalf("ID() error: %v", err)
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	handleValidate(t.Context(), provider, id, false)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)

	if !bytes.Contains(buf.Bytes(), []byte("valid: machine ID matches")) {
		t.Errorf("Expected 'valid: machine ID matches', got %q", buf.String())
	}
}

func TestHandleValidateValidJSON(t *testing.T) {
	provider := machineid.New().WithCPU().WithSystemUUID()
	id, err := provider.ID(t.Context())
	if err != nil {
		t.Fatalf("ID() error: %v", err)
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	handleValidate(t.Context(), provider, id, true)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("handleValidate JSON output is not valid JSON: %v", err)
	}
	if result["valid"] != true {
		t.Errorf("Expected valid=true, got %v", result["valid"])
	}
}
