package machineid_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/slashdevops/machineid"
)

// ExampleProvider demonstrates basic usage of the machineid package.
// This example shows how to generate a unique machine identifier
// that is stable across reboots and suitable for licensing.
func ExampleProvider() {
	// Create a new provider with CPU and System UUID
	// These are typically stable identifiers on most systems
	provider := machineid.New().
		WithCPU().
		WithSystemUUID().
		WithSalt("my-application-v1")

	// Generate the machine ID
	id, err := provider.ID(context.Background())
	if err != nil {
		fmt.Printf("Error generating ID: %v\n", err)
		return
	}

	// The ID is a 64-character hexadecimal string (SHA-256 hash, power of 2)
	fmt.Printf("Machine ID length: %d\n", len(id))
	fmt.Printf("Machine ID is hexadecimal: %v\n", isHexString(id))

	// Validate the ID
	valid, err := provider.Validate(context.Background(), id)
	if err != nil {
		fmt.Printf("Error validating ID: %v\n", err)
		return
	}
	fmt.Printf("ID is valid: %v\n", valid)

	// Output:
	// Machine ID length: 64
	// Machine ID is hexadecimal: true
	// ID is valid: true
}

// ExampleProvider_VMFriendly demonstrates creating a VM-friendly machine ID.
// This configuration works well in virtual machine environments where
// hardware like disk serials and MAC addresses may change frequently.
func ExampleProvider_VMFriendly() {
	provider := machineid.New().
		VMFriendly().
		WithSalt("vm-app")

	id, err := provider.ID(context.Background())
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Generated VM-friendly ID: %v\n", len(id) == 64)
	// Output:
	// Generated VM-friendly ID: true
}

