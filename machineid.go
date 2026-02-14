package machineid

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// FormatMode defines the output format and length of the machine ID.
type FormatMode int

const (
	// Format64 outputs 64 hex characters (2^6), default SHA-256 output without dashes
	Format64 FormatMode = iota
	// Format32 outputs 32 hex characters (2^5), truncated SHA-256
	Format32
	// Format128 outputs 128 hex characters (2^7), double SHA-256
	Format128
	// Format256 outputs 256 hex characters (2^8), quadruple SHA-256
	Format256
)

// Component names used as keys in DiagnosticInfo.
const (
	ComponentCPU         = "cpu"
	ComponentMotherboard = "motherboard"
	ComponentSystemUUID  = "uuid"
	ComponentMAC         = "mac"
	ComponentDisk        = "disk"
	ComponentMachineID   = "machine-id" // Linux systemd machine-id
)

// DiagnosticInfo contains information about what was collected during ID generation.
// Use [Provider.Diagnostics] to retrieve this information after calling [Provider.ID].
type DiagnosticInfo struct {
	Errors    map[string]error // Component names that failed with their errors
	Collected []string         // Component names that were successfully collected
}

// CommandExecutor is an interface for executing system commands, allowing for dependency injection and testing.
type CommandExecutor interface {
	Execute(ctx context.Context, name string, args ...string) (string, error)
}

// Provider configures and generates unique machine IDs.
// After the first call to ID(), the configuration is frozen and the result is cached.
// Provider methods are safe for concurrent use after configuration is complete.
type Provider struct {
	commandExecutor    CommandExecutor
	diagnostics        *DiagnosticInfo
	salt               string
	cachedID           string
	formatMode         FormatMode
	mu                 sync.Mutex
	includeCPU         bool
	includeMotherboard bool
	includeSystemUUID  bool
	includeMAC         bool
	includeDisk        bool
}

// New creates a new Provider with default settings.
// The provider uses real system commands by default.
// Default format is Format64 (64 hex characters, 2^6).
func New() *Provider {
	return &Provider{
		commandExecutor: &defaultCommandExecutor{
			TimeOut: 5 * time.Second,
		},
		formatMode: Format64,
	}
}

// WithSalt sets a custom salt for additional entropy.
func (g *Provider) WithSalt(salt string) *Provider {
	g.salt = salt

	return g
}

// WithFormat sets the output format and length.
// Use Format64 (default), Format32, Format128, or Format256.
func (g *Provider) WithFormat(mode FormatMode) *Provider {
	g.formatMode = mode

	return g
}

// WithCPU includes the CPU identifier in the generation.
func (g *Provider) WithCPU() *Provider {
	g.includeCPU = true

	return g
}

// WithMotherboard includes the motherboard serial number in the generation.
func (g *Provider) WithMotherboard() *Provider {
	g.includeMotherboard = true

	return g
}

// WithSystemUUID includes the system UUID in the generation.
func (g *Provider) WithSystemUUID() *Provider {
	g.includeSystemUUID = true

	return g
}

// WithMAC includes network interface MAC addresses in the generation.
func (g *Provider) WithMAC() *Provider {
	g.includeMAC = true

	return g
}

// WithDisk includes disk serial numbers in the generation.
func (g *Provider) WithDisk() *Provider {
	g.includeDisk = true

	return g
}

// WithExecutor sets a custom command executor for testing purposes.
// This method is primarily intended for testing and should not be used in production.
func (g *Provider) WithExecutor(executor CommandExecutor) *Provider {
	g.commandExecutor = executor

	return g
}

// VMFriendly configures the provider for virtual machines (CPU + UUID only).
func (g *Provider) VMFriendly() *Provider {
	g.includeCPU = true
	g.includeSystemUUID = true
	g.includeMotherboard = false
	g.includeMAC = false
	g.includeDisk = false

	return g
}

