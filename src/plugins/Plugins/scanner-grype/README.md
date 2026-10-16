# Grype Scanner

Anchore's Grype: OS and language package vulnerabilities, scanned locally.

## What it does

Wraps the Grype CLI that ships in the McHarbor container image.

Grype and Trivy draw on overlapping but not identical advisory feeds,
so running both is a reasonable arrangement rather than a redundant
one. A scanner is a named instance, so both can be enabled at once.

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
./scripts/build-plugin-tarball.sh scanner-grype 1.0.0
```

The host and every plugin must be compiled in the same module context or
`plugin.Open` rejects the `.so` with an ABI mismatch, which is why the
build runs inside the Docker backend stage rather than on a workstation.