// isHexString reports whether s is a non-empty string of lowercase hex digits.
func isHexString(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func TestProviderBasic(t *testing.T) {
	g := machineid.New().WithCPU().WithSystemUUID().WithMotherboard().WithMAC().WithDisk()

	id, err := g.ID(context.Background())
	if err != nil {
		t.Fatalf("ID() error = %v", err)
	}

	if len(id) != 64 {
		t.Errorf("ID() returned ID of length %d, expected 64", len(id))
	}

	// Test consistency
	id2, err := g.ID(context.Background())
	if err != nil {
		t.Fatalf("ID() second call error = %v", err)
	}

	if id != id2 {
		t.Error("ID() returned different IDs on consecutive calls")
	}
}

func TestProviderWithSalt(t *testing.T) {
	salt := "test-salt"
	g := machineid.New().WithCPU().WithSystemUUID().WithSalt(salt)

	id, err := g.ID(context.Background())
	if err != nil {
		t.Fatalf("ID() with salt error = %v", err)
	}

	if len(id) != 64 {
		t.Errorf("ID() returned ID of length %d, expected 64", len(id))
	}

	// Different salts should produce different IDs
	g2 := machineid.New().WithCPU().WithSystemUUID().WithSalt("different-salt")
	id2, err := g2.ID(context.Background())
	if err != nil {
		t.Fatalf("ID() with different salt error = %v", err)
	}

	if id == id2 {
		t.Error("ID() should return different IDs for different salts")
	}
}

func TestProviderValidate(t *testing.T) {
	g := machineid.New().WithCPU().WithSystemUUID()

	id, err := g.ID(context.Background())
	if err != nil {
		t.Fatalf("ID() error = %v", err)
	}

	valid, err := g.Validate(context.Background(), id)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if !valid {
		t.Error("Validate() returned false for valid ID")
	}

	// Test with invalid ID
	valid, err = g.Validate(context.Background(), "invalid-id")
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if valid {
		t.Error("Validate() returned true for invalid ID")
	}
}

func TestVMFriendly(t *testing.T) {
	g := machineid.New().VMFriendly().WithSalt("vm-test")

	id, err := g.ID(context.Background())
	if err != nil {
		t.Fatalf("VMFriendly().ID() error = %v", err)
	}

	if len(id) != 64 {
		t.Errorf("VMFriendly().ID() returned ID of length %d, expected 64", len(id))
	}

	// Test that it's different from full hardware
	g2 := machineid.New().WithCPU().WithSystemUUID().WithMotherboard().WithMAC().WithDisk().WithSalt("vm-test")
	id2, err := g2.ID(context.Background())
	if err != nil {
		t.Fatalf("Full hardware ID() error = %v", err)
	}

	if id == id2 {
		t.Error("VMFriendly() should produce different ID from full hardware")
	}
}

func TestNoIdentifiersError(t *testing.T) {
	g := machineid.New() // No identifiers enabled

	_, err := g.ID(context.Background())
	if err == nil {
		t.Error("ID() should return error when no identifiers are enabled")
	}

	if !errors.Is(err, machineid.ErrNoIdentifiers) {
		t.Errorf("ID() error should be ErrNoIdentifiers, got %v", err)
	}
}

func TestProviderChaining(t *testing.T) {
	// Test that method chaining works
	g := machineid.New().
		WithSalt("chain-test").
		WithCPU().
		WithSystemUUID().
		WithMotherboard()

	id, err := g.ID(context.Background())
	if err != nil {
		t.Fatalf("Chained provider ID() error = %v", err)
	}

	if len(id) != 64 {
		t.Errorf("Chained provider ID() returned ID of length %d, expected 64", len(id))
	}

	// Verify it validates correctly
	valid, err := g.Validate(context.Background(), id)
	if err != nil {
		t.Fatalf("Chained provider Validate() error = %v", err)
	}

	if !valid {
		t.Error("Chained provider Validate() returned false for valid ID")
	}
}

// TestProviderConcurrency tests that ID() is safe for concurrent use.
func TestProviderConcurrency(t *testing.T) {
	g := machineid.New().WithCPU().WithSystemUUID()

	// Call ID() concurrently from multiple goroutines
	const numGoroutines = 10
	results := make(chan string, numGoroutines)
	errors := make(chan error, numGoroutines)

	for range numGoroutines {
		go func() {
			id, err := g.ID(context.Background())
			if err != nil {
				errors <- err
				return
			}
			results <- id
		}()
	}

	// Collect results
	var ids []string
	for range numGoroutines {
		select {
		case id := <-results:
			ids = append(ids, id)
		case err := <-errors:
			t.Fatalf("Concurrent ID() call failed: %v", err)
		}
	}

	// All IDs should be identical
	firstID := ids[0]
	for i, id := range ids {
		if id != firstID {
			t.Errorf("ID mismatch at index %d: got %s, want %s", i, id, firstID)
		}
	}
}

// TestValidateWithDifferentConfiguration tests validation behavior.
func TestValidateWithDifferentConfiguration(t *testing.T) {
	g1 := machineid.New().WithCPU().WithSystemUUID()
	id1, err := g1.ID(context.Background())
	if err != nil {
		t.Fatalf("g1.ID(context.Background()) error = %v", err)
	}

	// Same configuration should validate
	g2 := machineid.New().WithCPU().WithSystemUUID()
	valid, err := g2.Validate(context.Background(), id1)
	if err != nil {
		t.Fatalf("g2.Validate() error = %v", err)
	}

	if !valid {
		t.Error("Same configuration should produce valid ID")
	}

	// Different configuration should not validate
	g3 := machineid.New().WithCPU() // Missing SystemUUID
	valid, err = g3.Validate(context.Background(), id1)
	if err != nil {
		t.Fatalf("g3.Validate() error = %v", err)
	}

	if valid {
		t.Error("Different configuration should not validate")
	}
}

// TestFormat32 tests the 32-character format (2^5).
func TestFormat32(t *testing.T) {
	g := machineid.New().
		WithCPU().
		WithSystemUUID().
		WithFormat(machineid.Format32)

	id, err := g.ID(context.Background())
	if err != nil {
		t.Fatalf("Format32 ID() error = %v", err)
	}

	if len(id) != 32 {
		t.Errorf("Format32 ID() returned ID of length %d, expected 32", len(id))
	}

	// Verify it's all hex characters
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Errorf("Format32 ID contains non-hex character: %c", c)
		}
	}
}

// TestFormat64 tests the 64-character format (2^6) - default.
func TestFormat64(t *testing.T) {
	g := machineid.New().
		WithCPU().
		WithSystemUUID().
		WithFormat(machineid.Format64)

	id, err := g.ID(context.Background())
	if err != nil {
		t.Fatalf("Format64 ID() error = %v", err)
	}

	if len(id) != 64 {
		t.Errorf("Format64 ID() returned ID of length %d, expected 64", len(id))
	}

	// Verify it's all hex characters (no dashes)
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Errorf("Format64 ID contains non-hex character: %c", c)
		}
	}
}

