//go:build linux

package machineid

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// collectIdentifiers gathers Linux-specific hardware identifiers based on provider config.
func collectIdentifiers(g *Provider, diag *DiagnosticInfo) ([]string, error) {
	var identifiers []string

	if g.includeCPU {
		identifiers = appendIdentifierIfValid(identifiers, linuxCPUID, "cpu:", diag, ComponentCPU)
	}

	if g.includeSystemUUID {
		identifiers = appendIdentifierIfValid(identifiers, linuxSystemUUID, "uuid:", diag, ComponentSystemUUID)
		identifiers = appendIdentifierIfValid(identifiers, linuxMachineID, "machine:", diag, ComponentMachineID)
	}

	if g.includeMotherboard {
		identifiers = appendIdentifierIfValid(identifiers, linuxMotherboardSerial, "mb:", diag, ComponentMotherboard)
	}

	if g.includeMAC {
		identifiers = appendIdentifiersIfValid(identifiers, collectMACAddresses, "mac:", diag, ComponentMAC)
	}

	if g.includeDisk {
		identifiers = appendIdentifiersIfValid(identifiers, func() ([]string, error) {
			return linuxDiskSerials(g.commandExecutor)
		}, "disk:", diag, ComponentDisk)
	}

	return identifiers, nil
}

// linuxCPUID retrieves CPU information from /proc/cpuinfo
func linuxCPUID() (string, error) {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return "", err
	}

	return parseCPUInfo(string(data)), nil
}

// parseCPUInfo extracts CPU information from /proc/cpuinfo content
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

// linuxSystemUUID retrieves system UUID from DMI
func linuxSystemUUID() (string, error) {
	// Try multiple locations for system UUID
	locations := []string{
		"/sys/class/dmi/id/product_uuid",
		"/sys/devices/virtual/dmi/id/product_uuid",
	}

	return readFirstValidFromLocations(locations, isValidUUID)
}

// linuxMotherboardSerial retrieves motherboard serial number from DMI
func linuxMotherboardSerial() (string, error) {
	locations := []string{
		"/sys/class/dmi/id/board_serial",
		"/sys/devices/virtual/dmi/id/board_serial",
	}

	return readFirstValidFromLocations(locations, isValidSerial)
}

// linuxMachineID retrieves systemd machine ID
func linuxMachineID() (string, error) {
	locations := []string{
		"/etc/machine-id",
		"/var/lib/dbus/machine-id",
	}

	return readFirstValidFromLocations(locations, isNonEmpty)
}

// readFirstValidFromLocations reads from multiple locations until valid value found
func readFirstValidFromLocations(locations []string, validator func(string) bool) (string, error) {
	for _, location := range locations {
		data, err := os.ReadFile(location)
		if err == nil {
			value := strings.TrimSpace(string(data))
			if validator(value) {
				return value, nil
			}
		}
	}

	return "", fmt.Errorf("valid value not found in any location")
}

// isValidUUID checks if UUID is valid (not empty or null)
func isValidUUID(uuid string) bool {
	if uuid == "" || uuid == "00000000-0000-0000-0000-000000000000" {
		return false
	}

	return true
}

// isValidSerial checks if serial is valid (not empty or placeholder)
func isValidSerial(serial string) bool {
	if serial == "" || serial == biosFirmwareMessage {
		return false
	}

	return true
}

// isNonEmpty checks if value is not empty
func isNonEmpty(value string) bool {
	return value != ""
}

// linuxDiskSerials retrieves disk serial numbers using various methods.
// Results are deduplicated across sources to prevent the same serial
// from appearing multiple times.
func linuxDiskSerials(executor CommandExecutor) ([]string, error) {
	seen := make(map[string]struct{})
	var serials []string

	// Try using lsblk command first
	if lsblkSerials, err := linuxDiskSerialsLSBLK(executor); err == nil {
		for _, s := range lsblkSerials {
			if _, exists := seen[s]; !exists {
				seen[s] = struct{}{}
				serials = append(serials, s)
			}
		}
	}

	// Try reading from /sys/block
	if sysSerials, err := linuxDiskSerialsSys(); err == nil {
		for _, s := range sysSerials {
			if _, exists := seen[s]; !exists {
				seen[s] = struct{}{}
				serials = append(serials, s)
			}
		}
	}

	return serials, nil
}

// linuxDiskSerialsLSBLK retrieves disk serials using lsblk command.
func linuxDiskSerialsLSBLK(executor CommandExecutor) ([]string, error) {
	output, err := executeCommand(executor, "lsblk", "-d", "-n", "-o", "SERIAL")
	if err != nil {
		return nil, fmt.Errorf("failed to get disk serials: %w", err)
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

// linuxDiskSerialsSys retrieves disk serials from /sys/block
func linuxDiskSerialsSys() ([]string, error) {
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
				}
			}
		}
	}

	return serials, nil
}
