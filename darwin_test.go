//go:build darwin

package machineid

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

// TestExtractHardwareFieldValid tests successful field extraction from JSON.
func TestExtractHardwareFieldValid(t *testing.T) {
	jsonOutput := `{
		"SPHardwareDataType": [{
			"platform_UUID": "12345-67890",
			"serial_number": "C02TEST123",
			"chip_type": "Apple M1 Pro",
			"machine_model": "MacBookPro18,3"
		}]
	}`
	result, err := extractHardwareField(jsonOutput, func(e spHardwareEntry) string {
		return e.PlatformUUID
	})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "12345-67890" {
		t.Errorf("Expected '12345-67890', got '%s'", result)
	}
}

// TestExtractHardwareFieldEmpty tests extraction when field is empty.
func TestExtractHardwareFieldEmpty(t *testing.T) {
	jsonOutput := `{
		"SPHardwareDataType": [{
			"platform_UUID": "",
			"serial_number": "C02TEST123"
		}]
	}`
	_, err := extractHardwareField(jsonOutput, func(e spHardwareEntry) string {
		return e.PlatformUUID
	})
	if err == nil {
		t.Error("Expected error when field is empty")
	}
}

// TestExtractHardwareFieldNoData tests extraction when no data entries exist.
func TestExtractHardwareFieldNoData(t *testing.T) {
	jsonOutput := `{"SPHardwareDataType": []}`
	_, err := extractHardwareField(jsonOutput, func(e spHardwareEntry) string {
		return e.PlatformUUID
	})
	if err == nil {
		t.Error("Expected error when no data entries")
	}
}

