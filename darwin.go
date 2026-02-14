//go:build darwin

package machineid

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// spHardwareDataType represents the JSON output of `system_profiler SPHardwareDataType -json`.
type spHardwareDataType struct {
	SPHardwareDataType []spHardwareEntry `json:"SPHardwareDataType"`
}

type spHardwareEntry struct {
	PlatformUUID string `json:"platform_UUID"`
	SerialNumber string `json:"serial_number"`
	ChipType     string `json:"chip_type"`
	ModelName    string `json:"machine_name"`
	MachineModel string `json:"machine_model"`
}

// spStorageDataType represents the JSON output of `system_profiler SPStorageDataType -json`.
type spStorageDataType struct {
	SPStorageDataType []spStorageEntry `json:"SPStorageDataType"`
}

type spStorageEntry struct {
	Name          string          `json:"_name"`
	BSDName       string          `json:"bsd_name"`
	PhysicalDrive spPhysicalDrive `json:"physical_drive"`
	VolumeUUID    string          `json:"volume_uuid"`
}

type spPhysicalDrive struct {
	DeviceName  string `json:"device_name"`
	IsInternal  string `json:"is_internal_disk"`
	MediaName   string `json:"media_name"`
	MediumType  string `json:"medium_type"`
	Protocol    string `json:"protocol"`
	SmartStatus string `json:"smart_status"`
}

// collectIdentifiers gathers macOS-specific hardware identifiers based on provider config.
func collectIdentifiers(g *Provider, diag *DiagnosticInfo) ([]string, error) {
	var identifiers []string

	if g.includeSystemUUID {
		identifiers = appendIdentifierIfValid(identifiers, func() (string, error) {
			return macOSHardwareUUID(g.commandExecutor)
		}, "uuid:", diag, ComponentSystemUUID)
	}

	if g.includeMotherboard {
		identifiers = appendIdentifierIfValid(identifiers, func() (string, error) {
			return macOSSerialNumber(g.commandExecutor)
		}, "serial:", diag, ComponentMotherboard)
	}

	if g.includeCPU {
		identifiers = appendIdentifierIfValid(identifiers, func() (string, error) {
			return macOSCPUInfo(g.commandExecutor)
		}, "cpu:", diag, ComponentCPU)
	}

	if g.includeMAC {
		identifiers = appendIdentifiersIfValid(identifiers, collectMACAddresses, "mac:", diag, ComponentMAC)
	}

	if g.includeDisk {
		identifiers = appendIdentifiersIfValid(identifiers, func() ([]string, error) {
			return macOSDiskInfo(g.commandExecutor)
		}, "disk:", diag, ComponentDisk)
	}

	return identifiers, nil
}

// macOSHardwareUUID retrieves hardware UUID using system_profiler with JSON parsing.
func macOSHardwareUUID(executor CommandExecutor) (string, error) {
	output, err := executeCommand(executor, "system_profiler", "SPHardwareDataType", "-json")
	if err == nil {
		uuid, parseErr := extractHardwareField(output, func(e spHardwareEntry) string {
			return e.PlatformUUID
		})
		if parseErr == nil {
			return uuid, nil
		}
	}

	// Fallback to ioreg
	return macOSHardwareUUIDViaIOReg(executor)
}

// macOSHardwareUUIDViaIOReg retrieves hardware UUID using ioreg as fallback.
func macOSHardwareUUIDViaIOReg(executor CommandExecutor) (string, error) {
	output, err := executeCommand(executor, "ioreg", "-d2", "-c", "IOPlatformExpertDevice")
	if err != nil {
		return "", fmt.Errorf("failed to get hardware UUID: %w", err)
	}

	re := regexp.MustCompile(`"IOPlatformUUID"\s*=\s*"([^"]+)"`)
	match := re.FindStringSubmatch(output)

	if len(match) > 1 {
		return match[1], nil
	}

	return "", fmt.Errorf("hardware UUID not found in ioreg output")
}

// macOSSerialNumber retrieves system serial number.
func macOSSerialNumber(executor CommandExecutor) (string, error) {
	output, err := executeCommand(executor, "system_profiler", "SPHardwareDataType", "-json")
	if err == nil {
		serial, parseErr := extractHardwareField(output, func(e spHardwareEntry) string {
			return e.SerialNumber
		})
		if parseErr == nil {
			return serial, nil
		}
	}

	// Fallback to ioreg
	return macOSSerialNumberViaIOReg(executor)
}

