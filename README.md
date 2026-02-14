# machineid

[![main branch](https://github.com/slashdevops/machineid/actions/workflows/main.yml/badge.svg)](https://github.com/slashdevops/machineid/actions/workflows/main.yml)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod-go-version/slashdevops/machineid?style=plastic)
[![Go Reference](https://pkg.go.dev/badge/github.com/slashdevops/machineid.svg)](https://pkg.go.dev/github.com/slashdevops/machineid)
[![Go Report Card](https://goreportcard.com/badge/github.com/slashdevops/machineid)](https://goreportcard.com/report/github.com/slashdevops/machineid)
[![license](https://img.shields.io/github/license/slashdevops/machineid.svg)](https://github.com/slashdevops/machineid/blob/main/LICENSE)
[![Release](https://github.com/slashdevops/machineid/actions/workflows/release.yml/badge.svg)](https://github.com/slashdevops/machineid/actions/workflows/release.yml)

A **zero-dependency** Go library that generates unique, deterministic machine identifiers from hardware characteristics. IDs are stable across reboots, sensitive to hardware changes, and ideal for software licensing, device fingerprinting, and telemetry correlation.

## Features

- **Zero Dependencies** — built entirely on the Go standard library
- **Cross-Platform** — macOS, Linux, and Windows
- **Configurable** — choose which hardware signals to include (CPU, Motherboard, System UUID, MAC, Disk)
- **Power-of-2 Output** — 32, 64, 128, or 256 hex characters
- **SHA-256 Hashing** — cryptographically secure, no collisions in practice
- **Salt Support** — application-specific IDs on the same machine
- **VM Friendly** — preset for virtual environments (CPU + UUID)
- **Thread-Safe** — safe for concurrent use after configuration
- **Diagnostic API** — inspect which components succeeded or failed
- **Optional Logging** — `*slog.Logger` support for observability with zero overhead when disabled
- **Testable** — dependency-injectable command executor

## Installation

### Library

Add the module to your Go project:

```bash
go get github.com/slashdevops/machineid
```

Requires **Go 1.25+**. No external dependencies.

### CLI Tool

#### Using `go install`

```bash
go install github.com/slashdevops/machineid/cmd/machineid@latest
```

Make sure `~/go/bin` is in your `PATH`:

```bash
mkdir -p ~/go/bin

# bash
cat >> ~/.bash_profile <<EOL
export PATH=\$PATH:~/go/bin
EOL

source ~/.bash_profile

# zsh
cat >> ~/.zshrc <<EOL
export PATH=\$PATH:~/go/bin
EOL

source ~/.zshrc
```

#### Installing a Precompiled Binary

Precompiled binaries for macOS, Linux, and Windows are available on the [releases page](https://github.com/slashdevops/machineid/releases).

You can download them with the [GitHub CLI](https://cli.github.com/manual/installation) (`gh`):

```bash
brew install gh   # if not already installed
```

Then fetch and install the binary:

```bash
export TOOL_NAME="machineid"
export GIT_ORG="slashdevops"
export GIT_REPO="machineid"
export OS=$(uname -s | tr '[:upper:]' '[:lower:]')
export OS_ARCH=$(uname -m | tr '[:upper:]' '[:lower:]')
export ASSETS_NAME=$(gh release view --repo ${GIT_ORG}/${GIT_REPO} --json assets -q "[.assets[] | select(.name | contains(\"${TOOL_NAME}\") and contains(\"${OS}\") and contains(\"${OS_ARCH}\"))] | sort_by(.createdAt) | last.name")
export APP_NAME="${ASSETS_NAME%.*}"

gh release download --repo $GIT_ORG/$GIT_REPO --pattern $ASSETS_NAME
unzip $ASSETS_NAME
mv $APP_NAME $TOOL_NAME
rm $ASSETS_NAME

mv $TOOL_NAME ~/go/bin/$TOOL_NAME
~/go/bin/$TOOL_NAME -version
```

#### Building from Source

Clone the repository and build with version metadata via the provided Makefile:

```bash
git clone https://github.com/slashdevops/machineid.git
cd machineid
make build
./build/machineid -version
```

## Quick Start

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
    fmt.Println(id)
    // Output: 64-character hex string (e.g. b5c42832542981af…)
}
```

## Usage

### Selecting Hardware Components

Enable one or more hardware sources via the `With*` methods:

```go
ctx := context.Background()
provider := machineid.New().
    WithCPU().            // processor ID and feature flags
    WithMotherboard().    // motherboard serial number
    WithSystemUUID().     // BIOS/UEFI system UUID
    WithMAC().            // physical network interface MAC addresses
    WithDisk()            // internal disk serial numbers

id, err := provider.ID(ctx)
```

### Output Formats

All formats produce pure hexadecimal strings without dashes:

```go
ctx := context.Background()

// 32 characters (2^5) — compact
id, _ := machineid.New().WithCPU().WithSystemUUID().WithFormat(machineid.Format32).ID(ctx)

// 64 characters (2^6) — default, full SHA-256
id, _ = machineid.New().WithCPU().WithSystemUUID().WithFormat(machineid.Format64).ID(ctx)

// 128 characters (2^7) — extended
id, _ = machineid.New().WithCPU().WithSystemUUID().WithFormat(machineid.Format128).ID(ctx)

// 256 characters (2^8) — maximum
id, _ = machineid.New().WithCPU().WithSystemUUID().WithFormat(machineid.Format256).ID(ctx)
```

| Format | Length | Bits | Collision Probability (1 B IDs) | Use Case |
|-----------|--------|------|--------------------------------|----------------------|
| `Format32`  | 32     | 128  | ~1.47 × 10⁻²¹                 | Compact identifiers  |
| `Format64`  | 64     | 256  | ~4.32 × 10⁻⁶⁰                 | Default, recommended |
| `Format128` | 128    | 512  | Virtually zero                 | Extended security    |
| `Format256` | 256    | 1024 | Astronomically low             | Maximum security     |

### Custom Salt

A salt ensures the same machine produces different IDs for different applications:

```go
ctx := context.Background()
id, _ := machineid.New().
    WithCPU().
    WithSystemUUID().
    WithSalt("my-app-v1").
    ID(ctx)
```

### VM-Friendly Mode

For virtual machines where disk serials and MACs may be unstable:

```go
ctx := context.Background()
id, _ := machineid.New().
    VMFriendly().  // CPU + System UUID only
    WithSalt("my-app").
    ID(ctx)
```

### Validation

Check whether a stored ID still matches the current hardware:

```go
ctx := context.Background()
provider := machineid.New().WithCPU().WithSystemUUID()
valid, err := provider.Validate(ctx, storedID)
```

### Diagnostics

Inspect which hardware components were successfully collected:

```go
ctx := context.Background()
provider := machineid.New().
    WithCPU().
    WithSystemUUID().
    WithDisk()

id, _ := provider.ID(ctx)

diag := provider.Diagnostics()
fmt.Println("Collected:", diag.Collected)  // e.g. [cpu uuid]
fmt.Println("Errors:", diag.Errors)        // e.g. map[disk: no internal disk identifiers found]
```

### Logging

Enable optional logging with any `*slog.Logger` for observability. When no logger is set (the default), there is zero overhead:

```go
import (
    "log/slog"
    "os"
)

ctx := context.Background()
logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
    Level: slog.LevelDebug,
}))

id, err := machineid.New().
    WithCPU().
    WithSystemUUID().
    WithLogger(logger).
    ID(ctx)
```

| Log Level | What's Logged                                                         |
|-----------|-----------------------------------------------------------------------|
| **Info**  | Component collected, fallback triggered, ID generation lifecycle      |
| **Warn**  | Component failed or returned empty value                              |
| **Debug** | Command execution details (name, args, duration), raw hardware values |

The logger is compatible with `slog.Default()` which bridges to the standard `log` package:

```go
// Use the standard library default logger
provider.WithLogger(slog.Default())
```

## CLI Tool

A ready-to-use command-line tool is included.

See the [Installation](#installation) section above for all ways to install the CLI.

### Examples

```bash
# Generate an ID from CPU + UUID (default 64 chars)
machineid -cpu -uuid

# All hardware sources, compact 32-char format
machineid -all -format 32

# VM-friendly with custom salt
machineid -vm -salt "my-app"

# JSON output with diagnostics
machineid -cpu -uuid -json -diagnostics

# Validate a previously stored ID
machineid -cpu -uuid -validate "b5c42832542981af58c9dc3bc241219e780ff7d276cfad05fac222846edb84f7"

# Version information
machineid -version
machineid -version.long
```

### All Flags

| Flag            | Description                                          |
|-----------------|------------------------------------------------------|
| `-cpu`          | Include CPU identifier                               |
| `-motherboard`  | Include motherboard serial number                    |
| `-uuid`         | Include system UUID                                  |
| `-mac`          | Include network MAC addresses                        |
| `-disk`         | Include disk serial numbers                          |
| `-all`          | Include all hardware identifiers                     |
| `-vm`           | VM-friendly mode (CPU + UUID only)                   |
| `-format N`     | Output length: `32`, `64` (default), `128`, or `256` |
| `-salt STRING`  | Custom salt for application-specific IDs             |
| `-validate ID`  | Validate an ID against the current machine           |
| `-diagnostics`  | Show collected/failed components                     |
| `-json`         | Output as JSON                                       |
| `-version`      | Show version information                             |
| `-version.long` | Show detailed version information                    |

## How It Works

1. **Collect** — gather hardware identifiers based on the provider configuration
2. **Sort** — sort identifiers alphabetically for deterministic ordering
3. **Hash** — apply SHA-256 to the concatenated identifiers (with optional salt)
4. **Format** — truncate or extend the hash to the selected power-of-2 length

### Platform Details

| Platform | CPU | UUID | Motherboard | Disk | MAC |
|----------|-----|------|-------------|------|-----|
| **macOS** | `sysctl`, `system_profiler` | `system_profiler`, `ioreg` | `system_profiler`, `ioreg` | `system_profiler` | `net.Interfaces` |
| **Linux** | `/proc/cpuinfo` | `/sys/class/dmi/id`, `/etc/machine-id` | `/sys/class/dmi/id` | `lsblk`, `/sys/block` | `net.Interfaces` |
| **Windows** | `wmic`, `PowerShell` | `wmic`, `PowerShell` | `wmic`, `PowerShell` | `wmic`, `PowerShell` | `net.Interfaces` |

Each source has fallback methods for resilience across OS versions and configurations.

## Testing

The library supports dependency injection for deterministic testing without real system commands:

```go
type mockExecutor struct {
    outputs map[string]string
}

func (m *mockExecutor) Execute(ctx context.Context, name string, args ...string) (string, error) {
    if output, ok := m.outputs[name]; ok {
        return output, nil
    }
    return "", fmt.Errorf("command not found: %s", name)
}

provider := machineid.New().
    WithExecutor(&mockExecutor{
        outputs: map[string]string{
            "sysctl": "Intel Core i9",
        },
    }).
    WithCPU()

id, err := provider.ID()
```

Run the test suite:

```bash
go test -v -race ./...
```

## Security Considerations

- SHA-256 is a cryptographically secure one-way hash — hardware details cannot be recovered from an ID
- Sorting ensures consistent output regardless of collection order
- Salt support prevents cross-application ID reuse
- No personally identifiable information (PII) is exposed in the output

## Best Practices

### Choosing a Format

| Format | Recommendation |
|--------|----------------|
| `Format32` | Embedded systems or storage-constrained environments |
| `Format64` | **Recommended for most use cases** (default) |
| `Format128` | Extra security margin or regulatory requirements |
| `Format256` | Maximum security for critical applications |

### Hardware Identifier Selection

```go
// Minimal (VMs, containers)
id, _ := machineid.New().VMFriendly().ID()

// Balanced (recommended)
id, _ := machineid.New().
    WithCPU().
    WithSystemUUID().
    WithMotherboard().
    ID()

// Maximum (most unique, but sensitive to hardware changes)
id, _ := machineid.New().
    WithCPU().
    WithSystemUUID().
    WithMotherboard().
    WithMAC().
    WithDisk().
    ID()
```

## Contributing

Contributions are welcome. Please open an issue or submit a pull request.

## License

[Apache License 2.0](LICENSE)