// TestFormat128 tests the 128-character format (2^7).
func TestFormat128(t *testing.T) {
	g := machineid.New().
		WithCPU().
		WithSystemUUID().
		WithFormat(machineid.Format128)

	id, err := g.ID(context.Background())
	if err != nil {
		t.Fatalf("Format128 ID() error = %v", err)
	}

	if len(id) != 128 {
		t.Errorf("Format128 ID() returned ID of length %d, expected 128", len(id))
	}

	// Verify it's all hex characters
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Errorf("Format128 ID contains non-hex character: %c", c)
		}
	}
}

// TestFormat256 tests the 256-character format (2^8).
func TestFormat256(t *testing.T) {
	g := machineid.New().
		WithCPU().
		WithSystemUUID().
		WithFormat(machineid.Format256)

	id, err := g.ID(context.Background())
	if err != nil {
		t.Fatalf("Format256 ID() error = %v", err)
	}

	if len(id) != 256 {
		t.Errorf("Format256 ID() returned ID of length %d, expected 256", len(id))
	}

	// Verify it's all hex characters
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Errorf("Format256 ID contains non-hex character: %c", c)
		}
	}
}

// TestFormatDifference tests that different formats produce different outputs.
func TestFormatDifference(t *testing.T) {
	// Create providers with same config but different formats
	g32 := machineid.New().WithCPU().WithSystemUUID().WithFormat(machineid.Format32)
	g64 := machineid.New().WithCPU().WithSystemUUID().WithFormat(machineid.Format64)
	g128 := machineid.New().WithCPU().WithSystemUUID().WithFormat(machineid.Format128)

	id32, _ := g32.ID(context.Background())
	id64, _ := g64.ID(context.Background())
	id128, _ := g128.ID(context.Background())

	// Format32 should be the first 32 chars of Format64
	if id32 != id64[:32] {
		t.Error("Format32 should be first 32 chars of Format64")
	}

	// Format64 should be the first 64 chars of Format128
	if id64 != id128[:64] {
		t.Error("Format64 should be first 64 chars of Format128")
	}
}

// TestFormatDefault tests that the default format is Format64.
func TestFormatDefault(t *testing.T) {
	// Create provider without explicit format
	g := machineid.New().WithCPU().WithSystemUUID()

	id, err := g.ID(context.Background())
	if err != nil {
		t.Fatalf("Default ID() error = %v", err)
	}

	// Default should be 64 characters
	if len(id) != 64 {
		t.Errorf("Default format should be 64 characters, got %d", len(id))
	}

	// Should match explicit Format64
	g64 := machineid.New().WithCPU().WithSystemUUID().WithFormat(machineid.Format64)
	id64, err := g64.ID(context.Background())
	if err != nil {
		t.Fatalf("Format64 ID() error = %v", err)
	}

	if id != id64 {
		t.Error("Default format should match Format64")
	}
}

// TestFormatConsistency tests that each format produces consistent results.
func TestFormatConsistency(t *testing.T) {
	formats := []struct {
		name   string
		format machineid.FormatMode
		length int
	}{
		{"Format32", machineid.Format32, 32},
		{"Format64", machineid.Format64, 64},
		{"Format128", machineid.Format128, 128},
		{"Format256", machineid.Format256, 256},
	}

	for _, tc := range formats {
		t.Run(tc.name, func(t *testing.T) {
			g := machineid.New().
				WithCPU().
				WithSystemUUID().
				WithFormat(tc.format)

			// Generate ID multiple times
			id1, err := g.ID(context.Background())
			if err != nil {
				t.Fatalf("%s first ID() error = %v", tc.name, err)
			}

			id2, err := g.ID(context.Background())
			if err != nil {
				t.Fatalf("%s second ID() error = %v", tc.name, err)
			}

			// Should be identical (cached)
			if id1 != id2 {
				t.Errorf("%s should produce consistent results, got different IDs", tc.name)
			}

			// Should have correct length
			if len(id1) != tc.length {
				t.Errorf("%s should have length %d, got %d", tc.name, tc.length, len(id1))
			}
		})
	}
}

