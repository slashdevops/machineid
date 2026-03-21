package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"runtime"
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
	uuid := flag.Bool("uuid", false, "Include system UUID (BIOS/UEFI)")
	mac := flag.Bool("mac", false, "Include network interface MAC addresses")
	macFilterFlag := flag.String("mac-filter", "physical", "MAC address filter: physical, all, or virtual (requires -mac or -all)")
	disk := flag.Bool("disk", false, "Include disk serial numbers")
	all := flag.Bool("all", false, "Include all hardware identifiers (CPU, motherboard, UUID, MAC, disk)")
	vm := flag.Bool("vm", false, "VM-friendly mode: use only CPU + UUID (ignores other component flags)")

	// Output options
	format := flag.Int("format", 64, "Output length in hex characters: 32, 64, 128, or 256")
	salt := flag.String("salt", "", "Application-specific salt to produce unique IDs per app")

	// Actions
	validate := flag.String("validate", "", "Validate a previously stored ID against this machine")
	diagnostics := flag.Bool("diagnostics", false, "Show which hardware components were collected or failed")
	jsonOutput := flag.Bool("json", false, "Format output as JSON")

	// Logging flags
	verbose := flag.Bool("verbose", false, "Log info-level messages to stderr (fallbacks, lifecycle events)")
	debugFlag := flag.Bool("debug", false, "Log debug-level messages to stderr (commands, raw values, timing)")

	// Info flags
	versionFlag := flag.Bool("version", false, "Print version and exit")
	versionLongFlag := flag.Bool("version-long", false, "Print detailed build information and exit")

	flag.Usage = printUsage

	flag.Parse()

	if *versionFlag {
		printVersion()
		os.Exit(0)
	}

	if *versionLongFlag {
		printVersionLong()
		os.Exit(0)
	}

	formatMode, err := parseFormatMode(*format)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
		printUsage()
		os.Exit(2)
	}

	// Configure logger
	if *debugFlag {
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
		slog.SetDefault(logger)
	} else if *verbose {
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
		slog.SetDefault(logger)
	}

	// Build provider
	provider := machineid.New().WithFormat(formatMode)

	if *verbose || *debugFlag {
		provider.WithLogger(slog.Default())
	}

	if *salt != "" {
		provider.WithSalt(*salt)
	}

	mFilter, err := parseMACFilter(*macFilterFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
		printUsage()
		os.Exit(2)
	}

	switch {
	case *vm:
		provider.VMFriendly()
	case *all:
		provider.WithCPU().WithMotherboard().WithSystemUUID().WithMAC(mFilter).WithDisk()
	default:
		if !*cpu && !*motherboard && !*uuid && !*mac && !*disk {
			// Default: CPU + Motherboard + System UUID
			provider.WithCPU().WithMotherboard().WithSystemUUID()
		} else {
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
				provider.WithMAC(mFilter)
			}
			if *disk {
				provider.WithDisk()
			}
		}
	}

	// Generate machine ID
	ctx := context.Background()

	id, err := provider.ID(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Validate mode
	if *validate != "" {
		handleValidate(ctx, provider, *validate, *jsonOutput)
		return
	}

	// Output
	if *jsonOutput {
		output := map[string]any{
			"id":     id,
			"format": *format,
			"length": len(id),
		}
		if *salt != "" {
			output["salt"] = *salt
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

func printUsage() {
	w := os.Stderr
	fmt.Fprintf(w, "Generate unique, deterministic machine identifiers from hardware characteristics.\n\n")
	fmt.Fprintf(w, "Usage:\n")
	fmt.Fprintf(w, "  %s [-cpu] [-uuid] [-motherboard] [-mac] [-disk] [options]\n", applicationName)
	fmt.Fprintf(w, "  %s -all [options]\n", applicationName)
	fmt.Fprintf(w, "  %s -vm [options]\n\n", applicationName)

	fmt.Fprintf(w, "When no component flags are specified, the default is -cpu -motherboard -uuid.\n\n")

	fmt.Fprintf(w, "Hardware Components:\n")
	printFlag(w, "-cpu", "Include CPU identifier")
	printFlag(w, "-motherboard", "Include motherboard serial number")
	printFlag(w, "-uuid", "Include system UUID (BIOS/UEFI)")
	printFlag(w, "-mac", "Include network interface MAC addresses")
	printFlag(w, "-disk", "Include disk serial numbers")
	printFlag(w, "-all", "Include all hardware identifiers")
	printFlag(w, "-vm", "VM-friendly mode: CPU + UUID only")
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Output Options:\n")
	printFlag(w, "-format N", "Output length: 32, 64 (default), 128, or 256 hex chars")
	printFlag(w, "-salt STRING", "Application-specific salt for unique IDs per app")
	printFlag(w, "-mac-filter F", "MAC filter: physical (default), all, or virtual")
	printFlag(w, "-json", "Format output as JSON")
	printFlag(w, "-diagnostics", "Show collected/failed hardware components")
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Validation:\n")
	printFlag(w, "-validate ID", "Check a stored ID against the current machine")
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Logging:\n")
	printFlag(w, "-verbose", "Info-level logs to stderr (fallbacks, lifecycle)")
	printFlag(w, "-debug", "Debug-level logs to stderr (commands, values, timing)")
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Info:\n")
	printFlag(w, "-version", "Print version and exit")
	printFlag(w, "-version-long", "Print detailed build information and exit")
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Examples:\n")
	fmt.Fprintf(w, "  %s                                    Default: CPU + motherboard + UUID\n", applicationName)
	fmt.Fprintf(w, "  %s -cpu -uuid                         Specific components\n", applicationName)
	fmt.Fprintf(w, "  %s -all -format 32                    All hardware, compact output\n", applicationName)
	fmt.Fprintf(w, "  %s -vm -salt \"my-app\"                  VM-friendly with salt\n", applicationName)
	fmt.Fprintf(w, "  %s -all -json -diagnostics            JSON with diagnostics\n", applicationName)
	fmt.Fprintf(w, "  %s -cpu -uuid -validate <id>          Validate a stored ID\n", applicationName)
	fmt.Fprintf(w, "  %s -mac -mac-filter all               Include all MAC addresses\n", applicationName)
	fmt.Fprintf(w, "  %s -all -verbose                      Show info-level logs\n", applicationName)
	fmt.Fprintf(w, "  %s -all -debug                        Show debug-level logs\n", applicationName)
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Exit Codes:\n")
	fmt.Fprintf(w, "  0  Success\n")
	fmt.Fprintf(w, "  1  ID generation or validation failed\n")
	fmt.Fprintf(w, "  2  Invalid arguments\n")
}

func printFlag(w *os.File, name, desc string) {
	fmt.Fprintf(w, "  %-20s %s\n", name, desc)
}

func printVersion() {
	v := resolveVersion()
	fmt.Printf("%s %s\n", applicationName, v)
}

func printVersionLong() {
	v := resolveVersion()
	fmt.Printf("%s %s\n", applicationName, v)

	if version.Version != "0.0.0" {
		printField("Build date", version.BuildDate)
		printField("Git commit", version.GitCommit)
		printField("Git branch", version.GitBranch)
		printField("Build user", version.BuildUser)
		printField("Go version", version.GoVersion)
		printField("Platform", fmt.Sprintf("%s/%s", version.GoVersionOS, version.GoVersionArch))
	} else if info, ok := debug.ReadBuildInfo(); ok {
		printField("Module", info.Main.Path)
		printField("Go version", info.GoVersion)
		printField("Platform", fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH))

		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				printField("Git commit", setting.Value)
			case "vcs.time":
				printField("Commit date", setting.Value)
			case "vcs.modified":
				if setting.Value == "true" {
					printField("Modified", "yes (uncommitted changes)")
				}
			}
		}
	}
}

func resolveVersion() string {
	if version.Version != "0.0.0" {
		return version.Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "devel"
}

func printField(label, value string) {
	if value == "" {
		return
	}
	fmt.Printf("  %-14s %s\n", label+":", value)
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

func parseMACFilter(value string) (machineid.MACFilter, error) {
	switch strings.ToLower(value) {
	case "physical":
		return machineid.MACFilterPhysical, nil
	case "all":
		return machineid.MACFilterAll, nil
	case "virtual":
		return machineid.MACFilterVirtual, nil
	default:
		return 0, fmt.Errorf("unsupported mac-filter %q; valid values are physical, all, virtual", value)
	}
}

func handleValidate(ctx context.Context, provider *machineid.Provider, expectedID string, jsonOut bool) {
	valid, err := provider.Validate(ctx, expectedID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: validation failed: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "Error: failed to encode JSON: %v\n", err)
		os.Exit(1)
	}
}
