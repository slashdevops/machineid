//go:build windows

package machineid

import (
	"context"
	"log/slog"
	"os/exec"
	"strings"
)

// lookPath resolves a command on PATH. It is a variable so tests can force
// the wmic probe to a known outcome regardless of the host.
var lookPath = exec.LookPath

// powerShellArgs precede every PowerShell script. -NoProfile keeps user
// profiles from writing to stdout (which would corrupt parsed values) and
// from slowing start-up; -NonInteractive prevents prompts from blocking.
var powerShellArgs = []string{"-NoProfile", "-NonInteractive", "-Command"}

// collectIdentifiers gathers Windows-specific hardware identifiers concurrently.
// Windows commands (wmic, PowerShell) are slow due to process startup overhead,
// so all components are collected in parallel to minimize total latency.
func collectIdentifiers(ctx context.Context, p *Provider, diag *DiagnosticInfo) ([]string, error) {
	logger := p.logger
	executor := p.commandExecutor

	var tasks []componentTask

	if p.includeCPU {
		tasks = append(tasks, componentTask{component: ComponentCPU, prefix: "cpu:",
			single: func(ctx context.Context) (string, error) {
				return windowsCPUID(ctx, executor, logger)
			}})
	}

	if p.includeMotherboard {
		tasks = append(tasks, componentTask{component: ComponentMotherboard, prefix: "mb:",
			single: func(ctx context.Context) (string, error) {
				return windowsMotherboardSerial(ctx, executor, logger)
			}})
	}

	if p.includeSystemUUID {
		tasks = append(tasks, componentTask{component: ComponentSystemUUID, prefix: "uuid:",
			single: func(ctx context.Context) (string, error) {
				return windowsSystemUUID(ctx, executor, logger)
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
				return windowsDiskSerials(ctx, executor, logger)
			}})
	}

	return runComponentTasks(ctx, tasks, diag, logger), nil
}

// wmicAvailable reports whether wmic is on PATH. wmic was removed from
// Windows 11 24H2 and Windows Server 2025, so skipping it avoids paying for
// a failed process spawn before every PowerShell fallback.
func wmicAvailable(logger *slog.Logger) bool {
	if _, err := lookPath("wmic"); err != nil {
		if logger != nil {
			logger.Debug("wmic not found, using PowerShell", "error", err)
		}

		return false
	}

	return true
}

// runPowerShell executes a PowerShell script with the standard non-interactive flags.
func runPowerShell(ctx context.Context, executor CommandExecutor, logger *slog.Logger, script string) (string, error) {
	args := append(append([]string{}, powerShellArgs...), script)

	return executeCommand(ctx, executor, logger, "powershell", args...)
}

// parseWmicValue extracts value from wmic output with given prefix.
func parseWmicValue(output, prefix string) (string, error) {
	lines := strings.SplitSeq(output, "\n")

	for line := range lines {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, prefix); ok {
			value := strings.TrimSpace(rest)
			if value == "" || value == biosFirmwareMessage {
				continue
			}

			return value, nil
		}
	}

	return "", &ParseError{Source: "wmic output", Err: ErrNotFound}
}

// parseWmicMultipleValues extracts all values from wmic output with given prefix.
func parseWmicMultipleValues(output, prefix string) []string {
	var values []string
	lines := strings.SplitSeq(output, "\n")

	for line := range lines {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, prefix); ok {
			value := strings.TrimSpace(rest)
			if value == "" || value == biosFirmwareMessage {
				continue
			}
			values = append(values, value)
		}
	}

	return values
}

// parsePowerShellValue extracts a trimmed, non-empty value from PowerShell output.
// OEM placeholder strings (see biosFirmwareMessage) are rejected with ErrOEMPlaceholder.
func parsePowerShellValue(output string) (string, error) {
	value := strings.TrimSpace(output)
	if value == "" {
		return "", &ParseError{Source: "PowerShell output", Err: ErrEmptyValue}
	}
	if value == biosFirmwareMessage {
		return "", &ParseError{Source: "PowerShell output", Err: ErrOEMPlaceholder}
	}

	return value, nil
}

// parsePowerShellMultipleValues extracts multiple trimmed, non-empty values from PowerShell output.
// OEM placeholder lines (see biosFirmwareMessage) are filtered out.
func parsePowerShellMultipleValues(output string) []string {
	var values []string
	lines := strings.SplitSeq(output, "\n")

	for line := range lines {
		value := strings.TrimSpace(line)
		if value == "" || value == biosFirmwareMessage {
			continue
		}
		values = append(values, value)
	}

	return values
}