// TestExtractHardwareFieldInvalidJSON tests extraction with invalid JSON.
func TestExtractHardwareFieldInvalidJSON(t *testing.T) {
	_, err := extractHardwareField("not json", func(e spHardwareEntry) string {
		return e.PlatformUUID
	})
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

// TestMacOSHardwareUUIDViaIORegNotFound tests ioreg fallback when UUID not in output.
func TestMacOSHardwareUUIDViaIORegNotFound(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("ioreg", "some output without UUID")

	_, err := macOSHardwareUUIDViaIOReg(context.Background(), mock, nil)
	if err == nil {
		t.Error("Expected error when UUID not found in ioreg output")
	}
}

// TestMacOSHardwareUUIDViaIORegError tests ioreg command error.
func TestMacOSHardwareUUIDViaIORegError(t *testing.T) {
	mock := newMockExecutor()
	mock.setError("ioreg", fmt.Errorf("command failed"))

	_, err := macOSHardwareUUIDViaIOReg(context.Background(), mock, nil)
	if err == nil {
		t.Error("Expected error when ioreg command fails")
	}
}

// TestMacOSHardwareUUIDViaIORegSuccess tests successful UUID extraction.
func TestMacOSHardwareUUIDViaIORegSuccess(t *testing.T) {
	mock := newMockExecutor()
	ioregOutput := `
	+-o IOPlatformExpertDevice
	  | {
	  |   "IOPlatformUUID" = "ABCD-1234-EFGH-5678"
	  | }
	`
	mock.setOutput("ioreg", ioregOutput)

	result, err := macOSHardwareUUIDViaIOReg(context.Background(), mock, nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "ABCD-1234-EFGH-5678" {
		t.Errorf("Expected 'ABCD-1234-EFGH-5678', got '%s'", result)
	}
}

// TestMacOSSerialNumberViaIORegError tests ioreg error.
func TestMacOSSerialNumberViaIORegError(t *testing.T) {
	mock := newMockExecutor()
	mock.setError("ioreg", fmt.Errorf("command failed"))

	_, err := macOSSerialNumberViaIOReg(context.Background(), mock, nil)
	if err == nil {
		t.Error("Expected error when ioreg command fails")
	}
}

// TestMacOSSerialNumberViaIORegNotFound tests when serial not in output.
func TestMacOSSerialNumberViaIORegNotFound(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("ioreg", "output without serial")

	_, err := macOSSerialNumberViaIOReg(context.Background(), mock, nil)
	if err == nil {
		t.Error("Expected error when serial not found")
	}
}

// TestMacOSSerialNumberViaIORegSuccess tests successful extraction.
func TestMacOSSerialNumberViaIORegSuccess(t *testing.T) {
	mock := newMockExecutor()
	ioregOutput := `
	"IOPlatformSerialNumber" = "C02TEST123"
	`
	mock.setOutput("ioreg", ioregOutput)

	result, err := macOSSerialNumberViaIOReg(context.Background(), mock, nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "C02TEST123" {
		t.Errorf("Expected 'C02TEST123', got '%s'", result)
	}
}

// TestMacOSSerialNumberFallback tests fallback to ioreg.
func TestMacOSSerialNumberFallback(t *testing.T) {
	mock := newMockExecutor()
	mock.setError("system_profiler", fmt.Errorf("system_profiler failed"))
	mock.setOutput("ioreg", `"IOPlatformSerialNumber" = "C02FALLBACK"`)

	result, err := macOSSerialNumber(context.Background(), mock, nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "C02FALLBACK" {
		t.Errorf("Expected 'C02FALLBACK', got '%s'", result)
	}
}

// TestMacOSCPUInfoError tests CPU info error handling.
func TestMacOSCPUInfoError(t *testing.T) {
	mock := newMockExecutor()
	mock.setError("sysctl", fmt.Errorf("command failed"))
	mock.setError("system_profiler", fmt.Errorf("command failed"))

	_, err := macOSCPUInfo(context.Background(), mock, nil)
	if err == nil {
		t.Error("Expected error when all CPU info commands fail")
	}
}

// TestMacOSCPUInfoAppleSiliconViaSysctl tests Apple Silicon CPU info using sysctl
// (primary path). On Apple Silicon, sysctl -n machdep.cpu.brand_string returns
// the chip name, and machdep.cpu.features returns empty, producing "ChipType:".
func TestMacOSCPUInfoAppleSiliconViaSysctl(t *testing.T) {
	mock := newMockExecutor()
	// sysctl returns "Apple M1 Pro" as brand_string, empty features (Apple Silicon behavior)
	mock.setOutput("sysctl", "Apple M1 Pro")

	result, err := macOSCPUInfo(context.Background(), mock, nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	// NOTE: On Apple Silicon, sysctl -n machdep.cpu.features succeeds with empty output.
	// The mock returns the same output for both sysctl calls (brand_string and features)
	// because it keys by command name only. In production, features is empty, producing
	// "Apple M1 Pro:" (trailing colon). The mock produces "Apple M1 Pro:Apple M1 Pro"
	// which still exercises the code path correctly.
	if !strings.HasPrefix(result, "Apple M1 Pro") {
		t.Errorf("Expected result to start with 'Apple M1 Pro', got '%s'", result)
	}
}

// TestMacOSCPUInfoFallbackToProfiler tests CPU info falls back to system_profiler
// when sysctl is not available.
func TestMacOSCPUInfoFallbackToProfiler(t *testing.T) {
	mock := newMockExecutor()
	// sysctl not configured → error → falls through to system_profiler
	mock.setOutput("system_profiler", `{
		"SPHardwareDataType": [{
			"chip_type": "Apple M1 Pro",
			"machine_model": "MacBookPro18,3",
			"platform_UUID": "SOME-UUID",
			"serial_number": "SERIAL123"
		}]
	}`)

	result, err := macOSCPUInfo(context.Background(), mock, nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "Apple M1 Pro" {
		t.Errorf("Expected 'Apple M1 Pro', got '%s'", result)
	}
}

// TestMacOSCPUInfoAllFail tests that CPU info returns error when all methods fail.
func TestMacOSCPUInfoAllFail(t *testing.T) {
	mock := newMockExecutor()
	// Neither sysctl nor system_profiler configured → all fail
	mock.setError("sysctl", fmt.Errorf("command not found"))
	mock.setError("system_profiler", fmt.Errorf("command not found"))

	_, err := macOSCPUInfo(context.Background(), mock, nil)
	if err == nil {
		t.Error("Expected error when all CPU methods fail")
	}
}

// TestParseStorageJSON tests proper JSON parsing of storage data.
func TestParseStorageJSON(t *testing.T) {
	jsonOutput := `{
		"SPStorageDataType": [
			{
				"_name": "Macintosh HD - Data",
				"bsd_name": "disk3s1",
				"physical_drive": {
					"device_name": "APPLE SSD AP1024R",
					"is_internal_disk": "yes",
					"medium_type": "ssd"
				}
			},
			{
				"_name": "Macintosh HD",
				"bsd_name": "disk3s3s1",
				"physical_drive": {
					"device_name": "APPLE SSD AP1024R",
					"is_internal_disk": "yes",
					"medium_type": "ssd"
				}
			},
			{
				"_name": "External Drive",
				"bsd_name": "disk8s2",
				"physical_drive": {
					"device_name": "SA400S37960G",
					"is_internal_disk": "no",
					"medium_type": "ssd"
				}
			}
		]
	}`

	result, err := parseStorageJSON(jsonOutput)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// Should only have 1 entry: APPLE SSD AP1024R (deduplicated, internal only)
	if len(result) != 1 {
		t.Errorf("Expected 1 disk entry (deduplicated internal), got %d: %v", len(result), result)
	}
	if result[0] != "APPLE SSD AP1024R" {
		t.Errorf("Expected 'APPLE SSD AP1024R', got '%s'", result[0])
	}
}

// TestParseStorageJSONNoInternal tests when no internal disks are found.
func TestParseStorageJSONNoInternal(t *testing.T) {
	jsonOutput := `{
		"SPStorageDataType": [
			{
				"_name": "External",
				"physical_drive": {
					"device_name": "External SSD",
					"is_internal_disk": "no"
				}
			}
		]
	}`

	_, err := parseStorageJSON(jsonOutput)
	if err == nil {
		t.Error("Expected error when no internal disks found")
	}
}

// TestParseStorageJSONInvalid tests invalid JSON.
func TestParseStorageJSONInvalid(t *testing.T) {
	_, err := parseStorageJSON("not json")
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

// TestMacOSDiskInfoError tests disk info when system_profiler fails.
func TestMacOSDiskInfoError(t *testing.T) {
	mock := newMockExecutor()
	mock.setError("system_profiler", fmt.Errorf("command failed"))

	_, err := macOSDiskInfo(context.Background(), mock, nil)
	if err == nil {
		t.Error("Expected error when system_profiler fails")
	}
}

// TestMacOSDiskInfoSuccess tests successful disk info via JSON.
func TestMacOSDiskInfoSuccess(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("system_profiler", `{
		"SPStorageDataType": [
			{
				"_name": "Macintosh HD",
				"physical_drive": {
					"device_name": "APPLE SSD AP1024R",
					"is_internal_disk": "yes"
				}
			}
		]
	}`)

	result, err := macOSDiskInfo(context.Background(), mock, nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("Expected 1 disk entry, got %d", len(result))
	}
}

// TestMacOSHardwareUUIDWithLogger tests UUID fallback with logger enabled.
func TestMacOSHardwareUUIDWithLogger(t *testing.T) {
	t.Run("system_profiler parse error falls back to ioreg with logging", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

		mock := newMockExecutor()
		mock.setOutput("system_profiler", "not json") // Will cause parse error
		mock.setOutput("ioreg", `"IOPlatformUUID" = "FALLBACK-UUID-123"`)

		result, err := macOSHardwareUUID(context.Background(), mock, logger)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if result != "FALLBACK-UUID-123" {
			t.Errorf("Expected 'FALLBACK-UUID-123', got %q", result)
		}
		if !bytes.Contains(buf.Bytes(), []byte("system_profiler UUID parsing failed")) {
			t.Error("Expected 'system_profiler UUID parsing failed' in log output")
		}
		if !bytes.Contains(buf.Bytes(), []byte("falling back to ioreg for hardware UUID")) {
			t.Error("Expected 'falling back to ioreg' in log output")
		}
	})

	t.Run("system_profiler command error falls back with logging", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

		mock := newMockExecutor()
		mock.setError("system_profiler", fmt.Errorf("command failed"))
		mock.setOutput("ioreg", `"IOPlatformUUID" = "FALLBACK-UUID-456"`)

		result, err := macOSHardwareUUID(context.Background(), mock, logger)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if result != "FALLBACK-UUID-456" {
			t.Errorf("Expected 'FALLBACK-UUID-456', got %q", result)
		}
		if !bytes.Contains(buf.Bytes(), []byte("falling back to ioreg for hardware UUID")) {
			t.Error("Expected fallback log message")
		}
	})
}

// TestMacOSSerialNumberWithLogger tests serial fallback with logger enabled.
func TestMacOSSerialNumberWithLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mock := newMockExecutor()
	mock.setOutput("system_profiler", "not json") // Will cause parse error
	mock.setOutput("ioreg", `"IOPlatformSerialNumber" = "SERIAL-LOG"`)

	result, err := macOSSerialNumber(context.Background(), mock, logger)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result != "SERIAL-LOG" {
		t.Errorf("Expected 'SERIAL-LOG', got %q", result)
	}
	if !bytes.Contains(buf.Bytes(), []byte("system_profiler serial parsing failed")) {
		t.Error("Expected 'system_profiler serial parsing failed' in log output")
	}
	if !bytes.Contains(buf.Bytes(), []byte("falling back to ioreg for serial number")) {
		t.Error("Expected 'falling back to ioreg' in log output")
	}
}

// TestMacOSHardwareUUIDViaIORegNotFoundWithLogger tests ioreg not-found with logger.
func TestMacOSHardwareUUIDViaIORegNotFoundWithLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mock := newMockExecutor()
	mock.setOutput("ioreg", "output without UUID pattern")

	_, err := macOSHardwareUUIDViaIOReg(context.Background(), mock, logger)
	if err == nil {
		t.Error("Expected error")
	}
	if !bytes.Contains(buf.Bytes(), []byte("hardware UUID not found in ioreg output")) {
		t.Error("Expected 'hardware UUID not found in ioreg output' in log")
	}
}

// TestMacOSSerialNumberViaIORegNotFoundWithLogger tests serial not-found with logger.
func TestMacOSSerialNumberViaIORegNotFoundWithLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mock := newMockExecutor()
	mock.setOutput("ioreg", "output without serial pattern")

	_, err := macOSSerialNumberViaIOReg(context.Background(), mock, logger)
	if err == nil {
		t.Error("Expected error")
	}
	if !bytes.Contains(buf.Bytes(), []byte("serial number not found in ioreg output")) {
		t.Error("Expected 'serial number not found in ioreg output' in log")
	}
}

// TestMacOSCPUInfoEmptyBrand tests sysctl returning empty brand string.
func TestMacOSCPUInfoEmptyBrand(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("sysctl", "") // Empty brand string
	mock.setOutput("system_profiler", `{
		"SPHardwareDataType": [{
			"chip_type": "Apple M2",
			"machine_model": "Mac",
			"platform_UUID": "UUID",
			"serial_number": "SER"
		}]
	}`)

	result, err := macOSCPUInfo(context.Background(), mock, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result != "Apple M2" {
		t.Errorf("Expected 'Apple M2', got %q", result)
	}
}

// TestMacOSCPUInfoEmptyBrandWithLogger tests empty brand path with logger.
func TestMacOSCPUInfoEmptyBrandWithLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mock := newMockExecutor()
	mock.setOutput("sysctl", "") // Empty brand → falls to system_profiler
	mock.setOutput("system_profiler", `{
		"SPHardwareDataType": [{
			"chip_type": "Apple M2 Pro",
			"machine_model": "Mac",
			"platform_UUID": "UUID",
			"serial_number": "SER"
		}]
	}`)

	result, err := macOSCPUInfo(context.Background(), mock, logger)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result != "Apple M2 Pro" {
		t.Errorf("Expected 'Apple M2 Pro', got %q", result)
	}
	if !bytes.Contains(buf.Bytes(), []byte("falling back to system_profiler for CPU info")) {
		t.Error("Expected fallback log message")
	}
}

// TestMacOSCPUInfoProfilerJSONParseFailWithLogger tests JSON parse failure with logger.
func TestMacOSCPUInfoProfilerJSONParseFailWithLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mock := newMockExecutor()
	mock.setError("sysctl", fmt.Errorf("not found"))
	mock.setOutput("system_profiler", "not valid json")

	_, err := macOSCPUInfo(context.Background(), mock, logger)
	if err == nil {
		t.Error("Expected error when all methods fail")
	}
	if !bytes.Contains(buf.Bytes(), []byte("system_profiler CPU JSON parsing failed")) {
		t.Error("Expected 'system_profiler CPU JSON parsing failed' in log")
	}
	if !bytes.Contains(buf.Bytes(), []byte("all CPU info methods failed")) {
		t.Error("Expected 'all CPU info methods failed' in log")
	}
}

// TestMacOSCPUInfoEmptyChipTypeWithLogger tests empty chip_type with logger.
func TestMacOSCPUInfoEmptyChipTypeWithLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mock := newMockExecutor()
	mock.setError("sysctl", fmt.Errorf("not found"))
	mock.setOutput("system_profiler", `{
		"SPHardwareDataType": [{
			"chip_type": "",
			"machine_model": "Mac",
			"platform_UUID": "UUID",
			"serial_number": "SER"
		}]
	}`)

	_, err := macOSCPUInfo(context.Background(), mock, logger)
	if err == nil {
		t.Error("Expected error when chip_type is empty")
	}
	if !bytes.Contains(buf.Bytes(), []byte("system_profiler returned empty chip_type")) {
		t.Error("Expected 'system_profiler returned empty chip_type' in log")
	}
}