// TestFormatNoDashes tests that all formats produce output without dashes.
func TestFormatNoDashes(t *testing.T) {
	formats := []struct {
		name   string
		format machineid.FormatMode
	}{
		{"Format32", machineid.Format32},
		{"Format64", machineid.Format64},
		{"Format128", machineid.Format128},
		{"Format256", machineid.Format256},
	}

	for _, tc := range formats {
		t.Run(tc.name, func(t *testing.T) {
			g := machineid.New().
				WithCPU().
				WithSystemUUID().
				WithFormat(tc.format)

			id, err := g.ID(context.Background())
			if err != nil {
				t.Fatalf("%s ID() error = %v", tc.name, err)
			}

			// Check for dashes
			for i, c := range id {
				if c == '-' {
					t.Errorf("%s should not contain dashes, found dash at position %d", tc.name, i)
				}
			}
		})
	}
}

// TestFormatWithDifferentHardware tests formats with various hardware configurations.
func TestFormatWithDifferentHardware(t *testing.T) {
	configs := []struct {
		name  string
		setup func(*machineid.Provider) *machineid.Provider
	}{
		{
			name: "CPU only",
			setup: func(p *machineid.Provider) *machineid.Provider {
				return p.WithCPU()
			},
		},
		{
			name: "UUID only",
			setup: func(p *machineid.Provider) *machineid.Provider {
				return p.WithSystemUUID()
			},
		},
		{
			name: "CPU + UUID",
			setup: func(p *machineid.Provider) *machineid.Provider {
				return p.WithCPU().WithSystemUUID()
			},
		},
		{
			name: "All hardware",
			setup: func(p *machineid.Provider) *machineid.Provider {
				return p.WithCPU().WithSystemUUID().WithMotherboard().WithMAC().WithDisk()
			},
		},
	}

	formats := []machineid.FormatMode{
		machineid.Format32,
		machineid.Format64,
		machineid.Format128,
		machineid.Format256,
	}

	for _, config := range configs {
		for _, format := range formats {
			t.Run(fmt.Sprintf("%s_Format%d", config.name, []int{32, 64, 128, 256}[format]), func(t *testing.T) {
				p := machineid.New()
				p = config.setup(p)
				p = p.WithFormat(format)

				id, err := p.ID(context.Background())
				if err != nil {
					t.Fatalf("ID() error = %v", err)
				}

				// Verify it's not empty
				if id == "" {
					t.Error("ID should not be empty")
				}

				// Verify it's hexadecimal
				for _, c := range id {
					if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
						t.Errorf("ID contains non-hex character: %c", c)
						break
					}
				}
			})
		}
	}
}

// TestFormatWithSalt tests that all formats work correctly with salt.
func TestFormatWithSalt(t *testing.T) {
	salt := "test-salt-12345"

	formats := []struct {
		name   string
		format machineid.FormatMode
		length int
	}{
		{"Format32", machineid.Format32, 32},
		{"Format64", machineid.Format64, 64},
		{"Format128", machineid.Format128, 128},
		{"Format256", machineid.Format256, 256},
	}

	for _, tc := range formats {
		t.Run(tc.name, func(t *testing.T) {
			// With salt
			g1 := machineid.New().
				WithCPU().
				WithSystemUUID().
				WithSalt(salt).
				WithFormat(tc.format)

			id1, err := g1.ID(context.Background())
			if err != nil {
				t.Fatalf("%s with salt ID() error = %v", tc.name, err)
			}

			// Without salt
			g2 := machineid.New().
				WithCPU().
				WithSystemUUID().
				WithFormat(tc.format)

			id2, err := g2.ID(context.Background())
			if err != nil {
				t.Fatalf("%s without salt ID() error = %v", tc.name, err)
			}

			// IDs should be different
			if id1 == id2 {
				t.Errorf("%s: IDs with and without salt should differ", tc.name)
			}

			// Both should have correct length
			if len(id1) != tc.length {
				t.Errorf("%s with salt: expected length %d, got %d", tc.name, tc.length, len(id1))
			}
			if len(id2) != tc.length {
				t.Errorf("%s without salt: expected length %d, got %d", tc.name, tc.length, len(id2))
			}
		})
	}
}

