# Verifying releases

How to check that a McHarbor image, agent image or native package came out of the project's
own release workflow, and how to get the software bill of materials (SBOM) that describes
what is inside it.

## What the release workflow produces

The `release` workflow (`.github/workflows/release.yml`) runs when the maintainer pushes to
the `release` branch. For each artifact it produces:

| Artifact | SBOM | Signature |
|---|---|---|
| `ghcr.io/therealmcsparrow/mcharbor:<version>` | SPDX JSON, attached to the image as a cosign attestation and as a BuildKit SBOM attestation; also kept as the workflow artifact `mcharbor-image-sbom` | cosign keyless signature on the image digest |
| `ghcr.io/therealmcsparrow/mcharbor-agent:<version>` | The same, workflow artifact `mcharbor-agent-image-sbom` | cosign keyless signature on the image digest |
| Native packages (`.deb`, `.rpm`, tarball) | `mcharbor-packages.spdx.json` next to the packages | `SHA256SUMS` signed with `cosign sign-blob`; the bundle is `SHA256SUMS.cosign.bundle` |

Each image also carries a SLSA provenance attestation (`provenance: mode=max`), which records
the workflow run, the commit and the build parameters.

**Keyless** means there is no long-lived signing key to leak or rotate. The workflow proves
its identity to Sigstore with a short-lived GitHub OIDC token; Sigstore's Fulcio issues a
certificate naming the workflow file and the branch, and the signature is logged in the
public Rekor transparency log. Verifying therefore means checking *who* signed (the exact
workflow on the exact branch), not holding a public key.

Only the signing jobs get the `id-token: write` permission; the build and test jobs do not.

## What you need

- [`cosign`](https://docs.sigstore.dev/cosign/system_config/installation/) v2.
- Network access to `ghcr.io` and to Sigstore's public Rekor instance.
- `jq` for the SBOM extraction examples.

## The identity to check

Every command below pins the same two values. Anything signed by a different workflow, a
different branch or a fork fails verification.

```bash
export MCHARBOR_ID='^https://github.com/therealmcsparrow/McHarbor/\.github/workflows/release\.yml@refs/heads/release$'
export MCHARBOR_ISSUER='https://token.actions.githubusercontent.com'
```

## Verify an image

```bash
cosign verify ghcr.io/therealmcsparrow/mcharbor:2026.09.01 \
  --certificate-identity-regexp "$MCHARBOR_ID" \
  --certificate-oidc-issuer "$MCHARBOR_ISSUER"
```

Success prints the verified signature payload, which names the image digest. Pin that digest
(`image: ghcr.io/therealmcsparrow/mcharbor@sha256:…`) in production rather than the tag: a tag
can be moved, a digest cannot.

The agent image is verified the same way with `ghcr.io/therealmcsparrow/mcharbor-agent:<version>`.

## Verify and extract the SBOM

The SBOM is attached as an attestation of type `spdxjson`. Verifying it checks the same
identity, and the payload is the SBOM itself:

```bash
cosign verify-attestation ghcr.io/therealmcsparrow/mcharbor:2026.09.01 \
  --type spdxjson \
  --certificate-identity-regexp "$MCHARBOR_ID" \
  --certificate-oidc-issuer "$MCHARBOR_ISSUER" \
  | jq -r '.payload' | base64 -d | jq '.predicate' > mcharbor.spdx.json
```

The BuildKit copy of the SBOM can also be read without cosign, though it is not signed by
the keyless identity:

```bash
docker buildx imagetools inspect ghcr.io/therealmcsparrow/mcharbor:2026.09.01 --format '{{ json .SBOM }}'
```

Feed `mcharbor.spdx.json` to a scanner that reads SPDX (Grype, Trivy, Dependency-Track) to
check the contents against vulnerability databases.

## Verify a downloaded SBOM file

When the SBOM comes from the workflow artifact rather than the registry, verify the file
against its bundle:

```bash
cosign verify-blob mcharbor-2026.09.01.spdx.json \
  --bundle mcharbor-2026.09.01.spdx.json.cosign.bundle \
  --certificate-identity-regexp "$MCHARBOR_ID" \
  --certificate-oidc-issuer "$MCHARBOR_ISSUER"
```

## Verify native packages

The packages are covered by one signed checksum file. Verify the signature first, then the
checksums:

```bash
cosign verify-blob SHA256SUMS \
  --bundle SHA256SUMS.cosign.bundle \
  --certificate-identity-regexp "$MCHARBOR_ID" \
  --certificate-oidc-issuer "$MCHARBOR_ISSUER"
sha256sum --ignore-missing -c SHA256SUMS
```

## Enforcing it in a cluster

A Kubernetes admission controller that understands Sigstore (the Sigstore policy-controller,
Kyverno's `verifyImages` rule) can refuse any McHarbor image that does not verify against the
identity above. Use the same `MCHARBOR_ID` pattern and issuer.

## What goes wrong

| Symptom | Cause |
|---|---|
| `no matching signatures` | The image was not produced by the release workflow: a local build, a `dev`/`test` tag, or a fork. Only images pushed by `release.yml` are signed |
| `none of the expected identities matched` | The identity regexp does not match the certificate. Check the repository name's case (`McHarbor`) and that the tag came from the `release` branch |
| `no matching attestations` for `--type spdxjson` | The image predates SBOM attestation, or the tag points at a manifest the workflow did not attest. Verify by digest |
| `sha256sum: WARNING: 1 computed checksum did NOT match` | The package was altered or truncated in transit. Download it again; do not install it |

## Related

- [Upgrading](../upgrading/README.md)
- [Deployment](../deployment/README.md)
- The compliance dashboard's **SBOM and image signing** section points here; see
  [Compliance](../../screens/enterprise/compliance.md).
