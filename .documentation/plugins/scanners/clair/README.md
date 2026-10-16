# Clair

Scans images against a Clair v4 server you run yourself: the image is indexed through Clair's indexer API, and matched CVEs come back from its matcher API.

- **Slug** `scanner-clair`, **version** 2026.09.01, **scanner type** `clair`
- **Host range** `>=2026.09.01`
- **Where it appears** Security → Scanners, as a scanner type
- **Gated by** `scans.view` to read results, `scans.manage` (or `settings.manage`) to configure a scanner

## How it works

- This is the only shipped scanner that is **not** a local binary. The work happens on the Clair server, which is what makes it the right choice when several tools should share one vulnerability database.
- The indexer and matcher paths are appended to `server_url` automatically; give the base URL only.
- `timeout_seconds` bounds each index and report call. Indexing a large image on a cold Clair can take a while, and the host's own scan timeout applies on top.
- `insecure_skip_verify` accepts any certificate from the Clair server; use it only for a self-signed certificate on a trusted network.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `EncryptedCredentials` | Yes | The configuration holds a secret, encrypted at rest with AES-256-GCM. |
| `RequiresLocalBinary` | No | The scanner runs as a binary inside the McHarbor container rather than calling a remote service. |
| `SupportsTest` | Yes | The scanner answers a reachability and configuration test from the Scanners screen. |

## In this folder

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |
| [Developer](../../../development/plugins/scanners/clair.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Image scanners](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
