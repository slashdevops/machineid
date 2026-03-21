//go:build linux

package machineid_test

import (
	"context"
	"fmt"

	"github.com/slashdevops/machineid"
)

// ExampleProvider_Diagnostics demonstrates inspecting which hardware components
// were successfully collected on Linux.
func ExampleProvider_Diagnostics() {
	provider := machineid.New().
		WithCPU().
		WithSystemUUID()

	//nolint:errcheck // Example: error handling omitted for brevity
	provider.ID(context.Background())

	diag := provider.Diagnostics()
	if diag == nil {
		fmt.Println("no diagnostics")
		return
	}

	// On Linux, the number of collected components depends on system access:
	//   /proc/cpuinfo is always readable (cpu)
	//   /sys/class/dmi/id/product_uuid may require root (uuid)
	//   /etc/machine-id is usually readable (machine-id)
	fmt.Printf("Has collected data: %v\n", len(diag.Collected) > 0)
	fmt.Printf("At least one component: %v\n", len(diag.Collected) >= 1)
	// Output:
	// Has collected data: true
	// At least one component: true
}

// Example_integrity demonstrates that salt produces different IDs on the
// same hardware, and that the same configuration is consistent across calls.
func Example_integrity() {
	// Salt-based differentiation works regardless of hardware access
	p1 := machineid.New().WithCPU().WithSystemUUID().WithSalt("app1")
	p2 := machineid.New().WithCPU().WithSystemUUID().WithSalt("app2")

	id1, _ := p1.ID(context.Background()) //nolint:errcheck // Example
	id2, _ := p2.ID(context.Background()) //nolint:errcheck // Example

	// Same configuration always produces same ID
	id1Again, _ := machineid.New().WithCPU().WithSystemUUID().WithSalt("app1").ID(context.Background()) //nolint:errcheck // Example
	fmt.Printf("Consistency: %v\n", id1 == id1Again)

	// Different salts produce different IDs
	fmt.Printf("Different salts: %v\n", id1 != id2)

	// All IDs are 64 characters (power of 2)
	fmt.Printf("All are 64 chars: %v\n", len(id1) == 64 && len(id2) == 64)

	// Output:
	// Consistency: true
	// Different salts: true
	// All are 64 chars: true
}

// Example_linuxFileSources shows that Linux reads hardware data from
// filesystem paths rather than spawning external commands.
func Example_linuxFileSources() {
	// On Linux, most hardware identifiers are read directly from /proc and /sys:
	//   CPU:         /proc/cpuinfo
	//   UUID:        /sys/class/dmi/id/product_uuid
	//   Machine ID:  /etc/machine-id
	//   Motherboard: /sys/class/dmi/id/board_serial
	//   Disk:        lsblk + /sys/block/*/device/serial
	//
	// File reads are fast — no process startup overhead.
	// Using CPU only since /proc/cpuinfo is always readable.
	provider := machineid.New().
		WithCPU()

	id, err := provider.ID(context.Background())
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	fmt.Printf("ID length: %d\n", len(id))
	// Output:
	// ID length: 64
}