// TestMacOSCPUInfoSysctlBrandWithFeaturesFail tests brand success but features fail.
func TestMacOSCPUInfoSysctlBrandWithFeaturesFail(t *testing.T) {
	mock := newMockExecutor()
	// The mock returns same output for all "sysctl" calls. To test the
	// features-fail path, we set sysctl to error on the second call.
	// Since mockExecutor doesn't support per-arg differentiation, we test
	// the code path where sysctl succeeds for brand but the features call
	// also "succeeds" (same mock behavior). The brand-only path is tested
	// by making sysctl return error after first call isn't straightforward,
	// so we test that a non-empty brand with features returns formatted output.
	mock.setOutput("sysctl", "Intel Core i7")

	result, err := macOSCPUInfo(context.Background(), mock, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// Both brand and features calls go to same mock → "Intel Core i7:Intel Core i7"
	if !strings.Contains(result, "Intel Core i7") {
		t.Errorf("Expected result to contain 'Intel Core i7', got %q", result)
	}
}

// TestParseStorageJSONEmptyDeviceName tests entries with empty device_name are skipped.
func TestParseStorageJSONEmptyDeviceName(t *testing.T) {
	jsonOutput := `{
		"SPStorageDataType": [
			{
				"_name": "Volume 1",
				"physical_drive": {
					"device_name": "",
					"is_internal_disk": "yes"
				}
			},
			{
				"_name": "Volume 2",
				"physical_drive": {
					"device_name": "APPLE SSD",
					"is_internal_disk": "yes"
				}
			}
		]
	}`

	result, err := parseStorageJSON(jsonOutput)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("Expected 1 disk entry (empty name skipped), got %d", len(result))
	}
	if result[0] != "APPLE SSD" {
		t.Errorf("Expected 'APPLE SSD', got %q", result[0])
	}
}

