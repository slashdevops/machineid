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

// collectIdentifiers gathers Linux-specific hardware identifiers concurrently.
// Most sources are sysfs and procfs reads; running them alongside the lsblk
// process keeps the disk lookup off the critical path.
func collectIdentifiers(ctx context.Context, p *Provider, diag *DiagnosticInfo) ([]string, error) {
	logger := p.logger
	executor := p.commandExecutor

	var tasks []componentTask

	if p.includeCPU {
		tasks = append(tasks, componentTask{component: ComponentCPU, prefix: "cpu:",
			single: func(context.Context) (string, error) {
				return linuxCPUID(logger)
			}})
	}

	if p.includeSystemUUID {
		tasks = append(tasks,
			componentTask{component: ComponentSystemUUID, prefix: "uuid:",
				single: func(context.Context) (string, error) {
					return linuxSystemUUID(logger)
				}},
			componentTask{component: ComponentMachineID, prefix: "machine:",
				single: func(context.Context) (string, error) {
					return linuxMachineID(logger)
				}},
		)
	}

	if p.includeMotherboard {
		tasks = append(tasks, componentTask{component: ComponentMotherboard, prefix: "mb:",
			single: func(context.Context) (string, error) {
				return linuxMotherboardSerial(logger)
			}})
	}

	if p.includeMAC {
		tasks = append(tasks, componentTask{component: ComponentMAC, prefix: "mac:",
			multi: func(context.Context) ([]string, error) {
				return collectMACAddresses(p.macFilter, logger)
			}})
	}

	if p.includeDisk {
		tasks = append(tasks, componentTask{component: ComponentDisk, prefix: "disk:",
			multi: func(ctx context.Context) ([]string, error) {
				return linuxDiskSerials(ctx, executor, logger)
			}})
	}

	return runComponentTasks(ctx, tasks, diag, logger), nil
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

	return parseCPUInfo(string(data))
}

// parseCPUInfo extracts CPU information from /proc/cpuinfo content.
// Returns ErrNotFound when none of the expected fields are present, so an
// empty or malformed /proc/cpuinfo does not silently contribute a fixed
// all-colons string to the machine ID.
func parseCPUInfo(content string) (string, error) {
	lines := strings.Split(content, "\n")
	var processor, vendorID, modelName, flags string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		_, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)

		switch {
		case strings.HasPrefix(line, "processor"):
			processor = value
		case strings.HasPrefix(line, "vendor_id"):
			vendorID = value
		case strings.HasPrefix(line, "model name"):
			modelName = value
		case strings.HasPrefix(line, "flags"):
			flags = value
		}
	}

	if processor == "" && vendorID == "" && modelName == "" && flags == "" {
		return "", &ParseError{Source: "/proc/cpuinfo", Err: ErrNotFound}
	}

	return fmt.Sprintf("%s:%s:%s:%s", processor, vendorID, modelName, flags), nil
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

// isValidSerial reports whether the serial is valid (not empty or placeholder).
func isValidSerial(serial string) bool {
	return serial != "" && serial != biosFirmwareMessage
}

// isNonEmpty reports whether the value is not empty.
func isNonEmpty(value string) bool {
	return value != ""
}

// sysBlockDir is the root directory scanned by linuxDiskSerialsSys.
// It is a variable so tests can inject a fake /sys/block tree.
var sysBlockDir = "/sys/block"

// linuxDiskSerials retrieves disk serial numbers using various methods.
// Results are deduplicated across sources to prevent the same serial
// from appearing multiple times. OEM placeholder strings (see
// biosFirmwareMessage) are filtered out.
// Returns ErrNotFound when both backends fail and no valid serial was found.
func linuxDiskSerials(ctx context.Context, executor CommandExecutor, logger *slog.Logger) ([]string, error) {
	seen := make(map[string]struct{})
	var serials []string

	appendValid := func(src []string) {
		for _, s := range src {
			if !isValidSerial(s) {
				continue
			}
			if _, exists := seen[s]; exists {
				continue
			}
			seen[s] = struct{}{}
			serials = append(serials, s)
		}
	}

	lsblkErr := error(nil)
	sysErr := error(nil)

	// Try using lsblk command first
	if lsblkSerials, err := linuxDiskSerialsLSBLK(ctx, executor, logger); err == nil {
		appendValid(lsblkSerials)

		if logger != nil {
			logger.Debug("collected disk serials via lsblk", "count", len(lsblkSerials))
		}
	} else {
		lsblkErr = err
		if logger != nil {
			logger.Debug("lsblk failed, trying /sys/block", "error", err)
		}
	}

	// Try reading from /sys/block
	if sysSerials, err := linuxDiskSerialsSys(logger); err == nil {
		appendValid(sysSerials)

		if logger != nil {
			logger.Debug("collected disk serials via /sys/block", "count", len(sysSerials))
		}
	} else {
		sysErr = err
		if logger != nil {
			logger.Debug("/sys/block read failed", "error", err)
		}
	}

	if len(serials) == 0 && lsblkErr != nil && sysErr != nil {
		return nil, ErrNotFound
	}

	return serials, nil
}

// linuxDiskSerialsLSBLK retrieves disk serials using lsblk command.
// OEM placeholder strings are filtered out.
func linuxDiskSerialsLSBLK(ctx context.Context, executor CommandExecutor, logger *slog.Logger) ([]string, error) {
	output, err := executeCommand(ctx, executor, logger, "lsblk", "-d", "-n", "-o", "SERIAL")
	if err != nil {
		return nil, err
	}

	var serials []string
	lines := strings.SplitSeq(output, "\n")
	for line := range lines {
		serial := strings.TrimSpace(line)
		if !isValidSerial(serial) {
			continue
		}
		serials = append(serials, serial)
	}

	return serials, nil
}

// linuxDiskSerialsSys retrieves disk serials from /sys/block.
// OEM placeholder strings are filtered out.
func linuxDiskSerialsSys(logger *slog.Logger) ([]string, error) {
	var serials []string

	entries, err := os.ReadDir(sysBlockDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), "loop") {
			serialFile := filepath.Join(sysBlockDir, entry.Name(), "device", "serial")
			if data, err := os.ReadFile(serialFile); err == nil {
				serial := strings.TrimSpace(string(data))
				if !isValidSerial(serial) {
					continue
				}
				serials = append(serials, serial)

				if logger != nil {
					logger.Debug("read disk serial from sysfs", "disk", entry.Name(), "path", serialFile)
				}
			}
		}
	}

	return serials, nil
}
