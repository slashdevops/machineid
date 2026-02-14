package machineid_test

import (
	"context"
	"fmt"
	"math"

	"github.com/slashdevops/machineid"
)

// Example_powerOfTwo demonstrates why power-of-2 lengths are beneficial.
func Example_powerOfTwo() {
	// Format32: 32 hex chars = 128 bits = 2^128 possible values
	id32, _ := machineid.New().WithCPU().WithSystemUUID().WithFormat(machineid.Format32).ID(context.Background())
	fmt.Printf("Format32 (2^5 chars): %d characters\n", len(id32))
	fmt.Printf("Format32 bits: %d (2^%d possible values)\n", len(id32)*4, len(id32)*4)

	// Format64: 64 hex chars = 256 bits = 2^256 possible values (full SHA-256)
	id64, _ := machineid.New().WithCPU().WithSystemUUID().WithFormat(machineid.Format64).ID(context.Background())
	fmt.Printf("Format64 (2^6 chars): %d characters\n", len(id64))
	fmt.Printf("Format64 bits: %d (2^%d possible values)\n", len(id64)*4, len(id64)*4)

	// Output:
	// Format32 (2^5 chars): 32 characters
	// Format32 bits: 128 (2^128 possible values)
	// Format64 (2^6 chars): 64 characters
	// Format64 bits: 256 (2^256 possible values)
}

// Example_collisionResistance demonstrates the collision resistance of different formats.
func Example_collisionResistance() {
	// Calculate collision probability (simplified)
	format32Bits := 128.0 // 32 hex chars = 128 bits
	format64Bits := 256.0 // 64 hex chars = 256 bits

	// For random IDs, probability of collision after N IDs (birthday paradox):
	// P(collision) ≈ N^2 / (2 * 2^bits)
	// For no collision with 1 billion IDs:
	n := 1e9 // 1 billion IDs

	// Format32 (128 bits)
	collisionProb32 := (n * n) / (2 * math.Pow(2, format32Bits))
	fmt.Printf("Format32 collision probability with 1B IDs: %.2e\n", collisionProb32)

	// Format64 (256 bits) - essentially zero
	collisionProb64 := (n * n) / (2 * math.Pow(2, format64Bits))
	fmt.Printf("Format64 collision probability with 1B IDs: %.2e\n", collisionProb64)

	fmt.Printf("Format64 is more secure: %v\n", collisionProb64 < collisionProb32)

	// Output:
	// Format32 collision probability with 1B IDs: 1.47e-21
	// Format64 collision probability with 1B IDs: 4.32e-60
	// Format64 is more secure: true
}
