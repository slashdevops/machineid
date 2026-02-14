// Package machineid generates unique, deterministic machine identifiers derived
// from hardware characteristics. The generated IDs are stable across reboots
// but sensitive to hardware changes, making them suitable for software licensing,
// device fingerprinting, and telemetry correlation.
//
// # Zero Dependencies
//
// This package relies exclusively on the Go standard library and OS-level
// commands. There are no third-party dependencies.
//
// # Overview
//
// A [Provider] collects hardware signals (CPU, motherboard serial, system UUID,
// MAC addresses, disk serials), sorts and concatenates them, then produces a
// SHA-256 based hexadecimal fingerprint. The result length is always a power of
// two: 32, 64, 128, or 256 characters, controlled by [FormatMode].
//
// # Quick Start
//
//	id, err := machineid.New().
//		WithCPU().
//		WithSystemUUID().
//		ID(ctx)
//
// # Configuring Hardware Sources
//
// Enable individual hardware components via the With* methods:
//
//   - [Provider.WithCPU] — processor identifier and feature flags
//   - [Provider.WithMotherboard] — motherboard / baseboard serial number
//   - [Provider.WithSystemUUID] — BIOS / UEFI system UUID
//   - [Provider.WithMAC] — MAC addresses of physical network interfaces
//   - [Provider.WithDisk] — serial numbers of internal disks
//
// Or use [Provider.VMFriendly] to select a minimal, virtual-machine-safe
// subset (CPU + System UUID).
//
// # Output Formats
//
// Set the output length with [Provider.WithFormat]:
//
//   - [Format32] — 32 hex characters (128 bits, truncated SHA-256)
//   - [Format64] — 64 hex characters (256 bits, full SHA-256, default)
//   - [Format128] — 128 hex characters (512 bits, double SHA-256)
//   - [Format256] — 256 hex characters (1024 bits, quadruple SHA-256)
//
// All formats produce pure hexadecimal strings without dashes.
//
// # Salt
//
// [Provider.WithSalt] mixes an application-specific string into the hash so
// that two applications on the same machine produce different IDs:
//
//	id, _ := machineid.New().
//		WithCPU().
//		WithSystemUUID().
//		WithSalt("my-app-v1").
//		ID(ctx)
//
// # Validation
//
// [Provider.Validate] regenerates the ID and compares it to a previously
// stored value:
//
//	valid, err := provider.Validate(ctx, storedID)
//
// # Diagnostics
//
// After calling [Provider.ID], call [Provider.Diagnostics] to inspect which
// components were collected and which encountered errors:
//
//	diag := provider.Diagnostics()
//	fmt.Println("Collected:", diag.Collected)
//	fmt.Println("Errors:", diag.Errors)
//
// # Thread Safety
//
// A [Provider] is safe for concurrent use after configuration is complete.
// The first successful call to [Provider.ID] freezes the configuration and
// caches the result; subsequent calls return the cached value.
//
// # Testing
//
// Inject a custom [CommandExecutor] via [Provider.WithExecutor] to replace
// real system commands with deterministic test doubles:
//
//	provider := machineid.New().
//		WithExecutor(myMock).
//		WithCPU()
//
// # Platform Support
//
// Supported operating systems: macOS (darwin), Linux, and Windows. Each
// platform uses native tools (system_profiler / ioreg, /sys / lsblk, wmic /
// PowerShell) to collect hardware data.
//
// # Installation
//
// To use machineid as a library in your Go project:
//
//	go get github.com/slashdevops/machineid
//
// To install the CLI tool:
//
//	go install github.com/slashdevops/machineid/cmd/machineid@latest
//
// Precompiled binaries for macOS, Linux, and Windows are available on the
// [releases page]: https://github.com/slashdevops/machineid/releases
//
// # CLI Tool
//
// A ready-to-use command-line tool is provided in cmd/machineid:
//
//	machineid -cpu -uuid
//	machineid -all -format 32 -json
//	machineid -vm -salt "my-app" -diagnostics
//	machineid -version
//	machineid -version.long
package machineid
