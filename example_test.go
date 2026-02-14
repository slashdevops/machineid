package machineid_test

import (
	"context"
	"fmt"

	"github.com/slashdevops/machineid"
)

// ExampleNew demonstrates the simplest way to generate a machine ID.
func ExampleNew() {
	provider := machineid.New().
		WithCPU().
		WithSystemUUID()

	id, err := provider.ID(context.Background())
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	fmt.Printf("ID length: %d\n", len(id))
	// Output:
	// ID length: 64
}

// ExampleProvider_WithSalt shows how a salt produces application-specific IDs.
func ExampleProvider_WithSalt() {
	ctx := context.Background()

	id1, _ := machineid.New().
		WithCPU().
		WithSystemUUID().
		WithSalt("app-one").
		ID(ctx)

	id2, _ := machineid.New().
		WithCPU().
		WithSystemUUID().
		WithSalt("app-two").
		ID(ctx)

	fmt.Printf("Same length: %v\n", len(id1) == len(id2))
	fmt.Printf("Different IDs: %v\n", id1 != id2)
	// Output:
	// Same length: true
	// Different IDs: true
}

// ExampleProvider_WithFormat demonstrates the four power-of-two output formats.
func ExampleProvider_WithFormat() {
	ctx := context.Background()
	base := func(mode machineid.FormatMode) int {
		id, err := machineid.New().
			WithCPU().
			WithSystemUUID().
			WithFormat(mode).
			ID(ctx)
		if err != nil {
			return -1
		}
		return len(id)
	}

	fmt.Printf("Format32:  %d chars\n", base(machineid.Format32))
	fmt.Printf("Format64:  %d chars\n", base(machineid.Format64))
	fmt.Printf("Format128: %d chars\n", base(machineid.Format128))
	fmt.Printf("Format256: %d chars\n", base(machineid.Format256))
	// Output:
	// Format32:  32 chars
	// Format64:  64 chars
	// Format128: 128 chars
	// Format256: 256 chars
}

// ExampleProvider_Validate shows how to check a stored ID against the current machine.
func ExampleProvider_Validate() {
	provider := machineid.New().
		WithCPU().
		WithSystemUUID()

	id, _ := provider.ID(context.Background())

	// Validate the correct ID
	valid, _ := provider.Validate(context.Background(), id)
	fmt.Printf("Correct ID valid: %v\n", valid)

	// Validate an incorrect ID
	valid, _ = provider.Validate(context.Background(), "0000000000000000000000000000000000000000000000000000000000000000")
	fmt.Printf("Wrong ID valid: %v\n", valid)

	// Output:
	// Correct ID valid: true
	// Wrong ID valid: false
}

// ExampleProvider_VMFriendly_preset demonstrates the VM-friendly preset.
func ExampleProvider_VMFriendly_preset() {
	id, err := machineid.New().
		VMFriendly().
		ID(context.Background())
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	fmt.Printf("VM-friendly ID length: %d\n", len(id))
	// Output:
	// VM-friendly ID length: 64
}

// ExampleProvider_ID_allComponents shows using every available hardware source.
func ExampleProvider_ID_allComponents() {
	provider := machineid.New().
		WithCPU().
		WithMotherboard().
		WithSystemUUID().
		WithMAC().
		WithDisk().
		WithSalt("full-example")

	id, err := provider.ID(context.Background())
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	fmt.Printf("Full ID length: %d\n", len(id))
	fmt.Printf("Is hex: %v\n", isAllHex(id))
	// Output:
	// Full ID length: 64
	// Is hex: true
}

func isAllHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return len(s) > 0
}