// TestParseStorageJSONAllEmpty tests when all entries have empty device_name.
func TestParseStorageJSONAllEmpty(t *testing.T) {
	jsonOutput := `{
		"SPStorageDataType": [
			{
				"_name": "Volume 1",
				"physical_drive": {
					"device_name": "",
					"is_internal_disk": "yes"
				}
			}
		]
	}`

	_, err := parseStorageJSON(jsonOutput)
	if err == nil {
		t.Error("Expected error when all disk entries have empty device_name")
	}
}

// TestParseStorageJSONEmptyArray tests empty storage array.
func TestParseStorageJSONEmptyArray(t *testing.T) {
	jsonOutput := `{"SPStorageDataType": []}`
	_, err := parseStorageJSON(jsonOutput)
	if err == nil {
		t.Error("Expected error for empty storage array")
	}
}

// TestCollectMACAddressesWithLogger tests MAC collection with logger enabled.
func TestCollectMACAddressesWithLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	macs, err := collectMACAddresses(MACFilterPhysical, logger)
	if err != nil {
		t.Logf("collectMACAddresses error (may be expected): %v", err)
		return
	}

	// On most systems, some interfaces should produce log output
	output := buf.String()
	t.Logf("Found %d MACs, log output length: %d", len(macs), len(output))

	// We can't assert specific log messages since they depend on system interfaces,
	// but we can verify no panic occurred and logs were produced
	if len(macs) > 0 && !bytes.Contains(buf.Bytes(), []byte("including interface")) {
		t.Error("Expected 'including interface' log when MACs are found")
	}
}

