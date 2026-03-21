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

func TestParseMACFilter(t *testing.T) {
	tests := []struct {
		input   string
		want    machineid.MACFilter
		wantErr bool
	}{
		{"physical", machineid.MACFilterPhysical, false},
		{"Physical", machineid.MACFilterPhysical, false},
		{"PHYSICAL", machineid.MACFilterPhysical, false},
		{"all", machineid.MACFilterAll, false},
		{"ALL", machineid.MACFilterAll, false},
		{"virtual", machineid.MACFilterVirtual, false},
		{"Virtual", machineid.MACFilterVirtual, false},
		{"invalid", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		got, err := parseMACFilter(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseMACFilter(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("parseMACFilter(%q) = %v, want %v", tt.input, got, tt.want)
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
	if _, err := provider.ID(t.Context()); err != nil {
		t.Logf("ID() error (may be expected): %v", err)
	}

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
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	os.Stdout = w

	printJSON(map[string]any{"key": "value"})

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("io.Copy error: %v", err)
	}

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
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	os.Stdout = w

	handleValidate(t.Context(), provider, id, false)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("io.Copy error: %v", err)
	}

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
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	os.Stdout = w

	handleValidate(t.Context(), provider, id, true)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("io.Copy error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("handleValidate JSON output is not valid JSON: %v", err)
	}
	if result["valid"] != true {
		t.Errorf("Expected valid=true, got %v", result["valid"])
	}
}

func TestResolveVersion(t *testing.T) {
	v := resolveVersion()
	if v == "" {
		t.Error("resolveVersion() returned empty string")
	}
	// In test environment without ldflags, should return "devel" or a module version
	t.Logf("resolveVersion() = %q", v)
}

func TestPrintFlag(t *testing.T) {
	// Capture stderr
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	os.Stderr = w

	printFlag(w, "-cpu", "Include CPU identifier")

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("io.Copy error: %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("-cpu")) {
		t.Errorf("Expected '-cpu' in output, got %q", output)
	}
	if !bytes.Contains([]byte(output), []byte("Include CPU identifier")) {
		t.Errorf("Expected description in output, got %q", output)
	}
}

func TestPrintField(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	os.Stdout = w

	printField("Go version", "go1.25.0")

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("io.Copy error: %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("Go version:")) {
		t.Errorf("Expected 'Go version:' in output, got %q", output)
	}
	if !bytes.Contains([]byte(output), []byte("go1.25.0")) {
		t.Errorf("Expected 'go1.25.0' in output, got %q", output)
	}
}

func TestPrintFieldEmpty(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	os.Stdout = w

	printField("Empty", "")

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("io.Copy error: %v", err)
	}

	if buf.Len() != 0 {
		t.Errorf("Expected no output for empty value, got %q", buf.String())
	}
}
