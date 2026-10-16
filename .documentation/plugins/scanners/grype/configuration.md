# Grype: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- Outbound access for Grype to refresh its vulnerability database.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `only_fixed` | bool | No | — | Report only vulnerabilities with a fix available. Useful for a patch-driven workflow, misleading for a risk review: an unfixed critical is still a critical. |
| `scope` | string | No | `squashed` | One of squashed or all-layers. `squashed` scans the final filesystem, which is what actually runs; `all-layers` also reports packages deleted in a later layer, which inflates counts with things that are not present. |
| `add_cpes_if_none` | bool | No | — | Generate CPEs for packages whose ecosystem has no advisory feed. Finds more, and produces more false positives. Leave off unless you are chasing a specific gap. |

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

- [Grype](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
