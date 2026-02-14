//go:build linux

package machineid

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// collectIdentifiers gathers Linux-specific hardware identifiers based on provider config.
func collectIdentifiers(ctx context.Context, p *Provider, diag *DiagnosticInfo) ([]string, error) {
	var identifiers []string
	logger := p.logger

	if p.includeCPU {
		identifiers = appendIdentifierIfValid(identifiers, func() (string, error) {
			return linuxCPUID(logger)
		}, "cpu:", diag, ComponentCPU, logger)
	}

	if p.includeSystemUUID {
		identifiers = appendIdentifierIfValid(identifiers, func() (string, error) {
			return linuxSystemUUID(logger)
		}, "uuid:", diag, ComponentSystemUUID, logger)
		identifiers = appendIdentifierIfValid(identifiers, func() (string, error) {
			return linuxMachineID(logger)
		}, "machine:", diag, ComponentMachineID, logger)
	}

	if p.includeMotherboard {
		identifiers = appendIdentifierIfValid(identifiers, func() (string, error) {
			return linuxMotherboardSerial(logger)
		}, "mb:", diag, ComponentMotherboard, logger)
	}

	if p.includeMAC {
		identifiers = appendIdentifiersIfValid(identifiers, func() ([]string, error) {
			return collectMACAddresses(p.macFilter, logger)
		}, "mac:", diag, ComponentMAC, logger)
	}

	if p.includeDisk {
		identifiers = appendIdentifiersIfValid(identifiers, func() ([]string, error) {
			return linuxDiskSerials(ctx, p.commandExecutor, logger)
		}, "disk:", diag, ComponentDisk, logger)
	}

	return identifiers, nil
}

// linuxCPUID retrieves CPU information from /proc/cpuinfo.
func linuxCPUID(logger *slog.Logger) (string, error) {
	const path = "/proc/cpuinfo"

	data, err := os.ReadFile(path)
	if err != nil {
		if logger != nil {
			logger.Debug("failed to read CPU info", "path", path, "error", err)
		}

		return "", err
	}

	if logger != nil {
		logger.Debug("read CPU info", "path", path)
	}

	return parseCPUInfo(string(data)), nil
}

// parseCPUInfo extracts CPU information from /proc/cpuinfo content.
func parseCPUInfo(content string) string {
	lines := strings.Split(content, "\n")
	var processor, vendorID, modelName, flags string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		switch {
		case strings.HasPrefix(line, "processor"):
			processor = strings.TrimSpace(parts[1])
		case strings.HasPrefix(line, "vendor_id"):
			vendorID = strings.TrimSpace(parts[1])
		case strings.HasPrefix(line, "model name"):
			modelName = strings.TrimSpace(parts[1])
		case strings.HasPrefix(line, "flags"):
			flags = strings.TrimSpace(parts[1])
		}
	}

	// Combine CPU information for unique identifier
	return fmt.Sprintf("%s:%s:%s:%s", processor, vendorID, modelName, flags)
}

// linuxSystemUUID retrieves system UUID from DMI.
func linuxSystemUUID(logger *slog.Logger) (string, error) {
	// Try multiple locations for system UUID
	locations := []string{
		"/sys/class/dmi/id/product_uuid",
		"/sys/devices/virtual/dmi/id/product_uuid",
	}

	return readFirstValidFromLocations(locations, isValidUUID, logger)
}

// linuxMotherboardSerial retrieves motherboard serial number from DMI.
func linuxMotherboardSerial(logger *slog.Logger) (string, error) {
	locations := []string{
		"/sys/class/dmi/id/board_serial",
		"/sys/devices/virtual/dmi/id/board_serial",
	}

	return readFirstValidFromLocations(locations, isValidSerial, logger)
}

// linuxMachineID retrieves systemd machine ID.
func linuxMachineID(logger *slog.Logger) (string, error) {
	locations := []string{
		"/etc/machine-id",
		"/var/lib/dbus/machine-id",
	}

	return readFirstValidFromLocations(locations, isNonEmpty, logger)
}

