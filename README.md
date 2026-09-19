# 🆔 machineid

**Deterministic, hardware-derived machine identifiers for Go. Zero dependencies. One binary.**

[![Pull Request](https://github.com/slashdevops/machineid/actions/workflows/pr.yml/badge.svg)](https://github.com/slashdevops/machineid/actions/workflows/pr.yml)
[![Release](https://github.com/slashdevops/machineid/actions/workflows/release.yml/badge.svg)](https://github.com/slashdevops/machineid/actions/workflows/release.yml)
[![CodeQL](https://github.com/slashdevops/machineid/actions/workflows/codeql.yml/badge.svg)](https://github.com/slashdevops/machineid/actions/workflows/codeql.yml)
[![Go version](https://img.shields.io/github/go-mod/go-version/slashdevops/machineid)](go.mod)
[![Go Reference](https://pkg.go.dev/badge/github.com/slashdevops/machineid.svg)](https://pkg.go.dev/github.com/slashdevops/machineid)
[![Go Report Card](https://goreportcard.com/badge/github.com/slashdevops/machineid)](https://goreportcard.com/report/github.com/slashdevops/machineid)
[![Latest release](https://img.shields.io/github/v/release/slashdevops/machineid?sort=semver)](https://github.com/slashdevops/machineid/releases/latest)
[![License](https://img.shields.io/github/license/slashdevops/machineid.svg)](LICENSE)

`machineid` turns the hardware a program is running on into a stable, opaque identifier. The same machine always produces the same ID; a different machine never does. IDs survive reboots and OS reinstalls, change when the hardware changes, and reveal nothing about the hardware itself.

Use it for **software licensing and activation**, **device fingerprinting**, **per-host telemetry correlation**, **fleet inventory**, or anywhere you need "which machine is this?" answered without a database.

```bash
$ machineid
b5c42832542981af58c9dc3bc241219e780ff7d276cfad05fac222846edb84f7
```

```go
id, err := machineid.New().WithCPU().WithSystemUUID().ID(ctx)
```

---

## 📚 Table of contents

- [✨ Features](#-features)
- [📦 Installation](#-installation)
- [⬆️ Updating the CLI](#️-updating-the-cli)
- [🚀 Quick start](#-quick-start)
- [🖥️ CLI](#️-cli)
- [📖 Library guide](#-library-guide)
- [⚙️ How it works](#️-how-it-works)
- [🧭 Choosing components](#-choosing-components)
- [🔒 Security](#-security)
- [🧪 Testing](#-testing)
- [🛠️ Troubleshooting](#️-troubleshooting)
- [🤝 Contributing](#-contributing)
- [📄 License](#-license)

---

## ✨ Features

| | |
|---|---|
| 🧩 **Zero dependencies** | Built entirely on the Go standard library. Nothing to audit, nothing to update. |
| 🌍 **Cross-platform** | macOS, Linux and Windows, each with native primary sources and fallbacks. |
| 🎛️ **Configurable signals** | Pick any mix of CPU, motherboard serial, system UUID, MAC addresses and disk serials. |
| 📏 **Power-of-two output** | 32, 64, 128 or 256 hex characters, pure hex, no dashes. |
| 🔐 **SHA-256 based** | One-way hash. Hardware details cannot be recovered from an ID. |
| 🧂 **Salt support** | Different IDs for different applications on the same machine. |
| ☁️ **VM friendly** | A preset that ignores the signals virtual machines and clouds change. |
| ⚡ **Concurrent collection** | Every hardware query runs in parallel on every platform. Latency is the slowest single query, not the sum. |
| 🔁 **Deterministic** | Results are folded in a fixed order. Same ID, same diagnostics, every run. |
| 🩺 **Diagnostics API** | See exactly which components were collected and why the others failed. |
| 🪵 **Optional `slog` logging** | Structured logs at info, warn and debug. Zero overhead when no logger is set. |
| 🚨 **Structured errors** | Sentinel errors for `errors.Is`, typed errors for `errors.AsType`, timeouts that match `context.DeadlineExceeded`. |
| 🧪 **Testable** | Inject a command executor and run the whole library against fixtures. |
| 🧵 **Thread-safe** | A configured provider can be shared freely across goroutines. |

---

## 📦 Installation

### Library

```bash
go get github.com/slashdevops/machineid
```

Requires **Go 1.27 or newer**. No external dependencies.

### CLI

#### With `go install`

```bash
go install github.com/slashdevops/machineid/cmd/machineid@latest
```

Make sure `$(go env GOPATH)/bin` is on your `PATH`:

```bash
# bash
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.bash_profile && source ~/.bash_profile

# zsh
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc && source ~/.zshrc
```

#### Pre-built binaries

Signed binaries are published on the [releases page](https://github.com/slashdevops/machineid/releases).

**🍎 macOS** (signed, notarized, universal `.pkg` for Apple Silicon and Intel; requires **macOS 13 Ventura or newer**):

```bash
curl -L https://github.com/slashdevops/machineid/releases/latest/download/machineid-darwin-universal.pkg -o machineid.pkg
sudo installer -pkg machineid.pkg -target /
```

Or double-click the `.pkg` in Finder.

**🐧 Linux** (`amd64` and `arm64`, signed with Sigstore):

```bash
ARCH=amd64   # or arm64
curl -L "https://github.com/slashdevops/machineid/releases/latest/download/machineid-linux-${ARCH}.zip" -o machineid.zip
unzip machineid.zip && sudo install -m 0755 machineid /usr/local/bin/machineid
```

**🪟 Windows**: use `go install` above or build from source. Pre-built Windows binaries are not published yet.

How to verify a download: [macOS signing and notarization](docs/macos-signing.md) and [Linux Sigstore verification](docs/linux-signing.md). Already installed? See [Updating the CLI](#️-updating-the-cli).

#### From source

```bash
git clone https://github.com/slashdevops/machineid.git
cd machineid
make build
./build/machineid -version
```

---

## ⬆️ Updating the CLI

Once installed, the CLI updates itself. Full guide with per-platform details, script recipes and a troubleshooting table: **[docs/updating.md](docs/updating.md)**.

```bash
machineid update            # check, show the plan, ask, install
machineid update -check     # what would happen, nothing changes
machineid update -yes       # non-interactive (sudo on macOS for the .pkg)
```

It looks up the newest release, shows a checklist and the plan, asks for confirmation, downloads the asset for your platform, verifies its SHA-256 and signature, and replaces the binary you are running. Nothing is downloaded until every check has passed. Root is never requested; where it is needed (the macOS package) the exact `sudo` command is printed.

```text
$ machineid update
Checking for updates…
  ✓ current version          v0.2.0
  ✓ running binary           /usr/local/bin/machineid
  ✓ platform supported       darwin/arm64
  ✓ latest release           v0.3.0  (live, 4 of 5 checks left this hour)
  ✓ install target           /usr/local/bin (running as root)

→ Updating machineid v0.2.0 → v0.3.0 using the signed macOS package

Update machineid now? [y/N] y
   downloading machineid-darwin-universal.pkg…
   ✓ SHA-256 verified
   ✓ pkgutil: Developer ID Installer: SlashDevOps
   ✓ installed to /usr/local/bin

✅ Updated to v0.3.0
```

| Flag | Meaning |
|------|---------|
| `-check` | Report what would happen and change nothing. Served from the cache when it is under an hour old. |
| `-refresh` | Look up the latest release now instead of using the cache. |
| `-version TAG` | Install a specific release, e.g. `-version v0.2.0`. This is also how to go back a version. |
| `-method auto\|release\|go` | `release` installs the signed asset (default where one exists), `go` rebuilds with `go install`. |
| `-force` | Install to the method's location even if this binary lives elsewhere, and reinstall an equal version. |
| `-yes` | Do not ask for confirmation. Required when stdin is not a terminal. |
| `-require-signature` | Fail unless the signature was verified: Sigstore via `cosign` on Linux, Apple's on macOS. Without the flag a missing `cosign` only prints a warning. |

| Exit code | Meaning |
|-----------|---------|
| `0` | Updated, already current, `-check`, or you declined |
| `1` | A prerequisite was not met. **Nothing was attempted.** Fix it and re-run. |
| `2` | Invalid arguments |
| `3` | The update was attempted and failed. A checksum failure leaves the old binary untouched. |

**Where it installs.** The Linux zip and the macOS universal zip replace the binary in place, wherever it is. The macOS `.pkg` always installs to `/usr/local/bin` and needs root, so it is only chosen when that is where you are running from. `go install` always writes to `GOBIN`. If the chosen method would land somewhere else, the update refuses and tells you which flag targets your copy. Windows has no published binaries yet, so it uses `-method go`.

**Network use and limits.** `machineid update` is the **only** thing in this tool that touches the network. Normal runs, `-validate` and `-version` never do, and there is no background check. The lookup asks `github.com` for the newest tag with a plain HTTPS request, without the GitHub API and without any token. The answer is cached for an hour and at most **5 live lookups per hour** are made per user, so a cron job running `machineid update -check` costs GitHub nothing after the first call. `MACHINEID_UPDATE_BUDGET` raises the limit if you must. Downloads only happen after you confirm a newer version.

---

## 🚀 Quick start

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/slashdevops/machineid"
)

func main() {
    ctx := context.Background()

    id, err := machineid.New().
        WithCPU().
        WithSystemUUID().
        ID(ctx)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(id) // 64 hex characters, e.g. b5c42832542981af…
}
```

The context bounds every system command the library runs. Pass one with a deadline if you need a hard upper limit.

---

## 🖥️ CLI

```bash
# Default: CPU + motherboard + UUID, 64 hex characters
machineid

# Pick components
machineid -cpu -uuid

# Everything, compact 32-character output
machineid -all -format 32

# VM-friendly preset with an application salt
machineid -vm -salt "my-app"

# JSON with per-component diagnostics
machineid -all -json -diagnostics

# Validate an ID you stored earlier (exit code 0 = match, 1 = mismatch)
machineid -cpu -uuid -validate "b5c42832542981af58c9dc3bc241219e780ff7d276cfad05fac222846edb84f7"

# Include virtual interfaces too
machineid -mac -mac-filter all

# Logs to stderr: info level, or debug level with commands, raw values and timing
machineid -all -verbose
machineid -all -debug

# Build information
machineid -version
machineid -version-long
```

Ctrl-C or `SIGTERM` cancels any hardware query that is still running.

### Flags

| Flag | Description |
|------|-------------|
| `-cpu` | Include the CPU identifier |
| `-motherboard` | Include the motherboard serial number |
| `-uuid` | Include the system UUID (BIOS/UEFI) |
| `-mac` | Include network interface MAC addresses |
| `-mac-filter F` | MAC filter: `physical` (default), `all` or `virtual` |
| `-disk` | Include disk serial numbers |
| `-all` | Include every component |
| `-vm` | VM-friendly preset: CPU + UUID only |
| `-format N` | Output length: `32`, `64` (default), `128` or `256` hex characters |
| `-salt STRING` | Application-specific salt |
| `-validate ID` | Compare a stored ID against this machine |
| `-diagnostics` | Show which components were collected or failed |
| `-json` | JSON output |
| `-verbose` | Info-level logs on stderr |
| `-debug` | Debug-level logs on stderr |
| `-version` | Print the version |
| `-version-long` | Print detailed build information |

With no component flags the default is `-cpu -motherboard -uuid`.

| Exit code | Meaning |
|-----------|---------|
| `0` | Success, or `-validate` matched |
| `1` | ID generation failed, or `-validate` did not match |
| `2` | Invalid arguments |

---

## 📖 Library guide

### Selecting hardware components

```go
provider := machineid.New().
    WithCPU().          // processor identifier and feature flags
    WithMotherboard().  // motherboard / baseboard serial number
    WithSystemUUID().   // BIOS / UEFI system UUID
    WithMAC().          // physical network interface MAC addresses
    WithDisk()          // internal disk serial numbers

id, err := provider.ID(ctx)
```

### MAC address filtering

```go
// Physical interfaces only (default, most stable on bare metal)
machineid.New().WithCPU().WithMAC()

// Physical and virtual (VPN, Docker, bridges)
machineid.New().WithCPU().WithMAC(machineid.MACFilterAll)

// Virtual interfaces only (container-specific fingerprinting)
machineid.New().WithCPU().WithMAC(machineid.MACFilterVirtual)
```

| Filter | Interfaces included | Best for |
|--------|---------------------|----------|
| `MACFilterPhysical` | `en0`, `eth0`, `wlan0`, … (default) | Bare-metal stability |
| `MACFilterAll` | Physical + virtual (`docker0`, `utun`, `bridge`, …) | Maximum uniqueness |
| `MACFilterVirtual` | `docker0`, `utun`, `bridge0`, `veth`, `vmnet`, … | Container fingerprinting |

Loopback interfaces and interfaces that are down are always excluded.

### Output formats

```go
machineid.New().WithCPU().WithSystemUUID().WithFormat(machineid.Format32)   // 32 hex chars
machineid.New().WithCPU().WithSystemUUID().WithFormat(machineid.Format64)   // 64, default
machineid.New().WithCPU().WithSystemUUID().WithFormat(machineid.Format128)  // 128
machineid.New().WithCPU().WithSystemUUID().WithFormat(machineid.Format256)  // 256
```

| Format | Length | Bits | Collision probability at 10⁹ IDs | Use case |
|--------|--------|------|----------------------------------|----------|
| `Format32` | 32 | 128 | ~1.5 × 10⁻²¹ | Compact identifiers |
| `Format64` | 64 | 256 | ~4.3 × 10⁻⁶⁰ | **Default, recommended** |
| `Format128` | 128 | 512 | Effectively zero | Extended margin |
| `Format256` | 256 | 1024 | Effectively zero | Maximum margin |

### Salt

A salt makes the same machine produce a different ID for each application:

```go
id, err := machineid.New().
    WithCPU().
    WithSystemUUID().
    WithSalt("my-app-v1").
    ID(ctx)
```

### VM-friendly preset

Virtual machines and cloud instances change disks, MACs and sometimes motherboards under you. The preset keeps only the CPU and the system UUID:

```go
id, err := machineid.New().VMFriendly().WithSalt("my-app").ID(ctx)
```

### Validation

```go
provider := machineid.New().WithCPU().WithSystemUUID()
valid, err := provider.Validate(ctx, storedID)
```

### Diagnostics

```go
provider := machineid.New().WithCPU().WithSystemUUID().WithDisk()
id, _ := provider.ID(ctx)

diag := provider.Diagnostics() // a copy; modify freely
fmt.Println("Collected:", diag.Collected) // [cpu uuid]
fmt.Println("Errors:", diag.Errors)       // map[disk:component "disk": no values found]
```

`Collected` is always in the same order for the same configuration, regardless of which query finished first.

### Logging

Any `*slog.Logger` works, including `slog.Default()`. Without a logger there is no overhead at all.

```go
logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

id, err := machineid.New().
    WithCPU().
    WithSystemUUID().
    WithLogger(logger).
    ID(ctx)
```

| Level | What is logged |
|-------|----------------|
| **Info** | Component collected, fallback taken, ID generation lifecycle |
| **Warn** | Component failed or returned an empty value |
| **Debug** | Every command with arguments and duration, raw hardware values, reuse of cached command output |

### Timeouts and cancellation

Every system command runs under the context you pass to `ID` plus its own five second timeout. A context that is already done is rejected before anything runs. When a command is killed by the deadline or by cancellation, the recorded error wraps the context error:

```go
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
defer cancel()

_, err := provider.ID(ctx)
if errors.Is(err, context.DeadlineExceeded) {
    // a hardware query took too long
}
```

### Error handling

Sentinel errors for `errors.Is`:

| Error | Meaning |
|-------|---------|
| `ErrNoIdentifiers` | No component produced a value with the current configuration |
| `ErrEmptyValue` | A component returned an empty value |
| `ErrNoValues` | A multi-value component (MAC, disk) returned nothing |
| `ErrNotFound` | The value was missing from command output or system files |
| `ErrOEMPlaceholder` | The value is a BIOS placeholder such as "To be filled by O.E.M." |
| `ErrAllMethodsFailed` | Every source for a component failed |

Typed errors for `errors.AsType`:

```go
// A system command failed
if cmdErr, ok := errors.AsType[*machineid.CommandError](err); ok {
    fmt.Println("command:", cmdErr.Command) // "sysctl", "ioreg", "wmic", "powershell", …
    fmt.Println("stderr:", cmdErr.Stderr)   // first line of stderr, if any
}

// Output could not be parsed
if parseErr, ok := errors.AsType[*machineid.ParseError](err); ok {
    fmt.Println("source:", parseErr.Source) // "system_profiler hardware JSON", …
}

// Per-component cause from the diagnostics
if compErr, ok := errors.AsType[*machineid.ComponentError](diag.Errors["cpu"]); ok {
    fmt.Println("component:", compErr.Component)
    fmt.Println("cause:", compErr.Err)
}
```

---

## ⚙️ How it works

1. **Collect** every enabled component, concurrently, one goroutine per component.
2. **Validate** each value. Malformed, nil and max UUIDs and OEM placeholder serials are discarded and the next source is tried.
3. **Sort** the collected `prefix:value` strings so the order of collection never matters.
4. **Hash** the joined string (with the salt, if any) with SHA-256.
5. **Format** to the requested power-of-two length.

### Platform sources

| Platform | CPU | System UUID | Motherboard | Disk | MAC |
|----------|-----|-------------|-------------|------|-----|
| 🍎 **macOS** | `sysctl`, `system_profiler` | `system_profiler`, `ioreg` | `system_profiler`, `ioreg` | `system_profiler` | `net.Interfaces` |
| 🐧 **Linux** | `/proc/cpuinfo` | `/sys/class/dmi/id`, `/etc/machine-id` | `/sys/class/dmi/id` | `lsblk`, `/sys/block` | `net.Interfaces` |
| 🪟 **Windows** | `wmic`, PowerShell | `wmic`, PowerShell | `wmic`, PowerShell | `wmic`, PowerShell | `net.Interfaces` |

Every source has a fallback. Some platform specifics worth knowing:

- **macOS**: `system_profiler SPHardwareDataType` is the slowest query and is shared by the UUID, serial and CPU collectors, so it runs once per ID.
- **Windows**: `wmic` was removed from Windows 11 24H2 and Windows Server 2025. The library checks for it once and goes straight to `Get-CimInstance` when it is absent. PowerShell always runs with `-NoProfile -NonInteractive`, so user profiles cannot alter the output.
- **Linux**: no processes are spawned except `lsblk` for disk serials; everything else is read from `/proc` and `/sys`.

### Performance

All queries overlap, so an ID costs roughly the slowest single query. On a MacBook Pro the CLI with `-all` completes in about 0.2 seconds. Windows is dominated by PowerShell start-up and typically finishes in one to three seconds.

---

## 🧭 Choosing components

The right mix depends on how stable you need the ID to be versus how unique.

| Profile | Configuration | Notes |
|---------|---------------|-------|
| **VMs and containers** | `VMFriendly()` | CPU + UUID. Survives disk and NIC changes. |
| **Balanced (recommended)** | `WithCPU().WithSystemUUID().WithMotherboard()` | The CLI default. Stable across reboots and OS reinstalls. |
| **Maximum uniqueness** | `WithCPU().WithSystemUUID().WithMotherboard().WithMAC().WithDisk()` | Changes when a NIC or disk is swapped. |

### What changes an ID

Be deliberate about which of these your users are likely to do.

| Event | `cpu` | `uuid` | `motherboard` | `mac` | `disk` |
|-------|:-----:|:------:|:-------------:|:-----:|:------:|
| Reboot, OS reinstall | – | – | – | – | – |
| Replace the motherboard | ✅ | ✅ | ✅ | – | – |
| Replace or add a NIC | – | – | – | ✅ | – |
| Replace, add or remove a disk | – | – | – | – | ✅ |
| Kernel or microcode update (Linux) | ⚠️ | – | – | – | – |
| VM migration or resize | ⚠️ | ⚠️ | ⚠️ | ✅ | ✅ |

⚠️ On Linux the CPU identifier includes the kernel's CPU `flags` line, which can gain entries after a kernel or microcode update. On virtual machines the hypervisor decides how stable the UUID and motherboard serial are. If either matters to you, prefer `VMFriendly()` plus a salt, or drop `WithCPU()` on Linux.

---

## 🔒 Security

- 🔐 SHA-256 is a one-way hash. An ID cannot be reversed into serial numbers, MAC addresses or any other hardware detail.
- 🧂 Salting prevents one application's IDs from being reused by another.
- 🪪 The output contains no personally identifiable information.
- ⏱️ Every system command has a timeout and is killed, together with its output pipes, when the context ends.
- 🛡️ Firmware sentinels (nil and max UUIDs, "To be filled by O.E.M.") are rejected so they can never make two different machines share an ID.
- 🔏 Release binaries are signed: Apple Developer ID plus notarization on macOS, Sigstore keyless signatures on Linux. `machineid update` verifies both before installing.
- 📴 The tool never makes a network request unless you run `machineid update`. There is no telemetry and no background update check.

Please report vulnerabilities as described in [SECURITY.md](SECURITY.md).

---

## 🧪 Testing

Inject a `CommandExecutor` to run the library against fixtures instead of real commands. Executors must be safe for concurrent use, because components are collected in parallel.

```go
type fakeExecutor struct {
    mu      sync.RWMutex
    outputs map[string]string
}

func (f *fakeExecutor) Execute(ctx context.Context, name string, args ...string) (string, error) {
    f.mu.RLock()
    defer f.mu.RUnlock()
    if out, ok := f.outputs[name]; ok {
        return out, nil
    }
    return "", fmt.Errorf("command not configured: %s", name)
}

provider := machineid.New().
    WithExecutor(&fakeExecutor{outputs: map[string]string{"sysctl": "Intel Core i9"}}).
    WithCPU()

id, err := provider.ID(ctx)
```

Run the suite:

```bash
go test -race ./...
```

Runnable examples are listed with `go doc -ex github.com/slashdevops/machineid`.

---

## 🛠️ Troubleshooting

**`ErrNoIdentifiers` on a VM or in a container.** The hypervisor or runtime is hiding the hardware. Run `machineid -all -diagnostics -debug` to see which sources fail, then use `VMFriendly()` or a subset that works in that environment.

**The ID changed after a Linux kernel update.** See [What changes an ID](#what-changes-an-id). The CPU flags line moved. Drop `WithCPU()` on Linux or switch to `VMFriendly()`.

**Windows is slow.** PowerShell start-up dominates. Make sure you are on a build where `wmic` is either present or cleanly absent; the library handles both. Use `-debug` to see per-command timing.

**Git tag rejected: "push declined due to repository rule violations".** The repository enforces ascending semantic versions. Check `git tag -l` and tag a version higher than every existing one.

---

## 🤝 Contributing

Contributions are welcome. [CONTRIBUTING.md](CONTRIBUTING.md) covers the toolchain, code style, testing and the release process. Please open an issue before large changes so we can agree on the approach.

---

## 📄 License

[Apache License 2.0](LICENSE)
