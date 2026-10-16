# Trivy: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- Outbound access for Trivy to refresh its vulnerability database, and disk for that database inside the container.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `severity` | string | No | `LOW` | Comma-separated Trivy severities to report: UNKNOWN, LOW, MEDIUM, HIGH, CRITICAL. Reporting everything and filtering in McHarbor keeps the history complete; raising the floor here permanently discards the findings below it. |
| `ignore_unfixed` | bool | No | — | Report only vulnerabilities that have a fix available. Useful for a patch-driven workflow, misleading for a risk review: an unfixed critical is still a critical. |
| `scanners` | string | No | `vuln` | Trivy's own scanner list: vuln, secret, misconfig, license. McHarbor stores vulnerability findings, so anything beyond vuln costs scan time without adding rows. |
| `insecure_registry` | bool | No | — | Skip TLS verification when pulling image metadata. Only for a registry with a self-signed certificate on a trusted network. |

Secrets are encrypted with the host's AES-256-GCM service before they are written, and
the API never returns them once saved. Editing an existing entry leaves a stored secret
in place unless you replace it.

## Setting it up

1. Install the plugin from **App Store → Plugins**.
2. Open **Security → Scanners** and configure it.
3. Press **Test**, then scan an image from the Images page.
4. Findings appear on the image, and on every container running it.

## Host scopes it requests

These are the capabilities the manifest asks the host for. The installer refuses a
manifest asking for a scope outside the known set.

| Scope token | What it allows |
|---|---|
| `scanner.adapter.register` | Register an image scanner with the host scanner registry. |
| `audit.emit` | Write entries into the tamper-evident audit trail. |

## Related

- [Trivy](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
