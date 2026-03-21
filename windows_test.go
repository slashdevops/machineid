//go:build windows

package machineid

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
)

// --- parseWmicValue tests ---

func TestParseWmicValueValid(t *testing.T) {
	output := "ProcessorId=BFEBFBFF000906EA\r\n"
	value, err := parseWmicValue(output, "ProcessorId=")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if value != "BFEBFBFF000906EA" {
		t.Errorf("Expected 'BFEBFBFF000906EA', got %q", value)
	}
}

func TestParseWmicValueWithBlankLines(t *testing.T) {
	output := "\r\n\r\nProcessorId=BFEBFBFF000906EA\r\n\r\n"
	value, err := parseWmicValue(output, "ProcessorId=")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if value != "BFEBFBFF000906EA" {
		t.Errorf("Expected 'BFEBFBFF000906EA', got %q", value)
	}
}

func TestParseWmicValueEmpty(t *testing.T) {
	output := "ProcessorId=\r\n"
	_, err := parseWmicValue(output, "ProcessorId=")
	if err == nil {
		t.Error("Expected error for empty value")
	}
	var parseErr *ParseError
	if !errors.As(err, &parseErr) {
		t.Errorf("Expected ParseError, got %T", err)
	}
}

func TestParseWmicValueOEMPlaceholder(t *testing.T) {
	output := "SerialNumber=To be filled by O.E.M.\r\n"
	_, err := parseWmicValue(output, "SerialNumber=")
	if err == nil {
		t.Error("Expected error for OEM placeholder value")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestParseWmicValueNotFound(t *testing.T) {
	output := "SomeOtherKey=value\r\n"
	_, err := parseWmicValue(output, "ProcessorId=")
	if err == nil {
		t.Error("Expected error when prefix not found")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestParseWmicValueEmptyOutput(t *testing.T) {
	_, err := parseWmicValue("", "ProcessorId=")
	if err == nil {
		t.Error("Expected error for empty output")
	}
}

// --- parseWmicMultipleValues tests ---

func TestParseWmicMultipleValuesValid(t *testing.T) {
	output := "SerialNumber=ABC123\r\nSerialNumber=DEF456\r\n"
	values := parseWmicMultipleValues(output, "SerialNumber=")
	if len(values) != 2 {
		t.Fatalf("Expected 2 values, got %d", len(values))
	}
	if values[0] != "ABC123" || values[1] != "DEF456" {
		t.Errorf("Unexpected values: %v", values)
	}
}

func TestParseWmicMultipleValuesFiltersOEM(t *testing.T) {
	output := "SerialNumber=ABC123\r\nSerialNumber=To be filled by O.E.M.\r\nSerialNumber=DEF456\r\n"
	values := parseWmicMultipleValues(output, "SerialNumber=")
	if len(values) != 2 {
		t.Fatalf("Expected 2 values (OEM filtered), got %d", len(values))
	}
}

func TestParseWmicMultipleValuesEmpty(t *testing.T) {
	output := "SerialNumber=\r\n"
	values := parseWmicMultipleValues(output, "SerialNumber=")
	if len(values) != 0 {
		t.Errorf("Expected 0 values for empty, got %d", len(values))
	}
}

func TestParseWmicMultipleValuesNoMatch(t *testing.T) {
	output := "OtherKey=value\r\n"
	values := parseWmicMultipleValues(output, "SerialNumber=")
	if len(values) != 0 {
		t.Errorf("Expected 0 values, got %d", len(values))
	}
}

// --- parsePowerShellValue tests ---

func TestParsePowerShellValueValid(t *testing.T) {
	value, err := parsePowerShellValue("  BFEBFBFF000906EA  ")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if value != "BFEBFBFF000906EA" {
		t.Errorf("Expected 'BFEBFBFF000906EA', got %q", value)
	}
}

func TestParsePowerShellValueEmpty(t *testing.T) {
	_, err := parsePowerShellValue("   ")
	if err == nil {
		t.Error("Expected error for empty value")
	}
	var parseErr *ParseError
	if !errors.As(err, &parseErr) {
		t.Errorf("Expected ParseError, got %T", err)
	}
	if !errors.Is(err, ErrEmptyValue) {
		t.Errorf("Expected ErrEmptyValue, got %v", err)
	}
}

// --- parsePowerShellMultipleValues tests ---

func TestParsePowerShellMultipleValuesValid(t *testing.T) {
	output := "ABC123\nDEF456\nGHI789\n"
	values := parsePowerShellMultipleValues(output)
	if len(values) != 3 {
		t.Fatalf("Expected 3 values, got %d", len(values))
	}
}

func TestParsePowerShellMultipleValuesSkipsEmpty(t *testing.T) {
	output := "ABC123\n\n\nDEF456\n"
	values := parsePowerShellMultipleValues(output)
	if len(values) != 2 {
		t.Fatalf("Expected 2 values (empty lines skipped), got %d", len(values))
	}
}

func TestParsePowerShellMultipleValuesAllEmpty(t *testing.T) {
	output := "\n\n\n"
	values := parsePowerShellMultipleValues(output)
	if len(values) != 0 {
		t.Errorf("Expected 0 values, got %d", len(values))
	}
}

// --- windowsCPUID tests ---

func TestWindowsCPUIDWmicSuccess(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("wmic", "ProcessorId=BFEBFBFF000906EA\r\n")

	result, err := windowsCPUID(context.Background(), mock, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result != "BFEBFBFF000906EA" {
		t.Errorf("Expected 'BFEBFBFF000906EA', got %q", result)
	}
}

func TestWindowsCPUIDFallbackToPowerShell(t *testing.T) {
	mock := newMockExecutor()
	mock.setError("wmic", fmt.Errorf("wmic not found"))
	mock.setOutput("powershell", "BFEBFBFF000906EA")

	result, err := windowsCPUID(context.Background(), mock, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result != "BFEBFBFF000906EA" {
		t.Errorf("Expected 'BFEBFBFF000906EA', got %q", result)
	}
}

func TestWindowsCPUIDAllFail(t *testing.T) {
	mock := newMockExecutor()
	mock.setError("wmic", fmt.Errorf("wmic not found"))
	mock.setError("powershell", fmt.Errorf("powershell failed"))

	_, err := windowsCPUID(context.Background(), mock, nil)
	if err == nil {
		t.Error("Expected error when all methods fail")
	}
	if !errors.Is(err, ErrAllMethodsFailed) {
		t.Errorf("Expected ErrAllMethodsFailed, got %v", err)
	}
}

func TestWindowsCPUIDWmicParseFailFallback(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("wmic", "garbage output")         // wmic succeeds but parse fails
	mock.setOutput("powershell", "BFEBFBFF000906EA") // PowerShell fallback succeeds

	result, err := windowsCPUID(context.Background(), mock, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result != "BFEBFBFF000906EA" {
		t.Errorf("Expected 'BFEBFBFF000906EA', got %q", result)
	}
}

func TestWindowsCPUIDWithLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mock := newMockExecutor()
	mock.setError("wmic", fmt.Errorf("wmic not found"))
	mock.setOutput("powershell", "BFEBFBFF000906EA")

	_, err := windowsCPUID(context.Background(), mock, logger)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("falling back to PowerShell for CPU ID")) {
		t.Error("Expected fallback log message")
	}
}

// --- windowsMotherboardSerial tests ---

func TestWindowsMotherboardSerialWmicSuccess(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("wmic", "SerialNumber=ABC12345\r\n")

	result, err := windowsMotherboardSerial(context.Background(), mock, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result != "ABC12345" {
		t.Errorf("Expected 'ABC12345', got %q", result)
	}
}

func TestWindowsMotherboardSerialFallbackToPowerShell(t *testing.T) {
	mock := newMockExecutor()
	mock.setError("wmic", fmt.Errorf("wmic not found"))
	mock.setOutput("powershell", "ABC12345")

	result, err := windowsMotherboardSerial(context.Background(), mock, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result != "ABC12345" {
		t.Errorf("Expected 'ABC12345', got %q", result)
	}
}

func TestWindowsMotherboardSerialOEMPlaceholder(t *testing.T) {
	mock := newMockExecutor()
	mock.setError("wmic", fmt.Errorf("wmic not found"))
	mock.setOutput("powershell", "To be filled by O.E.M.")

	_, err := windowsMotherboardSerial(context.Background(), mock, nil)
	if err == nil {
		t.Error("Expected error for OEM placeholder")
	}
	if !errors.Is(err, ErrOEMPlaceholder) {
		t.Errorf("Expected ErrOEMPlaceholder, got %v", err)
	}
}

func TestWindowsMotherboardSerialAllFail(t *testing.T) {
	mock := newMockExecutor()
	mock.setError("wmic", fmt.Errorf("wmic not found"))
	mock.setError("powershell", fmt.Errorf("powershell failed"))

	_, err := windowsMotherboardSerial(context.Background(), mock, nil)
	if err == nil {
		t.Error("Expected error when all methods fail")
	}
	if !errors.Is(err, ErrAllMethodsFailed) {
		t.Errorf("Expected ErrAllMethodsFailed, got %v", err)
	}
}

func TestWindowsMotherboardSerialWithLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mock := newMockExecutor()
	mock.setError("wmic", fmt.Errorf("wmic not found"))
	mock.setError("powershell", fmt.Errorf("powershell failed"))

	if _, err := windowsMotherboardSerial(context.Background(), mock, logger); err == nil {
		t.Error("Expected error when all methods fail")
	}
	if !bytes.Contains(buf.Bytes(), []byte("falling back to PowerShell for motherboard serial")) {
		t.Error("Expected fallback log message")
	}
	if !bytes.Contains(buf.Bytes(), []byte("all motherboard serial methods failed")) {
		t.Error("Expected all-methods-failed log message")
	}
}

// --- windowsSystemUUID tests ---

func TestWindowsSystemUUIDWmicSuccess(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("wmic", "UUID=4C4C4544-0058-5210-8048-B4C04F595031\r\n")

	result, err := windowsSystemUUID(context.Background(), mock, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result != "4C4C4544-0058-5210-8048-B4C04F595031" {
		t.Errorf("Expected UUID, got %q", result)
	}
}

func TestWindowsSystemUUIDFallbackToPowerShell(t *testing.T) {
	mock := newMockExecutor()
	mock.setError("wmic", fmt.Errorf("wmic not found"))
	mock.setOutput("powershell", "4C4C4544-0058-5210-8048-B4C04F595031")

	result, err := windowsSystemUUID(context.Background(), mock, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result != "4C4C4544-0058-5210-8048-B4C04F595031" {
		t.Errorf("Expected UUID, got %q", result)
	}
}

func TestWindowsSystemUUIDAllFail(t *testing.T) {
	mock := newMockExecutor()
	mock.setError("wmic", fmt.Errorf("wmic not found"))
	mock.setError("powershell", fmt.Errorf("powershell failed"))

	_, err := windowsSystemUUID(context.Background(), mock, nil)
	if err == nil {
		t.Error("Expected error when all methods fail")
	}
}

func TestWindowsSystemUUIDWmicParseFailFallback(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("wmic", "garbage") // parse fails
	mock.setOutput("powershell", "UUID-FROM-PS")

	result, err := windowsSystemUUID(context.Background(), mock, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result != "UUID-FROM-PS" {
		t.Errorf("Expected 'UUID-FROM-PS', got %q", result)
	}
}

func TestWindowsSystemUUIDWithLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mock := newMockExecutor()
	mock.setOutput("wmic", "garbage")
	mock.setOutput("powershell", "UUID-LOGGED")

	_, err := windowsSystemUUID(context.Background(), mock, logger)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("wmic UUID parsing failed")) {
		t.Error("Expected 'wmic UUID parsing failed' in log")
	}
	if !bytes.Contains(buf.Bytes(), []byte("falling back to PowerShell for system UUID")) {
		t.Error("Expected fallback log message")
	}
}

// --- windowsSystemUUIDViaPowerShell tests ---

func TestWindowsSystemUUIDViaPowerShellSuccess(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("powershell", "UUID-PS-123")

	result, err := windowsSystemUUIDViaPowerShell(context.Background(), mock, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result != "UUID-PS-123" {
		t.Errorf("Expected 'UUID-PS-123', got %q", result)
	}
}

func TestWindowsSystemUUIDViaPowerShellError(t *testing.T) {
	mock := newMockExecutor()
	mock.setError("powershell", fmt.Errorf("powershell failed"))

	_, err := windowsSystemUUIDViaPowerShell(context.Background(), mock, nil)
	if err == nil {
		t.Error("Expected error when PowerShell fails")
	}
}

func TestWindowsSystemUUIDViaPowerShellEmpty(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("powershell", "   ")

	_, err := windowsSystemUUIDViaPowerShell(context.Background(), mock, nil)
	if err == nil {
		t.Error("Expected error for empty PowerShell output")
	}
	if !errors.Is(err, ErrEmptyValue) {
		t.Errorf("Expected ErrEmptyValue, got %v", err)
	}
}

// --- windowsDiskSerials tests ---

func TestWindowsDiskSerialsWmicSuccess(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("wmic", "SerialNumber=WD-12345\r\nSerialNumber=WD-67890\r\n")

	result, err := windowsDiskSerials(context.Background(), mock, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("Expected 2 serials, got %d", len(result))
	}
	if result[0] != "WD-12345" || result[1] != "WD-67890" {
		t.Errorf("Unexpected serials: %v", result)
	}
}

func TestWindowsDiskSerialsFallbackToPowerShell(t *testing.T) {
	mock := newMockExecutor()
	mock.setError("wmic", fmt.Errorf("wmic not found"))
	mock.setOutput("powershell", "WD-12345\nWD-67890")

	result, err := windowsDiskSerials(context.Background(), mock, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("Expected 2 serials, got %d", len(result))
	}
}

func TestWindowsDiskSerialsAllFail(t *testing.T) {
	mock := newMockExecutor()
	mock.setError("wmic", fmt.Errorf("wmic not found"))
	mock.setError("powershell", fmt.Errorf("powershell failed"))

	_, err := windowsDiskSerials(context.Background(), mock, nil)
	if err == nil {
		t.Error("Expected error when all methods fail")
	}
	if !errors.Is(err, ErrAllMethodsFailed) {
		t.Errorf("Expected ErrAllMethodsFailed, got %v", err)
	}
}

func TestWindowsDiskSerialsPowerShellEmpty(t *testing.T) {
	mock := newMockExecutor()
	mock.setError("wmic", fmt.Errorf("wmic not found"))
	mock.setOutput("powershell", "\n\n")

	_, err := windowsDiskSerials(context.Background(), mock, nil)
	if err == nil {
		t.Error("Expected error for empty PowerShell output")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestWindowsDiskSerialsWmicEmptyFallback(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("wmic", "SerialNumber=\r\n") // wmic returns empty serials
	mock.setOutput("powershell", "WD-FALLBACK")

	result, err := windowsDiskSerials(context.Background(), mock, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result) != 1 || result[0] != "WD-FALLBACK" {
		t.Errorf("Expected [WD-FALLBACK], got %v", result)
	}
}

func TestWindowsDiskSerialsWithLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mock := newMockExecutor()
	mock.setOutput("wmic", "SerialNumber=\r\n")
	mock.setOutput("powershell", "WD-LOG")

	_, err := windowsDiskSerials(context.Background(), mock, logger)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("wmic returned no disk serials")) {
		t.Error("Expected 'wmic returned no disk serials' in log")
	}
	if !bytes.Contains(buf.Bytes(), []byte("falling back to PowerShell for disk serials")) {
		t.Error("Expected fallback log message")
	}
}

// --- appendSingleResult tests ---

func TestAppendSingleResultSuccess(t *testing.T) {
	diag := &DiagnosticInfo{Errors: make(map[string]error)}
	r := componentResult{component: "cpu", prefix: "cpu:", value: "CPUID123"}

	result := appendSingleResult(nil, r, diag, nil)
	if len(result) != 1 {
		t.Fatalf("Expected 1 identifier, got %d", len(result))
	}
	if result[0] != "cpu:CPUID123" {
		t.Errorf("Expected 'cpu:CPUID123', got %q", result[0])
	}
	if len(diag.Collected) != 1 || diag.Collected[0] != "cpu" {
		t.Errorf("Expected cpu in collected, got %v", diag.Collected)
	}
}

func TestAppendSingleResultError(t *testing.T) {
	diag := &DiagnosticInfo{Errors: make(map[string]error)}
	r := componentResult{component: "cpu", prefix: "cpu:", err: fmt.Errorf("failed")}

	result := appendSingleResult(nil, r, diag, nil)
	if len(result) != 0 {
		t.Errorf("Expected 0 identifiers, got %d", len(result))
	}
	if _, exists := diag.Errors["cpu"]; !exists {
		t.Error("Expected error recorded for cpu")
	}
}

func TestAppendSingleResultEmpty(t *testing.T) {
	diag := &DiagnosticInfo{Errors: make(map[string]error)}
	r := componentResult{component: "cpu", prefix: "cpu:", value: ""}

	result := appendSingleResult(nil, r, diag, nil)
	if len(result) != 0 {
		t.Errorf("Expected 0 identifiers, got %d", len(result))
	}
	if _, exists := diag.Errors["cpu"]; !exists {
		t.Error("Expected error recorded for empty cpu")
	}
	var compErr *ComponentError
	if !errors.As(diag.Errors["cpu"], &compErr) {
		t.Error("Expected ComponentError")
	}
}

func TestAppendSingleResultWithLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	diag := &DiagnosticInfo{Errors: make(map[string]error)}
	r := componentResult{component: "cpu", prefix: "cpu:", value: "CPUID123"}

	appendSingleResult(nil, r, diag, logger)
	if !bytes.Contains(buf.Bytes(), []byte("component collected")) {
		t.Error("Expected 'component collected' log")
	}
	if !bytes.Contains(buf.Bytes(), []byte("component value")) {
		t.Error("Expected 'component value' log")
	}
}

func TestAppendSingleResultNilDiag(t *testing.T) {
	r := componentResult{component: "cpu", prefix: "cpu:", value: "CPUID123"}
	result := appendSingleResult(nil, r, nil, nil)
	if len(result) != 1 {
		t.Errorf("Expected 1 identifier with nil diag, got %d", len(result))
	}
}

// --- appendMultiResult tests ---

func TestAppendMultiResultSuccess(t *testing.T) {
	diag := &DiagnosticInfo{Errors: make(map[string]error)}
	r := componentResult{component: "disk", prefix: "disk:", values: []string{"SN1", "SN2"}, multi: true}

	result := appendMultiResult(nil, r, diag, nil)
	if len(result) != 2 {
		t.Fatalf("Expected 2 identifiers, got %d", len(result))
	}
	if result[0] != "disk:SN1" || result[1] != "disk:SN2" {
		t.Errorf("Unexpected identifiers: %v", result)
	}
	if len(diag.Collected) != 1 || diag.Collected[0] != "disk" {
		t.Errorf("Expected disk in collected, got %v", diag.Collected)
	}
}

func TestAppendMultiResultError(t *testing.T) {
	diag := &DiagnosticInfo{Errors: make(map[string]error)}
	r := componentResult{component: "disk", prefix: "disk:", err: fmt.Errorf("failed"), multi: true}

	result := appendMultiResult(nil, r, diag, nil)
	if len(result) != 0 {
		t.Errorf("Expected 0 identifiers, got %d", len(result))
	}
	if _, exists := diag.Errors["disk"]; !exists {
		t.Error("Expected error recorded for disk")
	}
}

func TestAppendMultiResultEmpty(t *testing.T) {
	diag := &DiagnosticInfo{Errors: make(map[string]error)}
	r := componentResult{component: "disk", prefix: "disk:", values: []string{}, multi: true}

	result := appendMultiResult(nil, r, diag, nil)
	if len(result) != 0 {
		t.Errorf("Expected 0 identifiers, got %d", len(result))
	}
	if _, exists := diag.Errors["disk"]; !exists {
		t.Error("Expected error recorded for empty disk")
	}
	var compErr *ComponentError
	if !errors.As(diag.Errors["disk"], &compErr) {
		t.Error("Expected ComponentError")
	}
	if !errors.Is(diag.Errors["disk"], ErrNoValues) {
		t.Error("Expected ErrNoValues")
	}
}

func TestAppendMultiResultWithLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	diag := &DiagnosticInfo{Errors: make(map[string]error)}
	r := componentResult{component: "disk", prefix: "disk:", values: []string{"SN1"}, multi: true}

	appendMultiResult(nil, r, diag, logger)
	if !bytes.Contains(buf.Bytes(), []byte("component collected")) {
		t.Error("Expected 'component collected' log")
	}
	if !bytes.Contains(buf.Bytes(), []byte("component values")) {
		t.Error("Expected 'component values' log")
	}
}

func TestAppendMultiResultNilDiag(t *testing.T) {
	r := componentResult{component: "disk", prefix: "disk:", values: []string{"SN1"}, multi: true}
	result := appendMultiResult(nil, r, nil, nil)
	if len(result) != 1 {
		t.Errorf("Expected 1 identifier with nil diag, got %d", len(result))
	}
}

// --- collectIdentifiers concurrent collection tests ---

func TestCollectIdentifiersConcurrent(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("wmic", "ProcessorId=CPUID123\r\n")
	mock.setOutput("powershell", "UUID-FROM-PS")

	p := New().WithExecutor(mock).WithCPU().WithSystemUUID()
	diag := &DiagnosticInfo{Errors: make(map[string]error)}

	identifiers, err := collectIdentifiers(context.Background(), p, diag)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(identifiers) == 0 {
		t.Error("Expected at least one identifier")
	}
	if len(diag.Collected) == 0 {
		t.Error("Expected at least one collected component")
	}
}

func TestCollectIdentifiersAllComponents(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("wmic", "ProcessorId=CPUID\r\nSerialNumber=MBSERIAL\r\nUUID=UUID123\r\n")

	p := New().WithExecutor(mock).
		WithCPU().
		WithMotherboard().
		WithSystemUUID().
		WithMAC().
		WithDisk()

	diag := &DiagnosticInfo{Errors: make(map[string]error)}

	identifiers, err := collectIdentifiers(context.Background(), p, diag)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// At minimum, MAC should succeed (uses net.Interfaces, not mocked)
	// Other components depend on mock matching command names
	t.Logf("Collected %d identifiers, %d components", len(identifiers), len(diag.Collected))
	t.Logf("Errors: %v", diag.Errors)
}

func TestCollectIdentifiersNoComponents(t *testing.T) {
	p := New()
	diag := &DiagnosticInfo{Errors: make(map[string]error)}

	identifiers, err := collectIdentifiers(context.Background(), p, diag)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(identifiers) != 0 {
		t.Errorf("Expected 0 identifiers with no components, got %d", len(identifiers))
	}
}

func TestCollectIdentifiersAllFail(t *testing.T) {
	mock := newMockExecutor()
	mock.setError("wmic", fmt.Errorf("wmic not found"))
	mock.setError("powershell", fmt.Errorf("powershell failed"))

	p := New().WithExecutor(mock).WithCPU().WithSystemUUID()
	diag := &DiagnosticInfo{Errors: make(map[string]error)}

	identifiers, err := collectIdentifiers(context.Background(), p, diag)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// All components should fail, identifiers should be empty
	if len(identifiers) != 0 {
		t.Errorf("Expected 0 identifiers when all fail, got %d", len(identifiers))
	}
	if len(diag.Errors) == 0 {
		t.Error("Expected errors recorded in diagnostics")
	}
}

// --- Provider integration tests with mock executor (Windows) ---

func TestProviderWithMockExecutor(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("wmic", "ProcessorId=BFEBFBFF000906EA\r\n")

	p := New().WithExecutor(mock).WithCPU()

	id, err := p.ID(context.Background())
	if err != nil {
		t.Fatalf("ID() error: %v", err)
	}
	if len(id) != 64 {
		t.Errorf("Expected 64-char ID, got %d", len(id))
	}

	// Verify caching
	id2, err := p.ID(context.Background())
	if err != nil {
		t.Fatalf("Second ID() error: %v", err)
	}
	if id != id2 {
		t.Error("Cached ID mismatch")
	}
}

func TestProviderErrorHandling(t *testing.T) {
	mock := newMockExecutor()
	mock.setError("wmic", fmt.Errorf("wmic not found"))
	mock.setError("powershell", fmt.Errorf("powershell failed"))

	p := New().WithExecutor(mock).WithCPU()

	_, err := p.ID(context.Background())
	if err == nil {
		t.Error("Expected error when all commands fail")
	}
	if !errors.Is(err, ErrNoIdentifiers) {
		t.Errorf("Expected ErrNoIdentifiers, got %v", err)
	}
}

func TestProviderDiagnosticsWindows(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("wmic", "ProcessorId=CPUID123\r\n")

	p := New().WithExecutor(mock).WithCPU().WithSystemUUID()

	if p.Diagnostics() != nil {
		t.Error("Diagnostics should be nil before ID()")
	}

	if _, err := p.ID(context.Background()); err != nil {
		t.Logf("ID() error (may be expected): %v", err)
	}

	diag := p.Diagnostics()
	if diag == nil {
		t.Fatal("Diagnostics should not be nil after ID()")
	}
	t.Logf("Collected: %v, Errors: %v", diag.Collected, diag.Errors)
}

func TestProviderWithLoggerWindows(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mock := newMockExecutor()
	mock.setOutput("wmic", "ProcessorId=CPUID123\r\n")

	p := New().WithExecutor(mock).WithLogger(logger).WithCPU()

	_, err := p.ID(context.Background())
	if err != nil {
		t.Fatalf("ID() error: %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte("generating machine ID")) {
		t.Error("Expected 'generating machine ID' in log")
	}
	if !bytes.Contains(buf.Bytes(), []byte("machine ID generated")) {
		t.Error("Expected 'machine ID generated' in log")
	}
}

func TestProviderCachedIDWindows(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("wmic", "ProcessorId=CPU1\r\n")

	p := New().WithExecutor(mock).WithCPU()

	id1, err := p.ID(context.Background())
	if err != nil {
		t.Fatalf("First ID() error: %v", err)
	}

	// Change mock - should still return cached
	mock.setOutput("wmic", "ProcessorId=CPU2\r\n")

	id2, err := p.ID(context.Background())
	if err != nil {
		t.Fatalf("Second ID() error: %v", err)
	}
	if id1 != id2 {
		t.Error("Cached ID was modified on subsequent call")
	}
}

func TestProviderValidateWindows(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("wmic", "ProcessorId=CPUID\r\n")

	p := New().WithExecutor(mock).WithCPU()

	id, err := p.ID(context.Background())
	if err != nil {
		t.Fatalf("ID() error: %v", err)
	}

	valid, err := p.Validate(context.Background(), id)
	if err != nil {
		t.Fatalf("Validate error: %v", err)
	}
	if !valid {
		t.Error("Expected validation to succeed")
	}

	valid, err = p.Validate(context.Background(), "wrong-id")
	if err != nil {
		t.Fatalf("Validate error: %v", err)
	}
	if valid {
		t.Error("Expected validation to fail for wrong ID")
	}
}