// readFirstValidFromLocations reads from multiple locations until a valid value is found.
func readFirstValidFromLocations(locations []string, validator func(string) bool, logger *slog.Logger) (string, error) {
	for _, location := range locations {
		data, err := os.ReadFile(location)
		if err == nil {
			value := strings.TrimSpace(string(data))
			if validator(value) {
				if logger != nil {
					logger.Debug("read value from file", "path", location)
				}

				return value, nil
			}

			if logger != nil {
				logger.Debug("file value failed validation", "path", location)
			}
		} else if logger != nil {
			logger.Debug("failed to read file", "path", location, "error", err)
		}
	}

	return "", ErrNotFound
}

// isValidUUID reports whether the UUID is valid (not empty or null).
func isValidUUID(uuid string) bool {
	return uuid != "" && uuid != "00000000-0000-0000-0000-000000000000"
}

// isValidSerial reports whether the serial is valid (not empty or placeholder).
func isValidSerial(serial string) bool {
	return serial != "" && serial != biosFirmwareMessage
}

// isNonEmpty reports whether the value is not empty.
func isNonEmpty(value string) bool {
	return value != ""
}

// linuxDiskSerials retrieves disk serial numbers using various methods.
// Results are deduplicated across sources to prevent the same serial
// from appearing multiple times.
func linuxDiskSerials(ctx context.Context, executor CommandExecutor, logger *slog.Logger) ([]string, error) {
	seen := make(map[string]struct{})
	var serials []string

	// Try using lsblk command first
	if lsblkSerials, err := linuxDiskSerialsLSBLK(ctx, executor, logger); err == nil {
		for _, s := range lsblkSerials {
			if _, exists := seen[s]; !exists {
				seen[s] = struct{}{}
				serials = append(serials, s)
			}
		}

		if logger != nil {
			logger.Debug("collected disk serials via lsblk", "count", len(lsblkSerials))
		}
	} else if logger != nil {
		logger.Debug("lsblk failed, trying /sys/block", "error", err)
	}

	// Try reading from /sys/block
	if sysSerials, err := linuxDiskSerialsSys(logger); err == nil {
		for _, s := range sysSerials {
			if _, exists := seen[s]; !exists {
				seen[s] = struct{}{}
				serials = append(serials, s)
			}
		}

		if logger != nil {
			logger.Debug("collected disk serials via /sys/block", "count", len(sysSerials))
		}
	} else if logger != nil {
		logger.Debug("/sys/block read failed", "error", err)
	}

	return serials, nil
}

// linuxDiskSerialsLSBLK retrieves disk serials using lsblk command.
func linuxDiskSerialsLSBLK(ctx context.Context, executor CommandExecutor, logger *slog.Logger) ([]string, error) {
	output, err := executeCommand(ctx, executor, logger, "lsblk", "-d", "-n", "-o", "SERIAL")
	if err != nil {
		return nil, err
	}

	var serials []string
	lines := strings.SplitSeq(output, "\n")
	for line := range lines {
		serial := strings.TrimSpace(line)
		if serial != "" {
			serials = append(serials, serial)
		}
	}

	return serials, nil
}

// linuxDiskSerialsSys retrieves disk serials from /sys/block.
func linuxDiskSerialsSys(logger *slog.Logger) ([]string, error) {
	var serials []string

	blockDir := "/sys/block"
	entries, err := os.ReadDir(blockDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), "loop") {
			serialFile := filepath.Join(blockDir, entry.Name(), "device", "serial")
			if data, err := os.ReadFile(serialFile); err == nil {
				serial := strings.TrimSpace(string(data))
				if serial != "" {
					serials = append(serials, serial)

					if logger != nil {
						logger.Debug("read disk serial from sysfs", "disk", entry.Name(), "path", serialFile)
					}
				}
			}
		}
	}

	return serials, nil
}
