package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"

	"github.com/slashdevops/machineid"
	"github.com/slashdevops/machineid/internal/version"
)

const applicationName = "machineid"

func main() {
	// Hardware component flags
	cpu := flag.Bool("cpu", false, "Include CPU identifier")
	motherboard := flag.Bool("motherboard", false, "Include motherboard serial number")
	uuid := flag.Bool("uuid", false, "Include system UUID")
	mac := flag.Bool("mac", false, "Include network MAC addresses")
	disk := flag.Bool("disk", false, "Include disk serial numbers")
	all := flag.Bool("all", false, "Include all hardware identifiers")
	vm := flag.Bool("vm", false, "Use VM-friendly mode (CPU + UUID only)")

	// Output options
	format := flag.Int("format", 64, "Output format length: 32, 64, 128, or 256 characters")
	salt := flag.String("salt", "", "Custom salt for application-specific IDs")

	// Actions
	validate := flag.String("validate", "", "Validate a machine ID against the current machine")
	diagnostics := flag.Bool("diagnostics", false, "Show diagnostic information about collected components")
	jsonOutput := flag.Bool("json", false, "Output result as JSON")

	// Info flags
	versionFlag := flag.Bool("version", false, "Show version information")
	versionLongFlag := flag.Bool("version.long", false, "Show detailed version information")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "machineid - Generate unique machine identifiers based on hardware characteristics\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n  machineid [flags]\n\nFlags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  machineid -cpu -uuid                          Generate ID from CPU + UUID\n")
		fmt.Fprintf(os.Stderr, "  machineid -all -format 32                     All hardware, compact format\n")
		fmt.Fprintf(os.Stderr, "  machineid -vm -salt \"my-app\"                   VM-friendly with salt\n")
		fmt.Fprintf(os.Stderr, "  machineid -cpu -uuid -diagnostics             Show collected components\n")
		fmt.Fprintf(os.Stderr, "  machineid -cpu -uuid -validate <id>           Validate an existing ID\n")
		fmt.Fprintf(os.Stderr, "  machineid -cpu -uuid -json                    Output as JSON\n")
		fmt.Fprintf(os.Stderr, "  machineid -version                            Show version\n")
		fmt.Fprintf(os.Stderr, "  machineid -version.long                       Show detailed version\n")
	}

	flag.Parse()

	// Handle version flag
	if *versionFlag {
		if version.Version == "0.0.0" {
			if info, ok := debug.ReadBuildInfo(); ok {
				fmt.Printf("%s version: %s\n", applicationName, info.Main.Version)
			} else {
				fmt.Printf("%s version: %s\n", applicationName, version.Version)
			}
		} else {
			fmt.Printf("%s version: %s\n", applicationName, version.Version)
		}

		os.Exit(0)
	}

	// Handle detailed version flag
	if *versionLongFlag {
		var sb strings.Builder

		if version.Version == "0.0.0" {
			if info, ok := debug.ReadBuildInfo(); ok {
				fmt.Fprintf(&sb, "%s version: %s, ", applicationName, info.Main.Version)
				fmt.Fprintf(&sb, "Git commit: %s, ", info.Main.Sum)
				fmt.Fprintf(&sb, "Go version: %s\n", info.GoVersion)
			} else {
				fmt.Fprintf(&sb, "%s version: %s\n", applicationName, version.Version)
				fmt.Fprintf(&sb, "Build date: %s, ", version.BuildDate)
				fmt.Fprintf(&sb, "Build user: %s, ", version.BuildUser)
				fmt.Fprintf(&sb, "Git commit: %s, ", version.GitCommit)
				fmt.Fprintf(&sb, "Git branch: %s, ", version.GitBranch)
				fmt.Fprintf(&sb, "Go version: %s\n", version.GoVersion)
			}
		} else {
			fmt.Fprintf(&sb, "%s version: %s, ", applicationName, version.Version)
			fmt.Fprintf(&sb, "Build date: %s, ", version.BuildDate)
			fmt.Fprintf(&sb, "Build user: %s, ", version.BuildUser)
			fmt.Fprintf(&sb, "Git commit: %s, ", version.GitCommit)
			fmt.Fprintf(&sb, "Git branch: %s, ", version.GitBranch)
			fmt.Fprintf(&sb, "Go version: %s\n", version.GoVersion)
		}

		fmt.Print(sb.String())
		os.Exit(0)
	}

	formatMode, err := parseFormatMode(*format)
	if err != nil {
		slog.Error("invalid format", "error", err)
		flag.Usage()
		os.Exit(1)
	}

	// Build provider
	provider := machineid.New().WithFormat(formatMode)

	if *salt != "" {
		provider.WithSalt(*salt)
	}

	switch {
	case *vm:
		provider.VMFriendly()
	case *all:
		provider.WithCPU().WithMotherboard().WithSystemUUID().WithMAC().WithDisk()
	default:
		if !*cpu && !*motherboard && !*uuid && !*mac && !*disk {
			slog.Error("no hardware identifiers selected; use -cpu, -uuid, -all, -vm, etc.")
			flag.Usage()
			os.Exit(1)
		}
		if *cpu {
			provider.WithCPU()
		}
		if *motherboard {
			provider.WithMotherboard()
		}
		if *uuid {
			provider.WithSystemUUID()
		}
		if *mac {
			provider.WithMAC()
		}
		if *disk {
			provider.WithDisk()
		}
	}

	// Generate machine ID
	id, err := provider.ID()
	if err != nil {
		slog.Error("failed to generate machine ID", "error", err)
		os.Exit(1)
	}

	// Validate mode
	if *validate != "" {
		handleValidate(provider, *validate, *jsonOutput)
		return
	}

	// Output
	if *jsonOutput {
		output := map[string]any{
			"id":     id,
			"format": *format,
			"length": len(id),
		}
		if *diagnostics {
			output["diagnostics"] = formatDiagnostics(provider)
		}
		printJSON(output)
		return
	}

	fmt.Println(id)

	if *diagnostics {
		printDiagnostics(provider)
	}
}

