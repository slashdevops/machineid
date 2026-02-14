package machineid

import (
	"errors"
	"fmt"
)

// Sentinel errors returned by [Provider.ID] and recorded in [DiagnosticInfo.Errors].
var (
	// ErrNoIdentifiers is returned when no hardware identifiers could be
	// collected with the current configuration.
	ErrNoIdentifiers = errors.New("no hardware identifiers found with current configuration")

	// ErrEmptyValue is returned in [DiagnosticInfo.Errors] when a hardware
	// component returned an empty value.
	ErrEmptyValue = errors.New("empty value returned")

	// ErrNoValues is returned in [DiagnosticInfo.Errors] when a hardware
	// component returned no values.
	ErrNoValues = errors.New("no values found")

	// ErrNotFound is returned when a hardware value is not found in
	// command output or system files.
	ErrNotFound = errors.New("value not found")

	// ErrOEMPlaceholder is returned when a hardware value matches a
	// BIOS/UEFI OEM placeholder such as "To be filled by O.E.M.".
	ErrOEMPlaceholder = errors.New("value is OEM placeholder")

	// ErrAllMethodsFailed is returned when all collection methods for a
	// hardware component have been exhausted without success.
	ErrAllMethodsFailed = errors.New("all collection methods failed")
)

// CommandError records a failed system command execution.
// Use [errors.As] to extract the command name from wrapped errors.
type CommandError struct {
	Command string // command name, e.g. "sysctl", "ioreg", "wmic"
	Err     error  // underlying error from exec
}

// Error returns a human-readable description of the command failure.
func (e *CommandError) Error() string {
	return fmt.Sprintf("command %q failed: %v", e.Command, e.Err)
}

// Unwrap returns the underlying error.
func (e *CommandError) Unwrap() error {
	return e.Err
}

// ParseError records a failure while parsing command or system output.
// Use [errors.As] to extract the source from wrapped errors.
type ParseError struct {
	Source string // data source, e.g. "system_profiler JSON", "wmic output"
	Err    error  // underlying parse error
}

// Error returns a human-readable description of the parse failure.
func (e *ParseError) Error() string {
	return fmt.Sprintf("failed to parse %s: %v", e.Source, e.Err)
}

// Unwrap returns the underlying error.
func (e *ParseError) Unwrap() error {
	return e.Err
}

// ComponentError records a failure while collecting a specific hardware component.
// These errors appear in [DiagnosticInfo.Errors] and can be inspected with [errors.As].
type ComponentError struct {
	Component string // component name, e.g. "cpu", "uuid", "disk"
	Err       error  // underlying error
}

// Error returns a human-readable description of the component failure.
func (e *ComponentError) Error() string {
	return fmt.Sprintf("component %q: %v", e.Component, e.Err)
}

// Unwrap returns the underlying error.
func (e *ComponentError) Unwrap() error {
	return e.Err
}
