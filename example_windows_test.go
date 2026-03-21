//go:build windows

package machineid_test

import (
	"context"
	"fmt"

	"github.com/slashdevops/machineid"
)

// ExampleProvider_Diagnostics demonstrates inspecting which hardware components
// were successfully collected on Windows.
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

	fmt.Printf("Components collected: %d\n", len(diag.Collected))
	fmt.Printf("Has collected data: %v\n", len(diag.Collected) > 0)
	// Output:
	// Components collected: 2
	// Has collected data: true
}

// Example_integrity demonstrates that the format maintains integrity without collisions.
func Example_integrity() {
	p1 := machineid.New().WithCPU().WithSystemUUID()
	p2 := machineid.New().WithCPU().WithSystemUUID().WithMotherboard()
	p3 := machineid.New().WithCPU().WithSystemUUID().WithSalt("app1")
	p4 := machineid.New().WithCPU().WithSystemUUID().WithSalt("app2")

	id1, _ := p1.ID(context.Background()) //nolint:errcheck // Example
	id2, _ := p2.ID(context.Background()) //nolint:errcheck // Example
	id3, _ := p3.ID(context.Background()) //nolint:errcheck // Example
	id4, _ := p4.ID(context.Background()) //nolint:errcheck // Example

	// Same configuration always produces same ID
	id1Again, _ := machineid.New().WithCPU().WithSystemUUID().ID(context.Background()) //nolint:errcheck // Example
	fmt.Printf("Consistency: %v\n", id1 == id1Again)

	// Different configurations produce different IDs
	fmt.Printf("Different hardware: %v\n", id1 != id2)
	fmt.Printf("Different salts: %v\n", id3 != id4)

	// All IDs are 64 characters (power of 2)
	fmt.Printf("All are 64 chars: %v\n",
		len(id1) == 64 && len(id2) == 64 && len(id3) == 64 && len(id4) == 64)

	// Output:
	// Consistency: true
	// Different hardware: true
	// Different salts: true
	// All are 64 chars: true
}

// Example_concurrentCollection demonstrates that Windows collects hardware
// identifiers concurrently for better performance.
func Example_concurrentCollection() {
	// On Windows, all hardware queries (wmic/PowerShell) run in parallel,
	// reducing total latency from the sum of all commands to the max of
	// any single command.
	provider := machineid.New().
		WithCPU().
		WithMotherboard().
		WithSystemUUID().
		WithMAC().
		WithDisk()

	id, err := provider.ID(context.Background())
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	fmt.Printf("ID length: %d\n", len(id))
	// Output:
	// ID length: 64
}
