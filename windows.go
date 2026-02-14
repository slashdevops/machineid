//go:build windows

package machineid

import (
	"fmt"
	"strings"
)

// collectIdentifiers gathers Windows-specific hardware identifiers based on provider config.
func collectIdentifiers(g *Provider, diag *DiagnosticInfo) ([]string, error) {
	var identifiers []string

	if g.includeCPU {
		identifiers = appendIdentifierIfValid(identifiers, func() (string, error) {
			return windowsCPUID(g.commandExecutor)
		}, "cpu:", diag, ComponentCPU)
	}

	if g.includeMotherboard {
		identifiers = appendIdentifierIfValid(identifiers, func() (string, error) {
			return windowsMotherboardSerial(g.commandExecutor)
		}, "mb:", diag, ComponentMotherboard)
	}

	if g.includeSystemUUID {
		identifiers = appendIdentifierIfValid(identifiers, func() (string, error) {
			return windowsSystemUUID(g.commandExecutor)
		}, "uuid:", diag, ComponentSystemUUID)
	}

	if g.includeMAC {
		identifiers = appendIdentifiersIfValid(identifiers, collectMACAddresses, "mac:", diag, ComponentMAC)
	}

	if g.includeDisk {
		identifiers = appendIdentifiersIfValid(identifiers, func() ([]string, error) {
			return windowsDiskSerials(g.commandExecutor)
		}, "disk:", diag, ComponentDisk)
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

	return "", fmt.Errorf("value with prefix %s not found", prefix)
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
		return "", fmt.Errorf("empty value from PowerShell")
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
func windowsCPUID(executor CommandExecutor) (string, error) {
	output, err := executeCommand(executor, "wmic", "cpu", "get", "ProcessorId", "/value")
	if err == nil {
		if value, parseErr := parseWmicValue(output, "ProcessorId="); parseErr == nil {
			return value, nil
		}
	}

	// Fallback to PowerShell Get-CimInstance
	psOutput, psErr := executeCommand(executor, "powershell", "-Command",
		"Get-CimInstance -ClassName Win32_Processor | Select-Object -ExpandProperty ProcessorId")
	if psErr != nil {
		return "", fmt.Errorf("failed to get CPU ID: wmic: %w, powershell: %w", err, psErr)
	}

	return parsePowerShellValue(psOutput)
}

// windowsMotherboardSerial retrieves motherboard serial number using wmic, with PowerShell fallback.
func windowsMotherboardSerial(executor CommandExecutor) (string, error) {
	output, err := executeCommand(executor, "wmic", "baseboard", "get", "SerialNumber", "/value")
	if err == nil {
		if value, parseErr := parseWmicValue(output, "SerialNumber="); parseErr == nil {
			return value, nil
		}
	}

	// Fallback to PowerShell Get-CimInstance
	psOutput, psErr := executeCommand(executor, "powershell", "-Command",
		"Get-CimInstance -ClassName Win32_BaseBoard | Select-Object -ExpandProperty SerialNumber")
	if psErr != nil {
		return "", fmt.Errorf("failed to get motherboard serial: wmic: %w, powershell: %w", err, psErr)
	}

	value, parseErr := parsePowerShellValue(psOutput)
	if parseErr != nil {
		return "", parseErr
	}

	if value == biosFirmwareMessage {
		return "", fmt.Errorf("motherboard serial is OEM placeholder")
	}

	return value, nil
}

// windowsSystemUUID retrieves system UUID using wmic or PowerShell.
func windowsSystemUUID(executor CommandExecutor) (string, error) {
	// Try wmic first
	output, err := executeCommand(executor, "wmic", "csproduct", "get", "UUID", "/value")
	if err == nil {
		if value, parseErr := parseWmicValue(output, "UUID="); parseErr == nil {
			return value, nil
		}
	}

	// Fallback to PowerShell
	return windowsSystemUUIDViaPowerShell(executor)
}

// windowsSystemUUIDViaPowerShell retrieves system UUID using PowerShell.
func windowsSystemUUIDViaPowerShell(executor CommandExecutor) (string, error) {
	output, err := executeCommand(executor, "powershell", "-Command",
		"Get-CimInstance -ClassName Win32_ComputerSystemProduct | Select-Object -ExpandProperty UUID")
	if err != nil {
		return "", fmt.Errorf("failed to get UUID via PowerShell: %w", err)
	}

	return parsePowerShellValue(output)
}

// windowsDiskSerials retrieves disk serial numbers using wmic, with PowerShell fallback.
func windowsDiskSerials(executor CommandExecutor) ([]string, error) {
	output, err := executeCommand(executor, "wmic", "diskdrive", "get", "SerialNumber", "/value")
	if err == nil {
		if values := parseWmicMultipleValues(output, "SerialNumber="); len(values) > 0 {
			return values, nil
		}
	}

	// Fallback to PowerShell Get-CimInstance
	psOutput, psErr := executeCommand(executor, "powershell", "-Command",
		"Get-CimInstance -ClassName Win32_DiskDrive | Select-Object -ExpandProperty SerialNumber")
	if psErr != nil {
		return nil, fmt.Errorf("failed to get disk serials: wmic: %w, powershell: %w", err, psErr)
	}

	values := parsePowerShellMultipleValues(psOutput)
	if len(values) == 0 {
		return nil, fmt.Errorf("no disk serials found via PowerShell")
	}

	return values, nil
}