// ID generates the machine ID based on the configured options.
// It caches the result, so subsequent calls return the same ID.
// The configuration is frozen after the first successful call to ID().
// This method is safe for concurrent use.
func (g *Provider) ID() (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.cachedID != "" {
		return g.cachedID, nil
	}

	diag := &DiagnosticInfo{
		Errors: make(map[string]error),
	}

	identifiers, err := collectIdentifiers(g, diag)
	if err != nil {
		return "", fmt.Errorf("failed to collect hardware identifiers: %w", err)
	}

	if len(identifiers) == 0 {
		g.diagnostics = diag

		return "", fmt.Errorf("no hardware identifiers found with current configuration")
	}

	g.diagnostics = diag
	g.cachedID = hashIdentifiers(identifiers, g.salt, g.formatMode)

	return g.cachedID, nil
}

// Diagnostics returns information about which hardware components were
// successfully collected and which ones failed during the last call to [ID].
// Returns nil if [ID] has not been called yet.
func (g *Provider) Diagnostics() *DiagnosticInfo {
	g.mu.Lock()
	defer g.mu.Unlock()

	return g.diagnostics
}

// Validate checks if the provided ID matches the current machine ID.
func (g *Provider) Validate(id string) (bool, error) {
	currentID, err := g.ID()
	if err != nil {
		return false, err
	}

	return currentID == id, nil
}

// hashIdentifiers processes and hashes the hardware identifiers with optional salt.
// Returns a hash formatted according to the specified FormatMode.
func hashIdentifiers(identifiers []string, salt string, mode FormatMode) string {
	sort.Strings(identifiers)
	combined := strings.Join(identifiers, "|")
	if salt != "" {
		combined = salt + "|" + combined
	}

	// Generate SHA256 hash
	hash := sha256.Sum256([]byte(combined))
	rawHash := hex.EncodeToString(hash[:])

	return formatHash(rawHash, mode)
}

// formatHash formats a 64-character SHA-256 hash according to the specified mode.
// All formats (except legacy) produce power-of-2 lengths without dashes.
func formatHash(hash string, mode FormatMode) string {
	if len(hash) != 64 {
		return hash
	}

	switch mode {
	case Format32:
		// 32 hex characters (2^5 = 32)
		return hash[:32]

	case Format64:
		// 64 hex characters (2^6 = 64), no dashes - default
		return hash

	case Format128:
		// 128 hex characters (2^7 = 128)
		// Generate second hash by rehashing the first
		hash2 := sha256.Sum256([]byte(hash))

		return hash + hex.EncodeToString(hash2[:])

	case Format256:
		// 256 hex characters (2^8 = 256)
		// Generate additional hashes for extended length
		hash2 := sha256.Sum256([]byte(hash))
		hash3 := sha256.Sum256([]byte(hex.EncodeToString(hash2[:])))
		hash4 := sha256.Sum256([]byte(hex.EncodeToString(hash3[:])))
		return hash + hex.EncodeToString(hash2[:]) +
			hex.EncodeToString(hash3[:]) + hex.EncodeToString(hash4[:])

	default:
		return hash
	}
}

// appendIdentifierIfValid adds the result of getValue to identifiers with the given prefix if valid.
// It records the result in diag under the given component name.
func appendIdentifierIfValid(identifiers []string, getValue func() (string, error), prefix string, diag *DiagnosticInfo, component string) []string {
	value, err := getValue()
	if err != nil {
		if diag != nil {
			diag.Errors[component] = err
		}

		return identifiers
	}

	if value == "" {
		if diag != nil {
			diag.Errors[component] = fmt.Errorf("empty value returned")
		}

		return identifiers
	}

	if diag != nil {
		diag.Collected = append(diag.Collected, component)
	}

	return append(identifiers, prefix+value)
}

// appendIdentifiersIfValid adds the results of getValues to identifiers with the given prefix if valid.
// It records the result in diag under the given component name.
func appendIdentifiersIfValid(identifiers []string, getValues func() ([]string, error), prefix string, diag *DiagnosticInfo, component string) []string {
	values, err := getValues()
	if err != nil {
		if diag != nil {
			diag.Errors[component] = err
		}

		return identifiers
	}

	if len(values) == 0 {
		if diag != nil {
			diag.Errors[component] = fmt.Errorf("no values found")
		}

		return identifiers
	}

	if diag != nil {
		diag.Collected = append(diag.Collected, component)
	}

	for _, value := range values {
		identifiers = append(identifiers, prefix+value)
	}

	return identifiers
}
