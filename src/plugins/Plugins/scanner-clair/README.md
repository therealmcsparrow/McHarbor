# Clair Scanner

Red Hat's Clair v4: image indexing and CVE matching against an external Clair server.

## What it does

Calls an external Clair v4 server: it submits an index report to the
indexer API, then reads the matched CVEs back from the matcher API.

This is the only first-party scanner that depends on a service outside
the container, so it is the only one with a required config field
(`server_url`) and a stored secret (`auth_token`). Because a scanner is
a named instance rather than a singleton, one install of this plugin can
back a production Clair and a staging one at the same time.

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
./scripts/build-plugin-tarball.sh scanner-clair 1.0.0
```

The host and every plugin must be compiled in the same module context or
`plugin.Open` rejects the `.so` with an ABI mismatch, which is why the
build runs inside the Docker backend stage rather than on a workstation.