func parseFormatMode(format int) (machineid.FormatMode, error) {
	switch format {
	case 32:
		return machineid.Format32, nil
	case 64:
		return machineid.Format64, nil
	case 128:
		return machineid.Format128, nil
	case 256:
		return machineid.Format256, nil
	default:
		return 0, fmt.Errorf("unsupported format %d; valid values are 32, 64, 128, 256", format)
	}
}

func handleValidate(provider *machineid.Provider, expectedID string, jsonOut bool) {
	valid, err := provider.Validate(expectedID)
	if err != nil {
		slog.Error("validation failed", "error", err)
		os.Exit(1)
	}

	if jsonOut {
		printJSON(map[string]any{
			"valid":      valid,
			"expectedID": expectedID,
		})
		if !valid {
			os.Exit(1)
		}
		return
	}

	if valid {
		fmt.Println("valid: machine ID matches")
	} else {
		fmt.Println("invalid: machine ID does not match")
		os.Exit(1)
	}
}

func printDiagnostics(provider *machineid.Provider) {
	diag := provider.Diagnostics()
	if diag == nil {
		fmt.Fprintln(os.Stderr, "no diagnostic information available")
		return
	}

	fmt.Fprintln(os.Stderr, "\nDiagnostics:")
	if len(diag.Collected) > 0 {
		fmt.Fprintf(os.Stderr, "  Collected: %s\n", strings.Join(diag.Collected, ", "))
	}
	if len(diag.Errors) > 0 {
		fmt.Fprintln(os.Stderr, "  Errors:")
		for component, err := range diag.Errors {
			fmt.Fprintf(os.Stderr, "    %s: %v\n", component, err)
		}
	}
}

func formatDiagnostics(provider *machineid.Provider) map[string]any {
	diag := provider.Diagnostics()
	if diag == nil {
		return nil
	}

	result := map[string]any{
		"collected": diag.Collected,
	}

	if len(diag.Errors) > 0 {
		errors := make(map[string]string, len(diag.Errors))
		for component, err := range diag.Errors {
			errors[component] = err.Error()
		}
		result["errors"] = errors
	}

	return result
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		slog.Error("failed to encode JSON", "error", err)
		os.Exit(1)
	}
}
