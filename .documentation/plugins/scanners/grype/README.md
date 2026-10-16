# Grype

Scans images with Anchore's Grype, covering OS and language packages. Like Trivy it runs where McHarbor runs (shipped in the container image, downloaded by the plugin into its own folder on a [native install](../README.md#on-a-native-install)) and needs no external service.

- **Slug** `scanner-grype`, **version** 2026.09.01, **scanner type** `grype`
- **Host range** `>=2026.09.01`
- **Where it appears** Security → Scanners, as a scanner type
- **Gated by** `scans.view` to read results, `scans.manage` (or `settings.manage`) to configure a scanner

## How it works

- `scope` defaults to `squashed`, which scans the final filesystem, which is what actually runs. `all-layers` also reports packages that were deleted in a later layer, which inflates the count without changing your exposure.
- `add_cpes_if_none` infers CPEs for packages whose ecosystem has no advisory feed. It finds more, and produces more false positives.
- `only_fixed` narrows to vulnerabilities with a fix available.

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
| [Developer](../../../development/plugins/scanners/grype.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Image scanners](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