// TestFormatValidation tests validation with different formats.
func TestFormatValidation(t *testing.T) {
	formats := []struct {
		name   string
		format machineid.FormatMode
	}{
		{"Format32", machineid.Format32},
		{"Format64", machineid.Format64},
		{"Format128", machineid.Format128},
		{"Format256", machineid.Format256},
	}

	for _, tc := range formats {
		t.Run(tc.name, func(t *testing.T) {
			g := machineid.New().
				WithCPU().
				WithSystemUUID().
				WithFormat(tc.format)

			id, err := g.ID(context.Background())
			if err != nil {
				t.Fatalf("%s ID() error = %v", tc.name, err)
			}

			// Validation should succeed with same format
			valid, err := g.Validate(context.Background(), id)
			if err != nil {
				t.Fatalf("%s Validate() error = %v", tc.name, err)
			}
			if !valid {
				t.Errorf("%s Validate() should return true for matching ID", tc.name)
			}

			// Validation should fail with different format
			var differentFormat machineid.FormatMode
			if tc.format == machineid.Format32 {
				differentFormat = machineid.Format64
			} else {
				differentFormat = machineid.Format32
			}

			g2 := machineid.New().
				WithCPU().
				WithSystemUUID().
				WithFormat(differentFormat)

			valid2, err := g2.Validate(context.Background(), id)
			if err != nil {
				t.Fatalf("%s Validate() with different format error = %v", tc.name, err)
			}
			if valid2 {
				t.Errorf("%s Validate() should return false for different format", tc.name)
			}
		})
	}
}

// TestWithMACBackwardCompatibility tests that WithMAC() with no args works as before.
func TestWithMACBackwardCompatibility(t *testing.T) {
	// WithMAC() with no args should work (physical filter, same as current behavior)
	g := machineid.New().WithCPU().WithSystemUUID().WithMAC()

	id, err := g.ID(context.Background())
	if err != nil {
		t.Fatalf("WithMAC() error = %v", err)
	}

	if len(id) != 64 {
		t.Errorf("WithMAC() ID length = %d, want 64", len(id))
	}
}

// TestWithMACFilter tests WithMAC with explicit filter modes.
func TestWithMACFilter(t *testing.T) {
	filters := []struct {
		name   string
		filter machineid.MACFilter
	}{
		{"physical", machineid.MACFilterPhysical},
		{"all", machineid.MACFilterAll},
		{"virtual", machineid.MACFilterVirtual},
	}

	for _, tt := range filters {
		t.Run(tt.name, func(t *testing.T) {
			g := machineid.New().WithCPU().WithSystemUUID().WithMAC(tt.filter)

			id, err := g.ID(context.Background())
			if err != nil {
				t.Fatalf("WithMAC(%s) error = %v", tt.name, err)
			}

			if len(id) != 64 {
				t.Errorf("WithMAC(%s) ID length = %d, want 64", tt.name, len(id))
			}
		})
	}
}

// TestWithMACFilterAffectsID tests that different MAC filters can produce different IDs.
func TestWithMACFilterAffectsID(t *testing.T) {
	physical := machineid.New().WithMAC(machineid.MACFilterPhysical).WithCPU()
	all := machineid.New().WithMAC(machineid.MACFilterAll).WithCPU()

	idPhysical, err := physical.ID(context.Background())
	if err != nil {
		t.Fatalf("physical ID error: %v", err)
	}

	idAll, err := all.ID(context.Background())
	if err != nil {
		t.Fatalf("all ID error: %v", err)
	}

	// They may or may not differ depending on the system, just verify both are valid
	if len(idPhysical) != 64 || len(idAll) != 64 {
		t.Errorf("Expected 64-char IDs, got physical=%d, all=%d", len(idPhysical), len(idAll))
	}
}

// TestFormatPowerOfTwo verifies that all format lengths are powers of 2.
func TestFormatPowerOfTwo(t *testing.T) {
	formats := []struct {
		name   string
		format machineid.FormatMode
		length int
		power  int
	}{
		{"Format32", machineid.Format32, 32, 5},
		{"Format64", machineid.Format64, 64, 6},
		{"Format128", machineid.Format128, 128, 7},
		{"Format256", machineid.Format256, 256, 8},
	}

	for _, tc := range formats {
		t.Run(tc.name, func(t *testing.T) {
			g := machineid.New().
				WithCPU().
				WithSystemUUID().
				WithFormat(tc.format)

			id, err := g.ID(context.Background())
			if err != nil {
				t.Fatalf("%s ID() error = %v", tc.name, err)
			}

			// Verify length
			if len(id) != tc.length {
				t.Errorf("%s: expected length %d, got %d", tc.name, tc.length, len(id))
			}

			// Verify it's a power of 2
			power := 1
			for i := 0; i < tc.power; i++ {
				power *= 2
			}
			if len(id) != power {
				t.Errorf("%s: length should be 2^%d = %d, got %d", tc.name, tc.power, power, len(id))
			}
		})
	}
}
