package machineid

import (
	"errors"
	"fmt"
	"testing"
)

func TestCommandErrorMessage(t *testing.T) {
	inner := fmt.Errorf("exit status 1")
	err := &CommandError{Command: "sysctl", Err: inner}

	want := `command "sysctl" failed: exit status 1`
	if err.Error() != want {
		t.Errorf("CommandError.Error() = %q, want %q", err.Error(), want)
	}
}

func TestCommandErrorUnwrap(t *testing.T) {
	inner := fmt.Errorf("exit status 1")
	err := &CommandError{Command: "sysctl", Err: inner}

	if err.Unwrap() != inner {
		t.Error("CommandError.Unwrap() did not return inner error")
	}
}

func TestCommandErrorAs(t *testing.T) {
	inner := fmt.Errorf("exit status 1")
	err := fmt.Errorf("collecting CPU: %w", &CommandError{Command: "sysctl", Err: inner})

	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatal("errors.As() should find CommandError in wrapped chain")
	}

	if cmdErr.Command != "sysctl" {
		t.Errorf("CommandError.Command = %q, want %q", cmdErr.Command, "sysctl")
	}
}

func TestParseErrorMessage(t *testing.T) {
	inner := fmt.Errorf("unexpected end of JSON input")
	err := &ParseError{Source: "system_profiler JSON", Err: inner}

	want := "failed to parse system_profiler JSON: unexpected end of JSON input"
	if err.Error() != want {
		t.Errorf("ParseError.Error() = %q, want %q", err.Error(), want)
	}
}

func TestParseErrorUnwrap(t *testing.T) {
	inner := fmt.Errorf("unexpected end of JSON input")
	err := &ParseError{Source: "system_profiler JSON", Err: inner}

	if err.Unwrap() != inner {
		t.Error("ParseError.Unwrap() did not return inner error")
	}
}

func TestParseErrorAs(t *testing.T) {
	inner := fmt.Errorf("invalid character")
	err := fmt.Errorf("hardware UUID: %w", &ParseError{Source: "ioreg output", Err: inner})

	var parseErr *ParseError
	if !errors.As(err, &parseErr) {
		t.Fatal("errors.As() should find ParseError in wrapped chain")
	}

	if parseErr.Source != "ioreg output" {
		t.Errorf("ParseError.Source = %q, want %q", parseErr.Source, "ioreg output")
	}
}

func TestComponentErrorMessage(t *testing.T) {
	inner := ErrNotFound
	err := &ComponentError{Component: "uuid", Err: inner}

	want := `component "uuid": value not found`
	if err.Error() != want {
		t.Errorf("ComponentError.Error() = %q, want %q", err.Error(), want)
	}
}

func TestComponentErrorUnwrap(t *testing.T) {
	err := &ComponentError{Component: "cpu", Err: ErrAllMethodsFailed}

	if err.Unwrap() != ErrAllMethodsFailed {
		t.Error("ComponentError.Unwrap() did not return inner error")
	}
}

func TestComponentErrorIs(t *testing.T) {
	err := &ComponentError{Component: "cpu", Err: ErrAllMethodsFailed}

	if !errors.Is(err, ErrAllMethodsFailed) {
		t.Error("errors.Is(ComponentError, ErrAllMethodsFailed) should be true")
	}
}

func TestComponentErrorAs(t *testing.T) {
	err := fmt.Errorf("ID generation: %w", &ComponentError{Component: "disk", Err: ErrNotFound})

	var compErr *ComponentError
	if !errors.As(err, &compErr) {
		t.Fatal("errors.As() should find ComponentError in wrapped chain")
	}

	if compErr.Component != "disk" {
		t.Errorf("ComponentError.Component = %q, want %q", compErr.Component, "disk")
	}

	if !errors.Is(compErr, ErrNotFound) {
		t.Error("errors.Is(ComponentError, ErrNotFound) should be true through Unwrap")
	}
}

func TestSentinelErrorsWrapping(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		sentinel error
	}{
		{"ErrNotFound wrapped", fmt.Errorf("UUID not found: %w", ErrNotFound), ErrNotFound},
		{"ErrOEMPlaceholder wrapped", fmt.Errorf("serial is placeholder: %w", ErrOEMPlaceholder), ErrOEMPlaceholder},
		{"ErrAllMethodsFailed wrapped", fmt.Errorf("CPU failed: %w", ErrAllMethodsFailed), ErrAllMethodsFailed},
		{"ErrEmptyValue wrapped", fmt.Errorf("empty: %w", ErrEmptyValue), ErrEmptyValue},
		{"ErrNoValues wrapped", fmt.Errorf("no values: %w", ErrNoValues), ErrNoValues},
		{"ErrNoIdentifiers direct", ErrNoIdentifiers, ErrNoIdentifiers},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !errors.Is(tt.err, tt.sentinel) {
				t.Errorf("errors.Is() should find %v in %v", tt.sentinel, tt.err)
			}
		})
	}
}

func TestDeepWrappingChain(t *testing.T) {
	// CommandError -> ComponentError -> wrapped error
	cmdErr := &CommandError{Command: "ioreg", Err: fmt.Errorf("not found")}
	compErr := &ComponentError{Component: "uuid", Err: cmdErr}
	topErr := fmt.Errorf("ID generation failed: %w", compErr)

	var gotCmd *CommandError
	if !errors.As(topErr, &gotCmd) {
		t.Fatal("errors.As() should find CommandError through deep chain")
	}

	if gotCmd.Command != "ioreg" {
		t.Errorf("CommandError.Command = %q, want %q", gotCmd.Command, "ioreg")
	}

	var gotComp *ComponentError
	if !errors.As(topErr, &gotComp) {
		t.Fatal("errors.As() should find ComponentError through deep chain")
	}

	if gotComp.Component != "uuid" {
		t.Errorf("ComponentError.Component = %q, want %q", gotComp.Component, "uuid")
	}
}
