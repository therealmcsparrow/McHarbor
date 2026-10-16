# Trivy Scanner

Aqua Security's Trivy: OS packages and language dependencies, scanned locally.

## What it does

Wraps the Trivy CLI that ships in the McHarbor container image.

Trivy was the built-in default scanner before the engines moved into
plugins, so the host auto-installs this plugin on an upgrade that had
scanning configured. A fresh install that never scanned gets nothing
installed on its behalf.

## What it does not own

Scan rows, findings, severity rollups, retention and the scan history
page all belong to the host. This plugin turns an image reference into a
list of vulnerabilities and returns them; it keeps no tables and ships no
migration, so uninstalling it takes no scan history with it.

## Configure

Security ▸ Scanners ▸ Add scanner. The form is rendered generically from
the adapter's declared `ConfigSchema`, so there is no frontend bundle in
this plugin.

## Build

```bash
./scripts/build-plugin-tarball.sh scanner-trivy 1.0.0
```

The host and every plugin must be compiled in the same module context or
`plugin.Open` rejects the `.so` with an ABI mismatch, which is why the
build runs inside the Docker backend stage rather than on a workstation.
