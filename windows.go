//go:build windows

package machineid

import (
	"context"
	"log/slog"
	"strings"
)

// collectIdentifiers gathers Windows-specific hardware identifiers based on provider config.
func collectIdentifiers(ctx context.Context, p *Provider, diag *DiagnosticInfo) ([]string, error) {
	var identifiers []string
	logger := p.logger

	if p.includeCPU {
		identifiers = appendIdentifierIfValid(identifiers, func() (string, error) {
			return windowsCPUID(ctx, p.commandExecutor, logger)
		}, "cpu:", diag, ComponentCPU, logger)
	}

	if p.includeMotherboard {
		identifiers = appendIdentifierIfValid(identifiers, func() (string, error) {
			return windowsMotherboardSerial(ctx, p.commandExecutor, logger)
		}, "mb:", diag, ComponentMotherboard, logger)
	}

	if p.includeSystemUUID {
		identifiers = appendIdentifierIfValid(identifiers, func() (string, error) {
			return windowsSystemUUID(ctx, p.commandExecutor, logger)
		}, "uuid:", diag, ComponentSystemUUID, logger)
	}

	if p.includeMAC {
		identifiers = appendIdentifiersIfValid(identifiers, func() ([]string, error) {
			return collectMACAddresses(p.macFilter, logger)
		}, "mac:", diag, ComponentMAC, logger)
	}

	if p.includeDisk {
		identifiers = appendIdentifiersIfValid(identifiers, func() ([]string, error) {
			return windowsDiskSerials(ctx, p.commandExecutor, logger)
		}, "disk:", diag, ComponentDisk, logger)
	}

	return identifiers, nil
}

// parseWmicValue extracts value from wmic output with given prefix.
func parseWmicValue(output, prefix string) (string, error) {
	lines := strings.SplitSeq(output, "\n")

	for line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
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
		if strings.HasPrefix(line, prefix) {
			value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			if value == "" || value == biosFirmwareMessage {
				continue
			}
			values = append(values, value)
		}
	}

	return values
}

// parsePowerShellValue extracts a trimmed, non-empty value from PowerShell output.
func parsePowerShellValue(output string) (string, error) {
	value := strings.TrimSpace(output)
	if value == "" {
		return "", &ParseError{Source: "PowerShell output", Err: ErrEmptyValue}
	}

	return value, nil
}

// parsePowerShellMultipleValues extracts multiple trimmed, non-empty values from PowerShell output.
func parsePowerShellMultipleValues(output string) []string {
	var values []string
	lines := strings.SplitSeq(output, "\n")

	for line := range lines {
		value := strings.TrimSpace(line)
		if value != "" {
			values = append(values, value)
		}
	}

	return values
}

// windowsCPUID retrieves CPU processor ID using wmic, with PowerShell fallback.
func windowsCPUID(ctx context.Context, executor CommandExecutor, logger *slog.Logger) (string, error) {
	output, err := executeCommand(ctx, executor, logger, "wmic", "cpu", "get", "ProcessorId", "/value")
	if err == nil {
		if value, parseErr := parseWmicValue(output, "ProcessorId="); parseErr == nil {
			return value, nil
		} else if logger != nil {
			logger.Debug("wmic CPU ID parsing failed", "error", parseErr)
		}
	}

	// Fallback to PowerShell Get-CimInstance
	if logger != nil {
		logger.Info("falling back to PowerShell for CPU ID")
	}

	psOutput, psErr := executeCommand(ctx, executor, logger, "powershell", "-Command",
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
	output, err := executeCommand(ctx, executor, logger, "wmic", "baseboard", "get", "SerialNumber", "/value")
	if err == nil {
		if value, parseErr := parseWmicValue(output, "SerialNumber="); parseErr == nil {
			return value, nil
		} else if logger != nil {
			logger.Debug("wmic motherboard serial parsing failed", "error", parseErr)
		}
	}

	// Fallback to PowerShell Get-CimInstance
	if logger != nil {
		logger.Info("falling back to PowerShell for motherboard serial")
	}

	psOutput, psErr := executeCommand(ctx, executor, logger, "powershell", "-Command",
		"Get-CimInstance -ClassName Win32_BaseBoard | Select-Object -ExpandProperty SerialNumber")
	if psErr != nil {
		if logger != nil {
			logger.Warn("all motherboard serial methods failed")
		}

		return "", ErrAllMethodsFailed
	}

	value, parseErr := parsePowerShellValue(psOutput)
	if parseErr != nil {
		return "", parseErr
	}

	if value == biosFirmwareMessage {
		return "", &ParseError{Source: "PowerShell output", Err: ErrOEMPlaceholder}
	}

	return value, nil
}

// windowsSystemUUID retrieves system UUID using wmic or PowerShell.
func windowsSystemUUID(ctx context.Context, executor CommandExecutor, logger *slog.Logger) (string, error) {
	// Try wmic first
	output, err := executeCommand(ctx, executor, logger, "wmic", "csproduct", "get", "UUID", "/value")
	if err == nil {
		if value, parseErr := parseWmicValue(output, "UUID="); parseErr == nil {
			return value, nil
		} else if logger != nil {
			logger.Debug("wmic UUID parsing failed", "error", parseErr)
		}
	}

	// Fallback to PowerShell
	if logger != nil {
		logger.Info("falling back to PowerShell for system UUID")
	}

	return windowsSystemUUIDViaPowerShell(ctx, executor, logger)
}

// windowsSystemUUIDViaPowerShell retrieves system UUID using PowerShell.
func windowsSystemUUIDViaPowerShell(ctx context.Context, executor CommandExecutor, logger *slog.Logger) (string, error) {
	output, err := executeCommand(ctx, executor, logger, "powershell", "-Command",
		"Get-CimInstance -ClassName Win32_ComputerSystemProduct | Select-Object -ExpandProperty UUID")
	if err != nil {
		return "", err
	}

	return parsePowerShellValue(output)
}

// windowsDiskSerials retrieves disk serial numbers using wmic, with PowerShell fallback.
func windowsDiskSerials(ctx context.Context, executor CommandExecutor, logger *slog.Logger) ([]string, error) {
	output, err := executeCommand(ctx, executor, logger, "wmic", "diskdrive", "get", "SerialNumber", "/value")
	if err == nil {
		if values := parseWmicMultipleValues(output, "SerialNumber="); len(values) > 0 {
			return values, nil
		}

		if logger != nil {
			logger.Debug("wmic returned no disk serials")
		}
	}

	// Fallback to PowerShell Get-CimInstance
	if logger != nil {
		logger.Info("falling back to PowerShell for disk serials")
	}

	psOutput, psErr := executeCommand(ctx, executor, logger, "powershell", "-Command",
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
