# Trivy

Scans images with Aqua Security's Trivy, covering OS packages and language dependencies. The binary runs where McHarbor runs: shipped in the container image, downloaded by the plugin into its own folder on a native install (see [On a native install](../README.md#on-a-native-install)). There is no external service to stand up.

- **Slug** `scanner-trivy`, **version** 2026.09.01, **scanner type** `trivy`
- **Host range** `>=2026.09.01`
- **Where it appears** Security → Scanners, as a scanner type
- **Gated by** `scans.view` to read results, `scans.manage` (or `settings.manage`) to configure a scanner

## How it works

- `severity` is a comma-separated Trivy severity list and defaults to `LOW`, which reports everything from LOW up. Reporting broadly and filtering in McHarbor keeps history comparable between scans.
- `ignore_unfixed` hides vulnerabilities with no fix. Useful for a patch-driven workflow, misleading for a risk review: an unfixed critical is still a critical.
- `scanners` defaults to `vuln`. McHarbor stores vulnerability findings, so adding `secret`, `misconfig` or `license` costs scan time without producing anything the UI shows.
- `insecure_registry` skips TLS verification when pulling image metadata. Use it only for self-signed registries on a trusted network.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `EncryptedCredentials` | No | The configuration holds a secret, encrypted at rest with AES-256-GCM. |
| `RequiresLocalBinary` | Yes | The scanner runs as a binary beside McHarbor rather than calling a remote service. |
| `SupportsTest` | Yes | The scanner answers a reachability and configuration test from the Scanners screen. |

## In this folder

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |
| [Developer](../../../development/plugins/scanners/trivy.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Image scanners](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