// windowsCPUID retrieves CPU processor ID using wmic, with PowerShell fallback.
func windowsCPUID(ctx context.Context, executor CommandExecutor, logger *slog.Logger) (string, error) {
	if wmicAvailable(logger) {
		output, err := executeCommand(ctx, executor, logger, "wmic", "cpu", "get", "ProcessorId", "/value")
		if err == nil {
			if value, parseErr := parseWmicValue(output, "ProcessorId="); parseErr == nil {
				return value, nil
			} else if logger != nil {
				logger.Debug("wmic CPU ID parsing failed", "error", parseErr)
			}
		}
	}

	// Fallback to PowerShell Get-CimInstance
	if logger != nil {
		logger.Info("falling back to PowerShell for CPU ID")
	}

	psOutput, psErr := runPowerShell(ctx, executor, logger,
		"Get-CimInstance -ClassName Win32_Processor | Select-Object -ExpandProperty ProcessorId")
	if psErr != nil {
		if logger != nil {
			logger.Warn("all CPU ID methods failed")
		}

		return "", ErrAllMethodsFailed
	}

	return parsePowerShellValue(psOutput)
}

// windowsMotherboardSerial retrieves motherboard serial number using wmic, with PowerShell fallback.
func windowsMotherboardSerial(ctx context.Context, executor CommandExecutor, logger *slog.Logger) (string, error) {
	if wmicAvailable(logger) {
		output, err := executeCommand(ctx, executor, logger, "wmic", "baseboard", "get", "SerialNumber", "/value")
		if err == nil {
			if value, parseErr := parseWmicValue(output, "SerialNumber="); parseErr == nil {
				return value, nil
			} else if logger != nil {
				logger.Debug("wmic motherboard serial parsing failed", "error", parseErr)
			}
		}
	}

	// Fallback to PowerShell Get-CimInstance
	if logger != nil {
		logger.Info("falling back to PowerShell for motherboard serial")
	}

	psOutput, psErr := runPowerShell(ctx, executor, logger,
		"Get-CimInstance -ClassName Win32_BaseBoard | Select-Object -ExpandProperty SerialNumber")
	if psErr != nil {
		if logger != nil {
			logger.Warn("all motherboard serial methods failed")
		}

		return "", ErrAllMethodsFailed
	}

	return parsePowerShellValue(psOutput)
}

// windowsSystemUUID retrieves system UUID using wmic or PowerShell.
// Malformed, nil (all zeros) and max (all ones) UUIDs are rejected so the
// fallback path is triggered.
func windowsSystemUUID(ctx context.Context, executor CommandExecutor, logger *slog.Logger) (string, error) {
	if wmicAvailable(logger) {
		output, err := executeCommand(ctx, executor, logger, "wmic", "csproduct", "get", "UUID", "/value")
		if err == nil {
			value, parseErr := parseWmicValue(output, "UUID=")
			switch {
			case parseErr != nil:
				if logger != nil {
					logger.Debug("wmic UUID parsing failed", "error", parseErr)
				}
			case !isValidUUID(value):
				if logger != nil {
					logger.Debug("wmic returned invalid UUID, falling back", "uuid", value)
				}
			default:
				return value, nil
			}
		}
	}

	// Fallback to PowerShell
	if logger != nil {
		logger.Info("falling back to PowerShell for system UUID")
	}

	return windowsSystemUUIDViaPowerShell(ctx, executor, logger)
}

// windowsSystemUUIDViaPowerShell retrieves system UUID using PowerShell.
// Malformed, nil and max UUIDs are rejected with ErrNotFound.
func windowsSystemUUIDViaPowerShell(ctx context.Context, executor CommandExecutor, logger *slog.Logger) (string, error) {
	output, err := runPowerShell(ctx, executor, logger,
		"Get-CimInstance -ClassName Win32_ComputerSystemProduct | Select-Object -ExpandProperty UUID")
	if err != nil {
		return "", err
	}

	value, err := parsePowerShellValue(output)
	if err != nil {
		return "", err
	}

	if !isValidUUID(value) {
		if logger != nil {
			logger.Debug("PowerShell returned invalid UUID", "uuid", value)
		}

		return "", &ParseError{Source: "PowerShell output", Err: ErrNotFound}
	}

	return value, nil
}

// windowsDiskSerials retrieves disk serial numbers using wmic, with PowerShell fallback.
func windowsDiskSerials(ctx context.Context, executor CommandExecutor, logger *slog.Logger) ([]string, error) {
	if wmicAvailable(logger) {
		output, err := executeCommand(ctx, executor, logger, "wmic", "diskdrive", "get", "SerialNumber", "/value")
		if err == nil {
			if values := parseWmicMultipleValues(output, "SerialNumber="); len(values) > 0 {
				return values, nil
			}

			if logger != nil {
				logger.Debug("wmic returned no disk serials")
			}
		}
	}

	// Fallback to PowerShell Get-CimInstance
	if logger != nil {
		logger.Info("falling back to PowerShell for disk serials")
	}

	psOutput, psErr := runPowerShell(ctx, executor, logger,
		"Get-CimInstance -ClassName Win32_DiskDrive | Select-Object -ExpandProperty SerialNumber")
	if psErr != nil {
		if logger != nil {
			logger.Warn("all disk serial methods failed")
		}

		return nil, ErrAllMethodsFailed
	}

	values := parsePowerShellMultipleValues(psOutput)
	if len(values) == 0 {
		return nil, &ParseError{Source: "PowerShell output", Err: ErrNotFound}
	}

	return values, nil
}
