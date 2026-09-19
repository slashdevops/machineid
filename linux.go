//go:build linux

package machineid

import (
	"context"
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

// parseCPUInfo extracts a stable CPU identifier from /proc/cpuinfo content.
//
// The identifier is "vendor:model[:hardware]", built from the first processor
// entry. It deliberately excludes the "flags" line, which gains entries after
// kernel and microcode updates, and the processor index, which changes when a
// VM is resized; both used to rotate the machine ID on routine maintenance.
//
// x86 kernels provide "vendor_id" and "model name". ARM kernels provide
// "CPU implementer", "CPU part", "CPU variant" and "CPU revision" instead,
// often with a "Hardware" line naming the board; those are used when the x86
// fields are absent.
//
// Returns ErrNotFound when none of the expected fields are present, so an
// empty or malformed /proc/cpuinfo does not silently contribute a fixed
// string to the machine ID.
func parseCPUInfo(content string) (string, error) {
	fields := map[string]string{}

	for line := range strings.SplitSeq(content, "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		// First occurrence wins: every core repeats the same values.
		if _, seen := fields[key]; !seen {
			fields[key] = value
		}
	}

	vendor := fields["vendor_id"]
	if vendor == "" {
		vendor = fields["cpu implementer"]
	}

	model := fields["model name"]
	if model == "" {
		var parts []string
		for _, key := range []string{"cpu part", "cpu variant", "cpu revision"} {
			if v := fields[key]; v != "" {
				parts = append(parts, v)
			}
		}
		model = strings.Join(parts, "/")
	}

	if vendor == "" && model == "" {
		return "", &ParseError{Source: "/proc/cpuinfo", Err: ErrNotFound}
	}

	id := vendor + ":" + model
	if hw := fields["hardware"]; hw != "" {
		id += ":" + hw
	}

	return id, nil
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

// linuxDiskSerialsLSBLK retrieves the serials of fixed disks using lsblk.
//
// Removable devices (USB sticks, SD cards) and non-disk devices (optical
// drives, loop devices) are excluded: plugging one in must not change the
// machine ID. OEM placeholder strings are filtered out.
func linuxDiskSerialsLSBLK(ctx context.Context, executor CommandExecutor, logger *slog.Logger) ([]string, error) {
	output, err := executeCommand(ctx, executor, logger, "lsblk", "-d", "-n", "-P", "-o", "NAME,TYPE,RM,SERIAL")
	if err != nil {
		return nil, err
	}

	var serials []string
	for line := range strings.SplitSeq(output, "\n") {
		dev := parseKeyValueLine(line)
		if len(dev) == 0 {
			continue
		}

		if dev["TYPE"] != "disk" || dev["RM"] == "1" {
			if logger != nil {
				logger.Debug("skipping block device", "name", dev["NAME"], "type", dev["TYPE"], "removable", dev["RM"])
			}

			continue
		}

		if serial := dev["SERIAL"]; isValidSerial(serial) {
			serials = append(serials, serial)
		}
	}

	return serials, nil
}

// parseKeyValueLine parses lsblk -P output: KEY="value" pairs separated by spaces.
func parseKeyValueLine(line string) map[string]string {
	fields := map[string]string{}
	rest := strings.TrimSpace(line)

	for rest != "" {
		key, after, ok := strings.Cut(rest, "=\"")
		if !ok {
			break
		}
		value, remainder, ok := strings.Cut(after, "\"")
		if !ok {
			break
		}
		fields[strings.TrimSpace(key)] = value
		rest = strings.TrimSpace(remainder)
	}

	return fields
}

// virtualBlockPrefixes name block devices that are never fixed disks.
var virtualBlockPrefixes = []string{"loop", "ram", "zram", "dm-", "md", "sr", "fd", "nbd", "mtd"}

// isRemovableBlockDevice reports whether /sys/block/<name>/removable is set.
func isRemovableBlockDevice(name string) bool {
	data, err := os.ReadFile(filepath.Join(sysBlockDir, name, "removable"))

	return err == nil && strings.TrimSpace(string(data)) == "1"
}

// linuxDiskSerialsSys retrieves the serials of fixed disks from /sys/block.
// Virtual and removable block devices are skipped; OEM placeholder strings
// are filtered out.
func linuxDiskSerialsSys(logger *slog.Logger) ([]string, error) {
	var serials []string

	entries, err := os.ReadDir(sysBlockDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if hasAnyPrefix(name, virtualBlockPrefixes) || isRemovableBlockDevice(name) {
			if logger != nil {
				logger.Debug("skipping block device", "disk", name)
			}

			continue
		}

		serialFile := filepath.Join(sysBlockDir, name, "device", "serial")
		data, err := os.ReadFile(serialFile)
		if err != nil {
			continue
		}

		serial := strings.TrimSpace(string(data))
		if !isValidSerial(serial) {
			continue
		}
		serials = append(serials, serial)

		if logger != nil {
			logger.Debug("read disk serial from sysfs", "disk", name, "path", serialFile)
		}
	}

	return serials, nil
}