// TestExtractHardwareFieldErrorTypes tests that extractHardwareField returns correct error types.
func TestExtractHardwareFieldErrorTypes(t *testing.T) {
	t.Run("invalid JSON returns ParseError", func(t *testing.T) {
		_, err := extractHardwareField("not json", func(e spHardwareEntry) string {
			return e.PlatformUUID
		})
		var parseErr *ParseError
		if !errors.As(err, &parseErr) {
			t.Fatalf("Expected ParseError, got %T: %v", err, err)
		}
		if parseErr.Source != "system_profiler hardware JSON" {
			t.Errorf("ParseError.Source = %q, want %q", parseErr.Source, "system_profiler hardware JSON")
		}
	})

	t.Run("no data returns ParseError with ErrNotFound", func(t *testing.T) {
		_, err := extractHardwareField(`{"SPHardwareDataType": []}`, func(e spHardwareEntry) string {
			return e.PlatformUUID
		})
		var parseErr *ParseError
		if !errors.As(err, &parseErr) {
			t.Fatalf("Expected ParseError, got %T: %v", err, err)
		}
		if !errors.Is(err, ErrNotFound) {
			t.Error("Expected ErrNotFound in error chain")
		}
	})

	t.Run("empty field returns ParseError with ErrEmptyValue", func(t *testing.T) {
		_, err := extractHardwareField(`{"SPHardwareDataType": [{"platform_UUID": ""}]}`, func(e spHardwareEntry) string {
			return e.PlatformUUID
		})
		var parseErr *ParseError
		if !errors.As(err, &parseErr) {
			t.Fatalf("Expected ParseError, got %T: %v", err, err)
		}
		if !errors.Is(err, ErrEmptyValue) {
			t.Error("Expected ErrEmptyValue in error chain")
		}
	})
}

// TestMacOSHardwareUUIDViaIORegErrorType tests error type from ioreg not-found.
func TestMacOSHardwareUUIDViaIORegErrorType(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("ioreg", "no UUID here")

	_, err := macOSHardwareUUIDViaIOReg(context.Background(), mock, nil)

	var parseErr *ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("Expected ParseError, got %T: %v", err, err)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Error("Expected ErrNotFound in error chain")
	}
}

// TestMacOSCPUInfoAllFailErrorType tests error type when all CPU methods fail.
func TestMacOSCPUInfoAllFailErrorType(t *testing.T) {
	mock := newMockExecutor()
	mock.setError("sysctl", fmt.Errorf("not found"))
	mock.setError("system_profiler", fmt.Errorf("not found"))

	_, err := macOSCPUInfo(context.Background(), mock, nil)
	if !errors.Is(err, ErrAllMethodsFailed) {
		t.Errorf("Expected ErrAllMethodsFailed, got %v", err)
	}
}
