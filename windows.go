//go:build windows

package machineid

import (
	"context"
	"log/slog"
	"strings"
	"sync"
)

// componentResult holds the result from a single concurrent component collection.
type componentResult struct {
	component string
	prefix    string
	value     string   // for single-value components
	values    []string // for multi-value components (MAC, disk)
	err       error
	multi     bool // true if this is a multi-value result
}

// collectIdentifiers gathers Windows-specific hardware identifiers concurrently.
// Windows commands (wmic, PowerShell) are slow due to process startup overhead,
// so all components are collected in parallel to minimize total latency.
func collectIdentifiers(ctx context.Context, p *Provider, diag *DiagnosticInfo) ([]string, error) {
	logger := p.logger

	var wg sync.WaitGroup
	resultsCh := make(chan componentResult, 5)

	if p.includeCPU {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := windowsCPUID(ctx, p.commandExecutor, logger)
			resultsCh <- componentResult{component: ComponentCPU, prefix: "cpu:", value: value, err: err}
		}()
	}

	if p.includeMotherboard {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := windowsMotherboardSerial(ctx, p.commandExecutor, logger)
			resultsCh <- componentResult{component: ComponentMotherboard, prefix: "mb:", value: value, err: err}
		}()
	}

	if p.includeSystemUUID {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := windowsSystemUUID(ctx, p.commandExecutor, logger)
			resultsCh <- componentResult{component: ComponentSystemUUID, prefix: "uuid:", value: value, err: err}
		}()
	}

	if p.includeMAC {
		wg.Add(1)
		go func() {
			defer wg.Done()
			values, err := collectMACAddresses(p.macFilter, logger)
			resultsCh <- componentResult{component: ComponentMAC, prefix: "mac:", values: values, err: err, multi: true}
		}()
	}

	if p.includeDisk {
		wg.Add(1)
		go func() {
			defer wg.Done()
			values, err := windowsDiskSerials(ctx, p.commandExecutor, logger)
			resultsCh <- componentResult{component: ComponentDisk, prefix: "disk:", values: values, err: err, multi: true}
		}()
	}

	// Close channel once all goroutines complete.
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	// Collect results and build identifiers.
	var identifiers []string
	for r := range resultsCh {
		if r.multi {
			identifiers = appendMultiResult(identifiers, r, diag, logger)
		} else {
			identifiers = appendSingleResult(identifiers, r, diag, logger)
		}
	}

	return identifiers, nil
}

// appendSingleResult processes a single-value component result into identifiers.
func appendSingleResult(identifiers []string, r componentResult, diag *DiagnosticInfo, logger *slog.Logger) []string {
	if r.err != nil {
		compErr := &ComponentError{Component: r.component, Err: r.err}
		if diag != nil {
			diag.Errors[r.component] = compErr
		}
		if logger != nil {
			logger.Warn("component failed", "component", r.component, "error", r.err)
		}
		return identifiers
	}

	if r.value == "" {
		compErr := &ComponentError{Component: r.component, Err: ErrEmptyValue}
		if diag != nil {
			diag.Errors[r.component] = compErr
		}
		if logger != nil {
			logger.Warn("component returned empty value", "component", r.component)
		}
		return identifiers
	}

	if diag != nil {
		diag.Collected = append(diag.Collected, r.component)
	}
	if logger != nil {
		logger.Info("component collected", "component", r.component)
		logger.Debug("component value", "component", r.component, "value", r.value)
	}

	return append(identifiers, r.prefix+r.value)
}

// appendMultiResult processes a multi-value component result into identifiers.
func appendMultiResult(identifiers []string, r componentResult, diag *DiagnosticInfo, logger *slog.Logger) []string {
	if r.err != nil {
		compErr := &ComponentError{Component: r.component, Err: r.err}
		if diag != nil {
			diag.Errors[r.component] = compErr
		}
		if logger != nil {
			logger.Warn("component failed", "component", r.component, "error", r.err)
		}
		return identifiers
	}

	if len(r.values) == 0 {
		compErr := &ComponentError{Component: r.component, Err: ErrNoValues}
		if diag != nil {
			diag.Errors[r.component] = compErr
		}
		if logger != nil {
			logger.Warn("component returned no values", "component", r.component)
		}
		return identifiers
	}

	if diag != nil {
		diag.Collected = append(diag.Collected, r.component)
	}
	if logger != nil {
		logger.Info("component collected", "component", r.component, "count", len(r.values))
		logger.Debug("component values", "component", r.component, "values", r.values)
	}

	for _, value := range r.values {
		identifiers = append(identifiers, r.prefix+value)
	}

	return identifiers
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
