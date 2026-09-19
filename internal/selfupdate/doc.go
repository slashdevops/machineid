// Package selfupdate implements `machineid update`, which replaces the
// running CLI binary with a published release.
//
// It is a feature of the command-line tool only. The importable library at
// the module root has no update code, makes no network requests and gains no
// dependencies; this package is internal and cannot be imported from outside
// the module.
//
// # Flow
//
//	Checker.Run            probe the machine (local, free)
//	Checker.Resolve        pick release or go install
//	Updater.plan           resolve the tag via cache/budget, name the asset, HEAD it
//	CheckInstallTarget     will the install land where this binary lives?
//	Fetch                  download, SHA-256
//	Verify*                Sigstore (cosign) / Apple (pkgutil, codesign)
//	ReplaceBinary/InstallPkg/GoInstall
//	ConfirmVersion         run the result and read its version back
//
// Everything before Fetch costs no bandwidth, so -check is cheap and a
// permission or location problem is found before a download.
//
// # A public repository needs no API and no token
//
// The newest tag is read from the redirect that
// https://github.com/<repo>/releases/latest returns; a tag's existence from the
// status of its release page; assets from releases/download/<tag>/<name>.
// None of these are the REST API, so the unauthenticated 60-requests-per-hour
// quota never applies and no credential is needed, which a public binary could
// not keep secret anyway.
//
// # The hourly budget
//
// Live lookups are throttled per user per machine: at most [DefaultLimit] per
// rolling [Window], with the last answer cached for [CacheTTL]. The state
// lives in the user cache directory. Downloads are not counted; they only
// follow a confirmed plan to install a newer version. A missing or corrupt
// state file never blocks an update: the budget is a courtesy to github.com,
// not a security control. [EnvBudget] overrides the limit.
//
// # Nothing runs without asking
//
// Normal machineid runs never touch the network. Only the update verb does,
// and it never escalates privileges: where root is required the error carries
// the exact command to re-run.
//
// # Both install methods have a fixed destination
//
// `installer -pkg` always writes /usr/local/bin and `go install` always
// writes GOBIN, neither consulting the running binary. An update landing
// elsewhere would report success while the copy on PATH stays old, so
// [Checker.CheckInstallTarget] guards both directions and -force overrides
// it. The Linux zip and the macOS universal zip replace the binary in place
// through an atomic rename staged in the target's own directory.
//
// # Errors carry their remedy
//
// Every failure implements [Error], adding Remedy() so the command can print
// what to do next. [PrerequisiteError] wraps failures that happened before
// anything was installed, which the command maps to a distinct exit code.
package selfupdate
