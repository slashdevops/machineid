# Linux Binary Signing with Sigstore Cosign

Linux binaries are signed using [Sigstore Cosign](https://docs.sigstore.dev/cosign/overview/) in keyless mode. This provides cryptographic provenance without managing signing keys.

---

## How It Works

The release workflow signs each Linux binary using **keyless signing** via the Sigstore public infrastructure:

1. GitHub Actions authenticates via OIDC to Sigstore's Fulcio CA.
2. Cosign generates an ephemeral certificate tied to the GitHub Actions workflow identity.
3. The signature and certificate are bundled into a `.sigstore.json` file.
4. The bundle is uploaded alongside the binary as a release asset.

No secrets are required — authentication is handled automatically via GitHub's OIDC token.

---

## Verifying a Binary

Download both the binary zip and its `.sigstore.json` bundle from the [releases page](https://github.com/slashdevops/machineid/releases), then verify:

```bash
# Install cosign
go install github.com/sigstore/cosign/v2/cmd/cosign@latest

# Verify the binary against its Sigstore bundle
cosign verify-blob \
  --bundle machineid-linux-amd64.sigstore.json \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp "github.com/slashdevops/machineid" \
  machineid-linux-amd64
```

A successful verification confirms:
- The binary was built by the `slashdevops/machineid` GitHub Actions workflow.
- The binary has not been tampered with since signing.

---

## Workflow Permissions

The release workflow requires `id-token: write` permission to request an OIDC token from GitHub for Sigstore authentication. This is already configured in the workflow.
