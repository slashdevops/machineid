package machineid

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"runtime"
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

// defaultTimeout is the default timeout for system command execution.
const defaultTimeout = 5 * time.Second

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
	logger             *slog.Logger
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
			Timeout: defaultTimeout,
		},
		formatMode: Format64,
	}
}

// WithSalt sets a custom salt for additional entropy.
func (p *Provider) WithSalt(salt string) *Provider {
	p.salt = salt

	return p
}

// WithFormat sets the output format and length.
// Use Format64 (default), Format32, Format128, or Format256.
func (p *Provider) WithFormat(mode FormatMode) *Provider {
	p.formatMode = mode

	return p
}

// WithCPU includes the CPU identifier in the generation.
func (p *Provider) WithCPU() *Provider {
	p.includeCPU = true

	return p
}

// WithMotherboard includes the motherboard serial number in the generation.
func (p *Provider) WithMotherboard() *Provider {
	p.includeMotherboard = true

	return p
}

// WithSystemUUID includes the system UUID in the generation.
func (p *Provider) WithSystemUUID() *Provider {
	p.includeSystemUUID = true

	return p
}

// WithMAC includes network interface MAC addresses in the generation.
func (p *Provider) WithMAC() *Provider {
	p.includeMAC = true

	return p
}

// WithDisk includes disk serial numbers in the generation.
func (p *Provider) WithDisk() *Provider {
	p.includeDisk = true

	return p
}

// WithExecutor sets a custom [CommandExecutor], enabling deterministic testing
// without real system commands.
func (p *Provider) WithExecutor(executor CommandExecutor) *Provider {
	p.commandExecutor = executor

	return p
}

// WithLogger sets an optional [*slog.Logger] for observability.
// When set, the provider logs component collection, fallback paths, command
// execution timing, and errors. A nil logger (the default) disables all logging
// with zero overhead.
//
// Compatible with any [*slog.Logger], including [slog.Default] which bridges
// to the standard [log] package.
func (p *Provider) WithLogger(logger *slog.Logger) *Provider {
	p.logger = logger

	return p
}

// VMFriendly configures the provider for virtual machines (CPU + UUID only).
func (p *Provider) VMFriendly() *Provider {
	p.includeCPU = true
	p.includeSystemUUID = true
	p.includeMotherboard = false
	p.includeMAC = false
	p.includeDisk = false

	return p
}

// ID generates the machine ID based on the configured options.
// It caches the result, so subsequent calls return the same ID.
// The configuration is frozen after the first successful call to ID().
// The provided context controls the timeout and cancellation of any
// system commands executed during hardware identifier collection.
// This method is safe for concurrent use.
func (p *Provider) ID(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cachedID != "" {
		p.logDebug("returning cached machine ID")

		return p.cachedID, nil
	}

	p.logInfo("generating machine ID",
		"platform", runtime.GOOS,
		"format", p.formatMode,
		"components", p.enabledComponents(),
	)

	diag := &DiagnosticInfo{
		Errors: make(map[string]error),
	}

	identifiers, err := collectIdentifiers(ctx, p, diag)
	if err != nil {
		return "", err
	}

	if len(identifiers) == 0 {
		p.diagnostics = diag
		p.logWarn("no hardware identifiers collected", "errors", diag.Errors)

		return "", ErrNoIdentifiers
	}

	p.logDebug("collected identifiers", "count", len(identifiers), "identifiers", identifiers)

	p.diagnostics = diag
	p.cachedID = hashIdentifiers(identifiers, p.salt, p.formatMode)

	p.logInfo("machine ID generated",
		"collected", diag.Collected,
		"errors_count", len(diag.Errors),
	)

	return p.cachedID, nil
}

// Diagnostics returns information about which hardware components were
// successfully collected and which ones failed during the last call to [ID].
// Returns nil if [ID] has not been called yet.
func (p *Provider) Diagnostics() *DiagnosticInfo {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.diagnostics
}

// Validate checks if the provided ID matches the current machine ID.
// The provided context is forwarded to [ID] if it needs to generate the ID.
func (p *Provider) Validate(ctx context.Context, id string) (bool, error) {
	currentID, err := p.ID(ctx)
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
// All formats produce power-of-2 lengths without dashes.
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

// logDebug logs at debug level if a logger is configured.
func (p *Provider) logDebug(msg string, args ...any) {
	if p.logger != nil {
		p.logger.Debug(msg, args...)
	}
}

// logInfo logs at info level if a logger is configured.
func (p *Provider) logInfo(msg string, args ...any) {
	if p.logger != nil {
		p.logger.Info(msg, args...)
	}
}

// logWarn logs at warn level if a logger is configured.
func (p *Provider) logWarn(msg string, args ...any) {
	if p.logger != nil {
		p.logger.Warn(msg, args...)
	}
}

// enabledComponents returns the names of the hardware components that are enabled.
func (p *Provider) enabledComponents() []string {
	var components []string
	if p.includeCPU {
		components = append(components, ComponentCPU)
	}
	if p.includeMotherboard {
		components = append(components, ComponentMotherboard)
	}
	if p.includeSystemUUID {
		components = append(components, ComponentSystemUUID)
	}
	if p.includeMAC {
		components = append(components, ComponentMAC)
	}
	if p.includeDisk {
		components = append(components, ComponentDisk)
	}

	return components
}

// appendIdentifierIfValid adds the result of getValue to identifiers with the given prefix if valid.
// It records the result in diag under the given component name.
func appendIdentifierIfValid(identifiers []string, getValue func() (string, error), prefix string, diag *DiagnosticInfo, component string, logger *slog.Logger) []string {
	value, err := getValue()
	if err != nil {
		compErr := &ComponentError{Component: component, Err: err}
		if diag != nil {
			diag.Errors[component] = compErr
		}
		if logger != nil {
			logger.Warn("component failed", "component", component, "error", err)
		}

		return identifiers
	}

	if value == "" {
		compErr := &ComponentError{Component: component, Err: ErrEmptyValue}
		if diag != nil {
			diag.Errors[component] = compErr
		}
		if logger != nil {
			logger.Warn("component returned empty value", "component", component)
		}

		return identifiers
	}

	if diag != nil {
		diag.Collected = append(diag.Collected, component)
	}
	if logger != nil {
		logger.Info("component collected", "component", component)
		logger.Debug("component value", "component", component, "value", value)
	}

	return append(identifiers, prefix+value)
}

// appendIdentifiersIfValid adds the results of getValues to identifiers with the given prefix if valid.
// It records the result in diag under the given component name.
func appendIdentifiersIfValid(identifiers []string, getValues func() ([]string, error), prefix string, diag *DiagnosticInfo, component string, logger *slog.Logger) []string {
	values, err := getValues()
	if err != nil {
		compErr := &ComponentError{Component: component, Err: err}
		if diag != nil {
			diag.Errors[component] = compErr
		}
		if logger != nil {
			logger.Warn("component failed", "component", component, "error", err)
		}

		return identifiers
	}

	if len(values) == 0 {
		compErr := &ComponentError{Component: component, Err: ErrNoValues}
		if diag != nil {
			diag.Errors[component] = compErr
		}
		if logger != nil {
			logger.Warn("component returned no values", "component", component)
		}

		return identifiers
	}

	if diag != nil {
		diag.Collected = append(diag.Collected, component)
	}
	if logger != nil {
		logger.Info("component collected", "component", component, "count", len(values))
		logger.Debug("component values", "component", component, "values", values)
	}

	for _, value := range values {
		identifiers = append(identifiers, prefix+value)
	}

	return identifiers
}
