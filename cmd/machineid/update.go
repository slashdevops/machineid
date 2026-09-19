package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/slashdevops/machineid/internal/selfupdate"
)

// Exit codes of the update verb. 2 stays "invalid arguments" as in the rest
// of the CLI; the ds-utils-style split between "nothing attempted" and
// "attempted and failed" uses 1 and 3.
const (
	exitUpdateOK           = 0
	exitUpdatePrerequisite = 1
	exitUpdateUsage        = 2
	exitUpdateFailed       = 3
)

// updateVerb is the positional command that triggers a self-update.
const updateVerb = "update"

// isUpdateVerb reports whether args (without the program name) start the update verb.
func isUpdateVerb(args []string) bool {
	return len(args) > 0 && args[0] == updateVerb
}

// runUpdate executes `machineid update` and returns the process exit code.
func runUpdate(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(applicationName+" "+updateVerb, flag.ContinueOnError)
	fs.SetOutput(stderr)

	method := fs.String("method", string(selfupdate.MethodAuto), "How to update: auto, release (signed GitHub asset) or go (go install)")
	version := fs.String("version", "", "Install a specific release tag, e.g. v0.3.0 (also how to go back a version)")
	check := fs.Bool("check", false, "Report what would happen and exit without changing anything")
	refresh := fs.Bool("refresh", false, "Bypass the one-hour cache and look up the latest release now (counts against the hourly budget)")
	force := fs.Bool("force", false, "Install to the method's location even if this binary lives elsewhere, and reinstall an equal version")
	yes := fs.Bool("yes", false, "Do not ask for confirmation")
	requireSig := fs.Bool("require-signature", false, "Fail unless the release signature (Sigstore on Linux, Apple on macOS) was verified")
	verbose := fs.Bool("verbose", false, "Log info-level messages to stderr")
	debugFlag := fs.Bool("debug", false, "Log debug-level messages to stderr")

	fs.Usage = func() { printUpdateUsage(stderr, fs) }

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitUpdateOK
		}

		return exitUpdateUsage
	}

	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "Error: unexpected argument %q\n\n", fs.Arg(0))
		fs.Usage()

		return exitUpdateUsage
	}

	m := selfupdate.Method(strings.ToLower(strings.TrimSpace(*method)))
	switch m {
	case selfupdate.MethodAuto, selfupdate.MethodRelease, selfupdate.MethodGo:
	default:
		fmt.Fprintf(stderr, "Error: unknown -method %q; valid values are auto, release, go\n", *method)

		return exitUpdateUsage
	}

	var logger *slog.Logger
	switch {
	case *debugFlag:
		logger = slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	case *verbose:
		logger = slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	current := resolveVersion()

	statePath, err := selfupdate.DefaultStatePath()
	if err != nil {
		if logger != nil {
			logger.Debug("no user cache directory; the update budget is not persisted", "error", err)
		}
		statePath = ""
	}

	budget := selfupdate.NewBudget(statePath, selfupdate.LimitFromEnv(selfupdate.DefaultLimit))
	if err := budget.Load(); err != nil && logger != nil {
		logger.Debug("update state not loaded", "error", err)
	}

	updater := &selfupdate.Updater{
		Checker: selfupdate.NewChecker(current, selfupdate.DefaultExec),
		Client:  selfupdate.NewClient(current),
		Budget:  budget,
		Exec:    selfupdate.DefaultExec,
		Out:     stdout,
		Confirm: confirmer(stdin, stdout, stderr),
		Logger:  logger,
	}

	fmt.Fprintln(stdout, "Checking for updates…")

	result, err := updater.Run(ctx, selfupdate.Options{
		CurrentVersion:   current,
		Method:           m,
		Version:          strings.TrimSpace(*version),
		Check:            *check,
		Refresh:          *refresh,
		Force:            *force,
		AssumeYes:        *yes,
		RequireSignature: *requireSig,
	})
	if err != nil {
		return reportUpdateError(stderr, err)
	}

	if result.Changed {
		if result.To != "" {
			fmt.Fprintf(stdout, "\n✅ Updated to %s\n", result.To)
		} else {
			fmt.Fprintf(stdout, "\n✅ Updated to %s\n", result.Tag)
		}
	}

	return exitUpdateOK
}

// reportUpdateError prints the failure and its remedy and maps it to an exit code.
func reportUpdateError(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "\nError: %v\n", err)

	if remedy := selfupdate.RemedyOf(err); remedy != "" {
		fmt.Fprintf(stderr, "\n%s\n", remedy)
	}

	if selfupdate.IsPrerequisite(err) {
		return exitUpdatePrerequisite
	}

	return exitUpdateFailed
}

// confirmer returns a y/N prompt bound to the given streams. When stdin is
// not a terminal the prompt declines instead of blocking, so a piped
// invocation without --yes installs nothing.
func confirmer(stdin io.Reader, stdout, stderr io.Writer) func(string) (bool, error) {
	interactive := false
	if f, ok := stdin.(*os.File); ok {
		if info, err := f.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
			interactive = true
		}
	}

	reader := bufio.NewReader(stdin)

	return func(prompt string) (bool, error) {
		if !interactive {
			fmt.Fprintln(stderr, "stdin is not a terminal; pass -yes to update without confirmation")

			return false, nil
		}

		fmt.Fprintf(stdout, "\n%s [y/N] ", prompt)

		answer, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return false, fmt.Errorf("reading the confirmation: %w", err)
		}

		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "y", "yes":
			return true, nil
		default:
			return false, nil
		}
	}
}

// printUpdateUsage prints the help for the update verb.
func printUpdateUsage(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprintf(w, "Update %s to the latest release.\n\n", applicationName)
	fmt.Fprintf(w, "Usage:\n  %s update [options]\n\n", applicationName)
	fmt.Fprintf(w, "The newest release is looked up on github.com; the answer is cached for an hour and at most\n")
	fmt.Fprintf(w, "%d live lookups per hour are made (%s overrides). Nothing is downloaded until every\n", selfupdate.DefaultLimit, selfupdate.EnvBudget)
	fmt.Fprintf(w, "check has passed and you have confirmed. Root is never requested; where it is needed the\n")
	fmt.Fprintf(w, "exact command to re-run is printed.\n\n")
	fmt.Fprintf(w, "Options:\n")
	fs.VisitAll(func(f *flag.Flag) {
		name := "-" + f.Name
		switch f.Name {
		case "method":
			name += " M"
		case "version":
			name += " TAG"
		}
		printFlag(w, name, f.Usage)
	})
	fmt.Fprintf(w, "\nExamples:\n")
	fmt.Fprintf(w, "  %s update                     Check, show the plan, ask, install\n", applicationName)
	fmt.Fprintf(w, "  %s update -check              What would happen; nothing changes\n", applicationName)
	fmt.Fprintf(w, "  %s update -version v0.3.0     Install a specific release\n", applicationName)
	fmt.Fprintf(w, "  %s update -method go          Rebuild from source with go install\n", applicationName)
	fmt.Fprintf(w, "  sudo %s update -yes           Non-interactive, e.g. for the macOS package\n", applicationName)
	fmt.Fprintf(w, "\nExit Codes:\n")
	fmt.Fprintf(w, "  0  Updated, already current, -check, or declined\n")
	fmt.Fprintf(w, "  1  A prerequisite was not met; nothing was attempted\n")
	fmt.Fprintf(w, "  2  Invalid arguments\n")
	fmt.Fprintf(w, "  3  The update was attempted and failed\n")
}