// macOSSerialNumberViaIOReg retrieves serial number using ioreg as fallback.
func macOSSerialNumberViaIOReg(executor CommandExecutor) (string, error) {
	output, err := executeCommand(executor, "ioreg", "-d2", "-c", "IOPlatformExpertDevice")
	if err != nil {
		return "", fmt.Errorf("failed to get serial number: %w", err)
	}

	re := regexp.MustCompile(`"IOPlatformSerialNumber"\s*=\s*"([^"]+)"`)
	match := re.FindStringSubmatch(output)

	if len(match) > 1 {
		return match[1], nil
	}

	return "", fmt.Errorf("serial number not found in ioreg output")
}

// macOSCPUInfo retrieves CPU information.
// Uses sysctl as primary source (consistent with existing machine IDs).
// On Intel: returns brand_string:features.
// On Apple Silicon: sysctl returns brand_string with empty features,
// producing "ChipType:" — this trailing colon is preserved for backward
// compatibility with existing license activations.
// Falls back to system_profiler chip_type only if sysctl fails entirely.
func macOSCPUInfo(executor CommandExecutor) (string, error) {
	// Primary: sysctl (backward compatible)
	output, err := executeCommand(executor, "sysctl", "-n", "machdep.cpu.brand_string")
	if err == nil {
		cpuBrand := strings.TrimSpace(output)
		if cpuBrand != "" {
			// Get CPU features (populated on Intel, empty on Apple Silicon)
			featOutput, featErr := executeCommand(executor, "sysctl", "-n", "machdep.cpu.features")
			if featErr == nil {
				features := strings.TrimSpace(featOutput)

				return fmt.Sprintf("%s:%s", cpuBrand, features), nil
			}

			return cpuBrand, nil
		}
	}

	// Fallback: system_profiler for Apple Silicon chip type
	profilerOutput, profilerErr := executeCommand(executor, "system_profiler", "SPHardwareDataType", "-json")
	if profilerErr == nil {
		var hw spHardwareDataType
		if jsonErr := json.Unmarshal([]byte(profilerOutput), &hw); jsonErr == nil && len(hw.SPHardwareDataType) > 0 {
			entry := hw.SPHardwareDataType[0]
			if entry.ChipType != "" {
				return entry.ChipType, nil
			}
		}
	}

	return "", fmt.Errorf("failed to get CPU info: all methods failed")
}

// macOSDiskInfo retrieves internal disk device names for stable machine identification.
// It uses system_profiler with JSON output and filters to internal disks only,
// deduplicating across volumes on the same physical disk.
func macOSDiskInfo(executor CommandExecutor) ([]string, error) {
	output, err := executeCommand(executor, "system_profiler", "SPStorageDataType", "-json")
	if err != nil {
		return nil, fmt.Errorf("failed to get disk info: %w", err)
	}

	return parseStorageJSON(output)
}

// parseStorageJSON parses system_profiler SPStorageDataType JSON and extracts
// unique internal disk device names.
func parseStorageJSON(jsonOutput string) ([]string, error) {
	var storage spStorageDataType
	if err := json.Unmarshal([]byte(jsonOutput), &storage); err != nil {
		return nil, fmt.Errorf("failed to parse storage JSON: %w", err)
	}

	// Use a set to deduplicate — multiple volumes can share the same physical disk.
	seen := make(map[string]struct{})
	var diskNames []string

	for _, entry := range storage.SPStorageDataType {
		name := entry.PhysicalDrive.DeviceName
		if name == "" {
			continue
		}

		// Only include internal disks for stability.
		if entry.PhysicalDrive.IsInternal != "yes" {
			continue
		}

		if _, exists := seen[name]; exists {
			continue
		}

		seen[name] = struct{}{}
		diskNames = append(diskNames, name)
	}

	if len(diskNames) == 0 {
		return nil, fmt.Errorf("no internal disk identifiers found")
	}

	return diskNames, nil
}

// extractHardwareField extracts a field from system_profiler SPHardwareDataType JSON output.
func extractHardwareField(jsonOutput string, fieldFn func(spHardwareEntry) string) (string, error) {
	var hw spHardwareDataType
	if err := json.Unmarshal([]byte(jsonOutput), &hw); err != nil {
		return "", fmt.Errorf("failed to parse hardware JSON: %w", err)
	}

	if len(hw.SPHardwareDataType) == 0 {
		return "", fmt.Errorf("no hardware data found in JSON output")
	}

	value := fieldFn(hw.SPHardwareDataType[0])
	if value == "" {
		return "", fmt.Errorf("field is empty in hardware data")
	}

	return value, nil
}
