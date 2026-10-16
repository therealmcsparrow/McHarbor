# Image Scanners

An image-scanner plugin answers one question for McHarbor: what is known to be wrong with
this image? The host owns scan scheduling, the findings store and the severity display;
the plugin runs the scan and returns findings.

| Plugin | Scanner type | Runs | What it scans with |
|---|---|---|---|
| [Clair](./clair/README.md) | `clair` | On a server you run | Scan container images against a Clair v4 server. |
| [Grype](./grype/README.md) | `grype` | Where McHarbor runs | Scan container images for vulnerabilities with Anchore's Grype. |
| [Trivy](./trivy/README.md) | `trivy` | Where McHarbor runs | Scan container images for vulnerabilities with Aqua Security's Trivy. |

Each plugin has a folder of its own: a README, the configuration reference, a
troubleshooting table, and a developer page covering the adapter it registers and how to
build, sign and fork it.

## What they have in common

- They register through `AdapterRegistry.Scanner()` and are configured under
  **Security → Scanners**.
- Findings are stored by McHarbor, so history and comparison across scans are the host's
  job, not the scanner's.
- All three support **Test**, which checks configuration and reachability without running
  a full scan.
- Severity filtering is available in every one of them, and in every one of them it is a
  reporting decision, not a security one: hiding unfixed vulnerabilities makes a patch
  queue tidier and a risk review wrong.

## On a native install

The container image ships the `trivy` and `grype` binaries in `/usr/local/bin`. A native
install (tarball, .deb or .rpm) ships neither, so the plugins fetch their own:

- When the plugin starts and finds no binary on McHarbor's `PATH`, it downloads one pinned
  release into its own install folder, `<data>/plugins/<slug>@<version>/bin/`: Trivy
  **0.75.0**, Grype **0.120.0**, for `linux/amd64` or `linux/arm64`.
- The archive is refused unless its SHA-256 matches the value pinned in the plugin, and the
  binary is moved into place only once it is complete. Nothing else in the archive is kept.
- Uninstalling the plugin deletes its install folder, and the binary with it. A newer plugin
  build may pin a newer release; the scanner moves with the plugin.
- A binary already on `PATH` always wins, so the container never downloads anything, and an
  air-gapped host can put its own copy in `/usr/local/bin`.

The download needs outbound access to `github.com` (and its release storage) once per
plugin install. Until it finishes the scanner reads **Binary missing**, and its **Test** says
whether the download is still running or why it failed; the reason is also logged as
`scanner-trivy: downloading trivy failed`. Their vulnerability databases are cached under the
`mcharbor` user's home, `/var/lib/mcharbor`. Clair needs no binary; it reads **Not
configured** until it has a server URL.

## A fresh install's scanner list

A fresh install lists only the scanners whose plugin is installed. An install that never
used the built-in scanners of earlier versions no longer shows Trivy and Grype rows reading
**Plugin removed**, one of them marked as the default: those rows were carried forward by a
migration that also ran on new databases, and are removed at start-up unless the engine is
installed, the row was configured, or the old scanner settings show it was used.

## Choosing

Trivy and Grype both run where McHarbor runs and need no infrastructure, so start
with either. Clair is the one to pick when a Clair server already exists, or when several
tools should share one vulnerability database.

Running two scanners is reasonable; their feeds and matching rules differ, so their counts
will differ too. Compare each against its own history, not against the other.

## Related

- [Plugin catalog](../catalog.md): every shipped plugin in one table
- [Plugins](../README.md): the section index
