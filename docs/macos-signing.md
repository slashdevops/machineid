# macOS Code Signing & Notarization

This document covers every secret, certificate, and piece of configuration required to produce signed and notarized macOS `.pkg` installers through the release workflow.

---

## Prerequisites

| Requirement | Details |
| --- | --- |
| Apple Developer Program membership | Paid individual or organization account at [developer.apple.com](https://developer.apple.com) |
| Certificate: Developer ID Application | Required for code-signing the binaries with `codesign` |
| Certificate: Developer ID Installer | Required for signing the `.pkg` installers with `productsign` |
| Xcode Command Line Tools | Pre-installed on GitHub's `macos-latest` runner |

> **Important**: You need **two** Developer ID certificates — one for binaries (`Application`) and one for installers (`Installer`). Both should be exported into a single `.p12` file.

### Creating the certificates

If you don't already have both certificates:

1. Go to [developer.apple.com/account/resources/certificates](https://developer.apple.com/account/resources/certificates).
2. Click **+** to create a new certificate.
3. Under **Software**, select **Developer ID Application** and follow the CSR steps. Download and install it.
4. Click **+** again, select **Developer ID Installer**, and repeat. Download and install it.
5. Both certificates should now appear in **Keychain Access** under **My Certificates**.

---

## Secrets Reference

Add all of the following in your GitHub repository under **Settings > Secrets and variables > Actions > Repository secrets**.

### `MACOS_CERTIFICATE`

Base64-encoded `.p12` certificate file containing **both** the Developer ID Application and Developer ID Installer certificates with their private keys.

**How to export and encode:**

1. Open **Keychain Access** on your Mac.
2. Select both certificates:
   - **Developer ID Application: Your Name (XXXXXXXXXX)**
   - **Developer ID Installer: Your Name (XXXXXXXXXX)**
3. Right-click > **Export 2 items** > choose `.p12` format > save as `certificate.p12`.
4. Choose a strong export password (this becomes `MACOS_CERTIFICATE_PASSWORD`).
5. Encode it:

   ```bash
   base64 -i certificate.p12 | pbcopy
   ```

6. Paste the clipboard contents as the secret value.

### `MACOS_CERTIFICATE_PASSWORD`

The password you chose when exporting the `.p12` from Keychain Access.

### `MACOS_SIGNING_IDENTITY`

The exact **Developer ID Application** signing identity string for binary code-signing.

```bash
security find-identity -v -p codesigning
```

Example: `Developer ID Application: Acme Corp (AB12CD34EF)`

### `MACOS_INSTALLER_SIGNING_IDENTITY`

The exact **Developer ID Installer** signing identity string for `.pkg` signing.

```bash
security find-identity -v
```

Example: `Developer ID Installer: Acme Corp (AB12CD34EF)`

### `MACOS_NOTARIZATION_APPLE_ID`

The Apple ID (email) associated with your Apple Developer Program account.

### `MACOS_NOTARIZATION_PASSWORD`

An **app-specific password** — not your Apple ID password. Generate at [appleid.apple.com](https://appleid.apple.com) > Sign-In and Security > App-Specific Passwords.

### `MACOS_NOTARIZATION_TEAM_ID`

Your 10-character Apple Developer Team ID from [developer.apple.com/account](https://developer.apple.com/account) > Membership Details.

---

## Summary Table

| Secret name | Example value | Where to get it |
| --- | --- | --- |
| `MACOS_CERTIFICATE` | `MIIKxAIBAzCC...` (base64) | Keychain Access > Export both certs > `base64 -i cert.p12` |
| `MACOS_CERTIFICATE_PASSWORD` | `MyStr0ngP@ss!` | Password chosen during `.p12` export |
| `MACOS_SIGNING_IDENTITY` | `Developer ID Application: Acme Corp (AB12CD34EF)` | `security find-identity -v -p codesigning` |
| `MACOS_INSTALLER_SIGNING_IDENTITY` | `Developer ID Installer: Acme Corp (AB12CD34EF)` | `security find-identity -v` |
| `MACOS_NOTARIZATION_APPLE_ID` | `dev@example.com` | Apple ID email |
| `MACOS_NOTARIZATION_PASSWORD` | `xxxx-xxxx-xxxx-xxxx` | appleid.apple.com > App-Specific Passwords |
| `MACOS_NOTARIZATION_TEAM_ID` | `AB12CD34EF` | developer.apple.com > Membership Details |

---

## Set Secrets with GitHub CLI

```bash
gh secret set MACOS_CERTIFICATE --repo slashdevops/machineid --body "$(base64 -i certificate.p12)"
gh secret set MACOS_CERTIFICATE_PASSWORD --repo slashdevops/machineid
gh secret set MACOS_SIGNING_IDENTITY --repo slashdevops/machineid --body "Developer ID Application: Your Name (TEAMID)"
gh secret set MACOS_INSTALLER_SIGNING_IDENTITY --repo slashdevops/machineid --body "Developer ID Installer: Your Name (TEAMID)"
gh secret set MACOS_NOTARIZATION_APPLE_ID --repo slashdevops/machineid --body "yourname@example.com"
gh secret set MACOS_NOTARIZATION_PASSWORD --repo slashdevops/machineid
gh secret set MACOS_NOTARIZATION_TEAM_ID --repo slashdevops/machineid --body "AB12CD34EF"
```

---

## What the CI Does With These Secrets

1. Decodes `MACOS_CERTIFICATE` into a temporary `.p12` file.
2. Creates an ephemeral keychain on the runner (deleted after the job, via `if: always()`).
3. Imports the certificates into that keychain and grants `codesign` and `productsign` access.
4. Builds darwin arm64 + amd64 binaries, merges them into a universal binary with `lipo`.
5. Signs the binary with `codesign --options runtime --timestamp` (hardened runtime required for notarization).
6. Creates a `.pkg` installer using `pkgbuild` (installs to `/usr/local/bin`).
7. Signs the `.pkg` with `productsign` using the Developer ID Installer certificate.
8. Submits the `.pkg` to Apple's notarization service via `xcrun notarytool submit --wait`.
9. Staples the notarization ticket to the `.pkg` with `xcrun stapler staple`.
10. Uploads the `.pkg` as a GitHub Release asset.
