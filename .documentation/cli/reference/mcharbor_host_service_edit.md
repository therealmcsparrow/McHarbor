## mcharbor host service edit

Replace the unit's drop-in override with a file, like systemctl edit

### Synopsis

Writes /etc/systemd/system/<unit>.d/override.conf atomically on the host and runs
daemon-reload unless --no-reload. --verify runs systemd-analyze verify and prints its
findings as a warning; they never block the save. `-` reads the override from stdin.

```
mcharbor host service edit <unit> --file <override.conf> [flags]
```

### Examples

```
  mcharbor host service edit nginx.service --file override.conf --env prod-01 --yes
```

### Options

```
      --file string   the override file to write, or - for stdin
  -h, --help          help for edit
      --no-reload     write the override without running daemon-reload
      --verify        run systemd-analyze verify and print its findings
```

### Options inherited from parent commands

```
      --ca-file string     PEM root to trust for the server certificate ($MCHARBOR_CA_FILE)
      --confirm string     name the target of an irreversible change to confirm it non-interactively
      --context string     context to use instead of the current one ($MCHARBOR_CONTEXT)
      --env string         environment id for scoped commands ($MCHARBOR_ENV)
      --insecure           skip certificate verification; for labs only ($MCHARBOR_INSECURE=true)
      --json               shorthand for --output json
  -o, --output string      output format: table, json, yaml or tsv (default "table")
      --timeout duration   give up on a request after this long; 0 waits forever (default 1m0s)
      --url string         McHarbor URL, overriding the context ($MCHARBOR_URL)
      --yaml               shorthand for --output yaml
      --yes                skip the prompt for disruptive but reversible changes
```

### SEE ALSO

* [mcharbor host service](mcharbor_host_service.md)	 - Systemd services on a native agent host

