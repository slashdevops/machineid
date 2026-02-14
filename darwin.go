//go:build darwin

package machineid

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
)

// Compiled regexes for ioreg output parsing.
var (
	ioregUUIDRe   = regexp.MustCompile(`"IOPlatformUUID"\s*=\s*"([^"]+)"`)
	ioregSerialRe = regexp.MustCompile(`"IOPlatformSerialNumber"\s*=\s*"([^"]+)"`)
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
func collectIdentifiers(ctx context.Context, p *Provider, diag *DiagnosticInfo) ([]string, error) {
	var identifiers []string
	logger := p.logger

	if p.includeSystemUUID {
		identifiers = appendIdentifierIfValid(identifiers, func() (string, error) {
			return macOSHardwareUUID(ctx, p.commandExecutor, logger)
		}, "uuid:", diag, ComponentSystemUUID, logger)
	}

	if p.includeMotherboard {
		identifiers = appendIdentifierIfValid(identifiers, func() (string, error) {
			return macOSSerialNumber(ctx, p.commandExecutor, logger)
		}, "serial:", diag, ComponentMotherboard, logger)
	}

	if p.includeCPU {
		identifiers = appendIdentifierIfValid(identifiers, func() (string, error) {
			return macOSCPUInfo(ctx, p.commandExecutor, logger)
		}, "cpu:", diag, ComponentCPU, logger)
	}

	if p.includeMAC {
		identifiers = appendIdentifiersIfValid(identifiers, func() ([]string, error) {
			return collectMACAddresses(p.macFilter, logger)
		}, "mac:", diag, ComponentMAC, logger)
	}

	if p.includeDisk {
		identifiers = appendIdentifiersIfValid(identifiers, func() ([]string, error) {
			return macOSDiskInfo(ctx, p.commandExecutor, logger)
		}, "disk:", diag, ComponentDisk, logger)
	}

	return identifiers, nil
}

// macOSHardwareUUID retrieves hardware UUID using system_profiler with JSON parsing.
func macOSHardwareUUID(ctx context.Context, executor CommandExecutor, logger *slog.Logger) (string, error) {
	output, err := executeCommand(ctx, executor, logger, "system_profiler", "SPHardwareDataType", "-json")
	if err == nil {
		uuid, parseErr := extractHardwareField(output, func(e spHardwareEntry) string {
			return e.PlatformUUID
		})
		if parseErr == nil {
			return uuid, nil
		}

		if logger != nil {
			logger.Debug("system_profiler UUID parsing failed", "error", parseErr)
		}
	}

	// Fallback to ioreg
	if logger != nil {
		logger.Info("falling back to ioreg for hardware UUID")
	}

	return macOSHardwareUUIDViaIOReg(ctx, executor, logger)
}

// macOSHardwareUUIDViaIOReg retrieves hardware UUID using ioreg as fallback.
func macOSHardwareUUIDViaIOReg(ctx context.Context, executor CommandExecutor, logger *slog.Logger) (string, error) {
	output, err := executeCommand(ctx, executor, logger, "ioreg", "-d2", "-c", "IOPlatformExpertDevice")
	if err != nil {
		return "", err
	}

	match := ioregUUIDRe.FindStringSubmatch(output)
	if len(match) > 1 {
		return match[1], nil
	}

	if logger != nil {
		logger.Debug("hardware UUID not found in ioreg output")
	}

	return "", &ParseError{Source: "ioreg output", Err: ErrNotFound}
}

// macOSSerialNumber retrieves system serial number.
func macOSSerialNumber(ctx context.Context, executor CommandExecutor, logger *slog.Logger) (string, error) {
	output, err := executeCommand(ctx, executor, logger, "system_profiler", "SPHardwareDataType", "-json")
	if err == nil {
		serial, parseErr := extractHardwareField(output, func(e spHardwareEntry) string {
			return e.SerialNumber
		})
		if parseErr == nil {
			return serial, nil
		}

		if logger != nil {
			logger.Debug("system_profiler serial parsing failed", "error", parseErr)
		}
	}

	// Fallback to ioreg
	if logger != nil {
		logger.Info("falling back to ioreg for serial number")
	}

	return macOSSerialNumberViaIOReg(ctx, executor, logger)
}

// macOSSerialNumberViaIOReg retrieves serial number using ioreg as fallback.
func macOSSerialNumberViaIOReg(ctx context.Context, executor CommandExecutor, logger *slog.Logger) (string, error) {
	output, err := executeCommand(ctx, executor, logger, "ioreg", "-d2", "-c", "IOPlatformExpertDevice")
	if err != nil {
		return "", err
	}

	match := ioregSerialRe.FindStringSubmatch(output)
	if len(match) > 1 {
		return match[1], nil
	}

	if logger != nil {
		logger.Debug("serial number not found in ioreg output")
	}

	return "", &ParseError{Source: "ioreg output", Err: ErrNotFound}
}

// macOSCPUInfo retrieves CPU information.
// Uses sysctl as primary source (consistent with existing machine IDs).
// On Intel: returns brand_string:features.
// On Apple Silicon: sysctl returns brand_string with empty features,
// producing "ChipType:" — this trailing colon is preserved for backward
// compatibility with existing license activations.
// Falls back to system_profiler chip_type only if sysctl fails entirely.
func macOSCPUInfo(ctx context.Context, executor CommandExecutor, logger *slog.Logger) (string, error) {
	// Primary: sysctl (backward compatible)
	output, err := executeCommand(ctx, executor, logger, "sysctl", "-n", "machdep.cpu.brand_string")
	if err == nil {
		cpuBrand := strings.TrimSpace(output)
		if cpuBrand != "" {
			// Get CPU features (populated on Intel, empty on Apple Silicon)
			featOutput, featErr := executeCommand(ctx, executor, logger, "sysctl", "-n", "machdep.cpu.features")
			if featErr == nil {
				features := strings.TrimSpace(featOutput)

				return fmt.Sprintf("%s:%s", cpuBrand, features), nil
			}

			return cpuBrand, nil
		}
	}

	// Fallback: system_profiler for Apple Silicon chip type
	if logger != nil {
		logger.Info("falling back to system_profiler for CPU info")
	}

	profilerOutput, profilerErr := executeCommand(ctx, executor, logger, "system_profiler", "SPHardwareDataType", "-json")
	if profilerErr == nil {
		var hw spHardwareDataType
		if jsonErr := json.Unmarshal([]byte(profilerOutput), &hw); jsonErr == nil && len(hw.SPHardwareDataType) > 0 {
			entry := hw.SPHardwareDataType[0]
			if entry.ChipType != "" {
				return entry.ChipType, nil
			}

			if logger != nil {
				logger.Debug("system_profiler returned empty chip_type")
			}
		} else if logger != nil {
			logger.Debug("system_profiler CPU JSON parsing failed", "error", jsonErr)
		}
	}

	if logger != nil {
		logger.Warn("all CPU info methods failed")
	}

	return "", ErrAllMethodsFailed
}

// macOSDiskInfo retrieves internal disk device names for stable machine identification.
// It uses system_profiler with JSON output and filters to internal disks only,
// deduplicating across volumes on the same physical disk.
func macOSDiskInfo(ctx context.Context, executor CommandExecutor, logger *slog.Logger) ([]string, error) {
	output, err := executeCommand(ctx, executor, logger, "system_profiler", "SPStorageDataType", "-json")
	if err != nil {
		return nil, err
	}

	return parseStorageJSON(output)
}

// parseStorageJSON parses system_profiler SPStorageDataType JSON and extracts
// unique internal disk device names.
func parseStorageJSON(jsonOutput string) ([]string, error) {
	var storage spStorageDataType
	if err := json.Unmarshal([]byte(jsonOutput), &storage); err != nil {
		return nil, &ParseError{Source: "system_profiler storage JSON", Err: err}
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
		return nil, &ParseError{Source: "system_profiler storage output", Err: ErrNotFound}
	}

	return diskNames, nil
}

// extractHardwareField extracts a field from system_profiler SPHardwareDataType JSON output.
func extractHardwareField(jsonOutput string, fieldFn func(spHardwareEntry) string) (string, error) {
	var hw spHardwareDataType
	if err := json.Unmarshal([]byte(jsonOutput), &hw); err != nil {
		return "", &ParseError{Source: "system_profiler hardware JSON", Err: err}
	}

	if len(hw.SPHardwareDataType) == 0 {
		return "", &ParseError{Source: "system_profiler hardware JSON", Err: ErrNotFound}
	}

	value := fieldFn(hw.SPHardwareDataType[0])
	if value == "" {
		return "", &ParseError{Source: "system_profiler hardware JSON", Err: ErrEmptyValue}
	}

	return value, nil
}
