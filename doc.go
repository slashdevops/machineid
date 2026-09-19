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
//   - [Provider.WithCPU] — processor identifier (vendor and model; no volatile feature flags)
//   - [Provider.WithMotherboard] — motherboard / baseboard serial number
//   - [Provider.WithSystemUUID] — BIOS / UEFI system UUID
//   - [Provider.WithMAC] — MAC addresses of network interfaces (filterable)
//   - [Provider.WithDisk] — serial numbers of internal disks
//
// Or use [Provider.VMFriendly] to select a minimal, virtual-machine-safe
// subset (CPU + System UUID).
//
// # MAC Address Filtering
//
// [Provider.WithMAC] accepts an optional [MACFilter] to control which network
// interfaces contribute to the machine ID:
//
//   - [MACFilterPhysical] — only physical interfaces (default)
//   - [MACFilterAll] — all non-loopback, up interfaces (physical + virtual)
//   - [MACFilterVirtual] — only virtual interfaces (VPN, bridge, container)
//
// Examples:
//
//	// Physical interfaces only (default, most stable)
//	provider.WithMAC()
//
//	// Include all interfaces
//	provider.WithMAC(machineid.MACFilterAll)
//
//	// Only virtual interfaces (containers, VPNs)
//	provider.WithMAC(machineid.MACFilterVirtual)
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
// # Context, Timeouts and Cancellation
//
// [Provider.ID] takes a [context.Context] that bounds every system command it
// runs. Each command additionally has its own five second timeout. A context
// that is already done is rejected before any collection starts. When a
// command is ended by the deadline or by cancellation, the [CommandError]
// recorded for that component wraps [context.DeadlineExceeded] or
// [context.Canceled], so callers can match it with [errors.Is].
//
// # Diagnostics
//
// After calling [Provider.ID], call [Provider.Diagnostics] to inspect which
// components were collected and which encountered errors. The returned value
// is a copy and can be modified freely:
//
//	diag := provider.Diagnostics()
//	fmt.Println("Collected:", diag.Collected)
//	fmt.Println("Errors:", diag.Errors)
//
// # Logging
//
// [Provider.WithLogger] accepts a [*log/slog.Logger] for optional observability.
// When set, the provider logs component collection results, fallback paths,
// command execution timing, and errors. A nil logger (the default) disables
// all logging with zero overhead.
//
//	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
//	id, err := machineid.New().
//		WithCPU().
//		WithSystemUUID().
//		WithLogger(logger).
//		ID(ctx)
//
// Log levels:
//   - Info: component collected, fallback triggered, ID generation lifecycle
//   - Warn: component failed or returned empty value
//   - Debug: command execution details, raw hardware values, timing, reuse of
//     cached command output
//
// # Errors
//
// The package provides sentinel errors for programmatic error handling:
//
//   - [ErrNoIdentifiers] — no hardware identifiers collected
//   - [ErrEmptyValue] — a component returned an empty value
//   - [ErrNoValues] — a multi-value component returned no values
//   - [ErrNotFound] — a value was not found in command output or system files
//   - [ErrOEMPlaceholder] — a value matches a BIOS/UEFI OEM placeholder
//   - [ErrAllMethodsFailed] — all collection methods for a component were exhausted
//
// Typed errors provide structured context for [errors.AsType]:
//
//   - [CommandError] — a system command execution failed (includes the command
//     name and the first line of its stderr)
//   - [ParseError] — output parsing failed (includes the data source)
//   - [ComponentError] — a hardware component failed (includes the component name)
//
// Errors in [DiagnosticInfo.Errors] are wrapped in [ComponentError], so callers
// can inspect both the component name and the underlying cause:
//
//	if compErr, ok := errors.AsType[*machineid.ComponentError](diag.Errors["cpu"]); ok {
//		fmt.Println("component:", compErr.Component)
//		fmt.Println("cause:", compErr.Err)
//	}
//
// # Input Validation
//
// Values that cannot identify a machine are rejected before hashing:
//
//   - System UUIDs must parse as a UUID and must not be the nil UUID
//     (all zeros) or the max UUID (all ones), which firmware reports when no
//     UUID is programmed. The raw string is hashed exactly as the platform
//     reported it, so a well-formed UUID always produces the same ID.
//   - Serial numbers equal to the BIOS placeholder "To be filled by O.E.M."
//     are discarded.
//
// A rejected value falls through to the next source for that component, and
// the reason is recorded in [DiagnosticInfo.Errors] if every source fails.
//
// # Thread Safety
//
// A [Provider] is safe for concurrent use after configuration is complete.
// The first successful call to [Provider.ID] freezes the configuration and
// caches the result; subsequent calls return the cached value.
//
// # Concurrency and Performance
//
// On every platform, the enabled components are collected concurrently, one
// goroutine per component, so the total latency is that of the slowest
// single query rather than the sum. Results are folded in a fixed order, so
// the ID and [DiagnosticInfo.Collected] are deterministic no matter which
// query finishes first. Each collector goroutine carries a runtime/pprof
// label (machineid.component=<name>) that appears in tracebacks.
//
// On macOS, the UUID, serial and CPU collectors share one execution of
// `system_profiler SPHardwareDataType -json` per [Provider.ID] call instead
// of spawning it once each.
//
// # Testing
//
// Inject a custom [CommandExecutor] via [Provider.WithExecutor] to replace
// real system commands with deterministic test doubles. Custom executors
// must be safe for concurrent use, since components are collected in
// parallel goroutines. Passing nil keeps the current executor.
//
//	provider := machineid.New().
//		WithExecutor(myMock).
//		WithCPU()
//
// # Platform Support
//
// Supported operating systems: macOS (darwin), Linux, and Windows. Each
// platform uses native tools to collect hardware data, with fallbacks:
//
//   - macOS: system_profiler (primary), ioreg and sysctl (fallbacks)
//   - Linux: /proc/cpuinfo, /sys/class/dmi/id, /etc/machine-id, lsblk, /sys/block
//   - Windows: wmic when present, PowerShell Get-CimInstance otherwise
//
// On Windows 11 24H2 and Windows Server 2025, where wmic has been removed,
// the library detects its absence and uses PowerShell directly. PowerShell
// is always started with -NoProfile -NonInteractive so user profiles cannot
// alter the output.
//
// # Installation
//
// To use machineid as a library in your Go project (Go 1.27 or newer):
//
//	go get github.com/slashdevops/machineid
//
// To install the CLI tool:
//
//	go install github.com/slashdevops/machineid/cmd/machineid@latest
//
// Signed binaries for macOS (13 Ventura or newer) and Linux are available on
// the [releases page]: https://github.com/slashdevops/machineid/releases
//
// # CLI Tool
//
// A ready-to-use command-line tool is provided in cmd/machineid.
// When no component flags are specified, the default is -cpu -motherboard -uuid.
//
//	machineid                               # default: CPU + motherboard + UUID
//	machineid -cpu -uuid                    # specific components
//	machineid -all -format 32 -json         # all hardware, compact JSON
//	machineid -vm -salt "my-app"            # VM-friendly with salt
//	machineid -mac -mac-filter all          # include all MAC addresses
//	machineid -all -json -diagnostics       # JSON with per-component diagnostics
//	machineid -cpu -uuid -validate <id>     # validate a stored ID
//	machineid -all -verbose                 # info-level logs
//	machineid -all -debug                   # debug-level logs
//	machineid -version                      # version info
//	machineid -version-long                 # detailed build info
//
// Ctrl-C or SIGTERM cancels any in-flight hardware query.
package machineid
