# ⬆️ Updating the `machineid` CLI

`machineid update` replaces the binary you are running with the newest published release. This guide is for people who **use** the tool. If you want to know how it is built, read the package documentation in [`internal/selfupdate`](../internal/selfupdate/doc.go).

> **This is a feature of the command-line tool only.** If you use `machineid` as a Go library in your own program, nothing here applies to you: the library has no update code and never makes a network request.

---

## Contents

- [Quick start](#quick-start)
- [What happens when you run it](#what-happens-when-you-run-it)
- [Flags](#flags)
- [Exit codes](#exit-codes)
- [Per-platform behaviour](#per-platform-behaviour)
- [Where it installs, and why that matters](#where-it-installs-and-why-that-matters)
- [Checking for updates without installing](#checking-for-updates-without-installing)
- [Installing a specific version, or going back](#installing-a-specific-version-or-going-back)
- [Network use, caching and the hourly limit](#network-use-caching-and-the-hourly-limit)
- [Verification: checksums and signatures](#verification-checksums-and-signatures)
- [Using it from scripts, cron and CI](#using-it-from-scripts-cron-and-ci)
- [Privacy](#privacy)
- [Troubleshooting](#troubleshooting)
- [FAQ](#faq)

---

## Quick start

```bash
machineid update
```

That is all most people need. It shows a checklist, tells you what it is about to do, asks `[y/N]`, and only then downloads anything.

To see what would happen without changing anything:

```bash
machineid update -check
```

To update without being asked (scripts, CI):

```bash
machineid update -yes
```

On macOS, if you installed with the `.pkg`, the installer needs root:

```bash
sudo machineid update -yes
```

`machineid` never asks for your password itself. If root is needed, it stops before downloading and prints the exact command to run.

---

## What happens when you run it

```text
$ machineid update
Checking for updates…
  ✓ current version          v0.2.0
  ✓ running binary           /usr/local/bin/machineid
  ✓ platform supported       darwin/arm64
  ✓ latest release           v0.3.0  (live, 4 of 5 checks left this hour)
  ✓ install target           /usr/local/bin (running as root)

→ Updating machineid v0.2.0 → v0.3.0 using the signed macOS package

Update machineid now? [y/N] y
   downloading machineid-darwin-universal.pkg…
   ✓ SHA-256 verified
   ✓ pkgutil: Developer ID Installer: SlashDevOps
   ✓ installed to /usr/local/bin

✅ Updated to v0.3.0
```

In order:

1. **Checklist.** Each line is one thing that has to be true. A failing line shows `✗` and the reason.
2. **Latest release.** Looked up on github.com, or taken from the local cache when it is under an hour old. The note in brackets tells you which.
3. **Install target.** Confirms the update will land on the binary you are running, and that you may write there.
4. **The plan.** One line saying from which version, to which version, and how.
5. **Confirmation.** `y` proceeds, anything else cancels. Skip with `-yes`.
6. **Download and verify.** The asset and its checksum are downloaded; the SHA-256 must match. Then the signature is checked (see [Verification](#verification-checksums-and-signatures)).
7. **Install.** In place, or through the macOS installer, or with `go install`.
8. **Confirm.** The new binary is run with `-version` and the answer is compared with what was installed.

If anything fails before step 6, **nothing has been downloaded and nothing has changed**. If the checksum or signature fails in step 6, the old binary is untouched.

---

## Flags

| Flag | Meaning |
|------|---------|
| `-check` | Report what would happen and exit. Nothing is downloaded. Uses the cached answer when it is under an hour old. |
| `-refresh` | Look up the latest release now instead of using the cache. Counts against the hourly limit. |
| `-version TAG` | Install exactly this release, e.g. `-version v0.2.0`. Works for older versions too. |
| `-method auto\|release\|go` | How to install. `auto` (default) uses `release` where a signed asset exists for your platform and `go` otherwise. See [Per-platform behaviour](#per-platform-behaviour). |
| `-force` | Two things: install to the method's location even if this binary lives elsewhere, and reinstall a version that is already installed. |
| `-yes` | Do not ask for confirmation. Required when stdin is not a terminal. |
| `-require-signature` | Fail if the signature could not be verified. Without it, a missing verification tool only prints a warning. |
| `-verbose`, `-debug` | Log to stderr, like the rest of the CLI. `-debug` shows every command run and every HTTP request made. |
| `-h` | Help. |

Flags take one or two dashes (`-check` and `--check` are the same), like every other `machineid` flag.

---

## Exit codes

| Code | Meaning | What to do |
|------|---------|------------|
| `0` | Updated, or already on the latest version, or `-check`, or you declined. | Nothing. |
| `1` | A prerequisite was not met. **Nothing was attempted and nothing changed.** | Read the remedy printed under the error, fix it, run again. |
| `2` | Invalid arguments. | Run `machineid update -h`. |
| `3` | The update was attempted and failed (download, checksum, signature, install). The old binary is left untouched on checksum or signature failure. | Read the message. Retrying is safe. |

A script can rely on `1` meaning "safe to retry later, nothing happened" and `3` meaning "look at this".

---

## Per-platform behaviour

| Platform | Default method | Asset | Installs to | Needs root? |
|----------|----------------|-------|-------------|-------------|
| 🐧 Linux amd64 / arm64 | `release` | `machineid-linux-<arch>.zip` | **In place**, wherever the running binary is | Only if that directory is not writable by you |
| 🍎 macOS, binary in `/usr/local/bin` | `release` | `machineid-darwin-universal.pkg` (signed, notarized) | `/usr/local/bin` via the system installer | **Yes** (`sudo`), the installer updates the receipt database |
| 🍎 macOS, binary anywhere else (e.g. `go install`) | `release` | `machineid-darwin-universal.zip` (same signed binary) | **In place** | Only if that directory is not writable by you |
| 🪟 Windows | `go` | none published | `GOBIN` via `go install` | No |
| Any, with `-method go` | `go` | source | `GOBIN` (`go env GOBIN`, else `GOPATH/bin`) | No |

`-method go` needs a Go toolchain on PATH. It builds the release tag from source, so the resulting binary reports the correct version but does not carry the Apple signature or the release build metadata that `-version-long` shows.

---

## Where it installs, and why that matters

Two of the install methods have a **fixed destination** that ignores where your binary actually is:

- the macOS `.pkg` always writes `/usr/local/bin/machineid`;
- `go install` always writes `$GOBIN/machineid`.

If you are running `machineid` from somewhere else, one of those methods would "succeed" while the copy you actually use stays old. `machineid update` refuses that case:

```text
  ✗ install target           this method installs to /usr/local/bin, but the binary you are running is /Users/me/go/bin/machineid

Error: this method installs to /usr/local/bin, but the binary you are running is /Users/me/go/bin/machineid

Use machineid update -method go to update the copy you are running, or -force to install to /usr/local/bin anyway.
```

The remedy names the method that *does* target your copy when there is one. `-force` overrides the guard when you really want the other location populated, for example to put a fresh copy in `/usr/local/bin` from a locally built binary.

On Linux, and on macOS outside `/usr/local/bin`, there is no fixed destination: the binary is replaced **in place** with an atomic rename, so the only requirement is that its directory is writable. A running process keeps working; the *next* invocation is the new version.

---

## Checking for updates without installing

```bash
machineid update -check
```

```text
Checking for updates…
  ✓ current version          v0.2.0
  ✓ running binary           /usr/local/bin/machineid
  ✓ platform supported       linux/amd64
  ✓ latest release           v0.3.0  (cached 12m0s ago)
  ✓ install target           /usr/local/bin (in place)

→ Updating machineid v0.2.0 → v0.3.0 using the release archive machineid-linux-amd64.zip
   (-check: nothing was changed)
```

`-check` exits `0` whether or not an update is available. To act on the result in a script, compare versions yourself (see [Using it from scripts](#using-it-from-scripts-cron-and-ci)) or just run `machineid update -yes`, which is a no-op when already current.

`-check` prefers the cached answer. Add `-refresh` to force a live lookup.

---

## Installing a specific version, or going back

```bash
machineid update -version v0.2.0
```

An explicit tag is installed whether it is newer or older than what you have, so this is also how to **downgrade**. The tag must exist on the [releases page](https://github.com/slashdevops/machineid/releases) and carry an asset for your platform.

If you built `machineid` yourself, its version is something like `devel` or a branch name, which cannot be compared with a release. The updater says so and asks you to name a tag:

```text
Error: the running version "devel" is not a release version, so it cannot be compared with a release

Name the release to install explicitly, e.g. machineid update -version v0.3.0
```

---

## Network use, caching and the hourly limit

**Only `machineid update` uses the network.** Generating an ID, `-validate`, `-version`, everything else: no requests, ever. There is no background check and no "a new version is available" notice.

When it does run, the update needs to know the newest release. It asks `https://github.com/slashdevops/machineid/releases/latest`, which answers with a redirect to the newest tag. This is an ordinary page on github.com, not the GitHub API, so it needs no token and is not subject to the API's rate limit.

To be a good citizen anyway:

| Rule | Value |
|------|-------|
| The latest-release answer is cached for | **1 hour** |
| Live lookups allowed per user, per rolling hour | **5** |
| Override | `MACHINEID_UPDATE_BUDGET=<n>` (`0` disables the limit, please don't) |
| Downloads | not counted; they only happen after you confirm a newer version |

What "over the limit" looks like:

- `machineid update -check` still works and prints the cached answer with the time of the next allowed live lookup.
- `machineid update` (or `-refresh`) refuses with exit `1` and that same time in the remedy. Nothing is downloaded.

If GitHub itself asks to back off (HTTP 429), the wait it requested is honoured on later runs.

**Where the state lives.** One small JSON file, readable only by you:

| OS | Path |
|----|------|
| macOS | `~/Library/Caches/machineid/update-state.json` |
| Linux | `$XDG_CACHE_HOME/machineid/update-state.json`, usually `~/.cache/machineid/update-state.json` |
| Windows | `%LocalAppData%\machineid\update-state.json` |

It holds the last answer, its timestamp, the timestamps of recent lookups and any backoff. Delete it whenever you like; a missing, unreadable or unwritable file never prevents an update, it only means the limit is not enforced for that run.

**Proxies.** Standard `HTTPS_PROXY` / `NO_PROXY` environment variables are respected. `github.com` and `*.githubusercontent.com` must be reachable over HTTPS; downloads refuse to follow a redirect anywhere else.

---

## Verification: checksums and signatures

Every download is checked in two layers.

**1. SHA-256, always.** The release publishes a `.sha256` next to every asset. The download must match it or nothing is installed. This catches corruption and truncation.

**2. Signature, when possible.** This proves the asset was produced by this project's release workflow.

| Platform | How | Tool needed | If the tool is missing |
|----------|-----|-------------|------------------------|
| Linux | Sigstore keyless bundle (`*.sigstore.json`) checked against the release workflow identity `https://github.com/slashdevops/machineid/.github/workflows/release.yml` and issuer `token.actions.githubusercontent.com` | [`cosign`](https://docs.sigstore.dev/cosign/system_config/installation/) | Warning printed, install continues on the strength of SHA-256 + TLS |
| macOS `.pkg` | Apple Developer ID Installer certificate and notarization | `pkgutil` (always present) | n/a |
| macOS `.zip` | Apple code signature on the binary | `codesign` (always present) | n/a |

To make a skipped signature check a hard failure:

```bash
machineid update -require-signature
```

Recommended on Linux fleets where `cosign` is installed. A signature that **fails** is always fatal, with or without the flag.

To verify a download by hand, see [macOS signing](macos-signing.md) and [Linux signing](linux-signing.md).

---

## Using it from scripts, cron and CI

**Non-interactive update:**

```bash
machineid update -yes
case $? in
  0) echo "up to date" ;;
  1) echo "cannot update yet (see message above); nothing changed" ;;
  3) echo "update failed" ;;
esac
```

When stdin is not a terminal and `-yes` is absent, the prompt **declines** rather than blocking, and exits `0` without installing. Always pass `-yes` in automation.

**Nightly check, install only when a new version exists:**

```bash
# cron: 0 3 * * *   /usr/local/bin/machineid update -yes >> /var/log/machineid-update.log 2>&1
```

`update -yes` is a no-op when already current, so this is safe to run as often as you like; the hourly limit keeps it polite even at high frequency.

**Pin a version in CI:**

```bash
machineid update -yes -version v0.3.0 -require-signature
```

**macOS with the `.pkg`:** run under `sudo`, or install once with `go install` and let the updater manage that copy in place.

**Detect "update available" without installing:**

```bash
current=$(machineid -version | awk '{print $2}')
latest=$(curl -sI https://github.com/slashdevops/machineid/releases/latest | awk -F/ '/^location:/ {print $NF}' | tr -d '\r')
[ "$current" != "$latest" ] && echo "update available: $current -> $latest"
```

---

## Privacy

- The only request is to github.com, and only when you run `machineid update`.
- The request carries a `User-Agent` of `machineid/<version> (+https://github.com/slashdevops/machineid)` and nothing else: no machine ID, no hostname, no telemetry.
- Nothing is written outside the state file above and the binary being replaced.

---

## Troubleshooting

Every error prints a **remedy** underneath it. The common ones:

| You see | Meaning | Do this |
|---------|---------|---------|
| `cannot reach https://github.com/...` | No network, or a proxy blocks github.com | Check connectivity / `HTTPS_PROXY`. Exit 1, nothing changed. |
| `the local limit of 5 update checks per 1h0m0s is reached` | You ran it more than five times this hour with live lookups | Use `-check` (cached) or wait until the time shown. |
| `github.com answered HTTP 429 (rate limited)` | GitHub asked to back off | Wait until the time shown; later runs honour it automatically. |
| `this method installs to /usr/local/bin, but the binary you are running is …` | Fixed-destination mismatch | Use the method named in the remedy, or `-force`. See [Where it installs](#where-it-installs-and-why-that-matters). |
| `root privileges are required: the macOS installer updates the system receipt database` | `.pkg` needs root | `sudo machineid update -yes` |
| `/opt/tools is not writable` | In-place replacement needs write access to the directory | Run with enough privileges, or `chown` the directory. |
| `the running version "devel" is not a release version` | Locally built binary | `machineid update -version vX.Y.Z` |
| `release "v9.9.9" was not found` | Typo, or the tag was never published | Check the releases page. |
| `release v0.3.0 does not carry machineid-linux-arm64.zip` | That release was published without your platform's asset | Pick another `-version`, or `-method go`. |
| `does not match its published SHA-256` | Corrupt or tampered download; nothing installed | Retry. If it persists, open an issue. |
| `signature verification of … with cosign failed` | The bundle does not verify | Do **not** install. Retry; if it persists, open an issue. |
| `signature verification … cosign is not installed (-require-signature)` | You demanded a signature but have no `cosign` | Install cosign, or drop the flag. |
| `no release asset is published for windows/amd64` | No Windows binaries yet | `machineid update -method go` (needs Go). |
| `go is not installed` | `-method go` without a toolchain | Install Go, or use `-method release`. |
| `stdin is not a terminal; pass -yes to update without confirmation` | Piped or scripted run without `-yes` | Add `-yes`. |
| `… reports v0.2.0, not the v0.3.0 just installed` | The install landed somewhere other than the binary on your PATH | Check `which machineid`; you probably have two copies. |

`-debug` prints every command and every request, which is the fastest way to see what a failing run actually did.

---

## FAQ

**Does `machineid` check for updates on its own?**
No. Never. Only when you run `machineid update`.

**Does updating change my machine IDs?**
No. IDs depend on the hardware and on the version's collection logic. Release notes call out explicitly if a release changes how any component is collected; the updater itself does not alter anything but the binary.

**Can I update while `machineid` is running elsewhere?**
Yes. On Linux and macOS the running process keeps its old file open and finishes normally; new invocations use the new binary. On Windows the running `.exe` is renamed to `machineid.exe.old` and cleaned up on the next run.

**I installed with Homebrew / a package manager.**
Let that manager update it. `machineid update` will refuse to overwrite a location it does not own only if the directory is not writable; if it is writable, it will happily replace the file and your package manager will not know. Prefer one mechanism.

**Why does the macOS `.pkg` need root when `/usr/local/bin` is writable by me?**
Because `installer` also records the package in the system receipt database (`pkgutil --pkgs`), which is root-only. If you would rather not use `sudo`, install once with `go install` or by unzipping `machineid-darwin-universal.zip`; those copies update in place without root.

**Can I point it at a mirror or an internal GitHub?**
Not today. The repository and host are fixed in the binary so the signature identity can be pinned. Open an issue if you need this.

**What about air-gapped machines?**
Download the asset and its `.sha256` on a connected machine, verify by hand (see the signing docs), and replace the binary. `machineid update` needs github.com.
