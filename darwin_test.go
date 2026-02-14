//go:build darwin

package machineid

import (
	"context"
	"fmt"
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

	_, err := macOSHardwareUUIDViaIOReg(context.Background(), mock)
	if err == nil {
		t.Error("Expected error when UUID not found in ioreg output")
	}
}

// TestMacOSHardwareUUIDViaIORegError tests ioreg command error.
func TestMacOSHardwareUUIDViaIORegError(t *testing.T) {
	mock := newMockExecutor()
	mock.setError("ioreg", fmt.Errorf("command failed"))

	_, err := macOSHardwareUUIDViaIOReg(context.Background(), mock)
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

	result, err := macOSHardwareUUIDViaIOReg(context.Background(), mock)
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

	_, err := macOSSerialNumberViaIOReg(context.Background(), mock)
	if err == nil {
		t.Error("Expected error when ioreg command fails")
	}
}

// TestMacOSSerialNumberViaIORegNotFound tests when serial not in output.
func TestMacOSSerialNumberViaIORegNotFound(t *testing.T) {
	mock := newMockExecutor()
	mock.setOutput("ioreg", "output without serial")

	_, err := macOSSerialNumberViaIOReg(context.Background(), mock)
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

	result, err := macOSSerialNumberViaIOReg(context.Background(), mock)
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

	result, err := macOSSerialNumber(context.Background(), mock)
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

	_, err := macOSCPUInfo(context.Background(), mock)
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

	result, err := macOSCPUInfo(context.Background(), mock)
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

	result, err := macOSCPUInfo(context.Background(), mock)
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

	_, err := macOSCPUInfo(context.Background(), mock)
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

	_, err := macOSDiskInfo(context.Background(), mock)
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

	result, err := macOSDiskInfo(context.Background(), mock)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("Expected 1 disk entry, got %d", len(result))
	}
}
