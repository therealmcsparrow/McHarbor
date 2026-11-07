## mcharbor host service revert

Delete the unit's McHarbor override and reload systemd (irreversible)

### Synopsis

The override's content is gone afterwards; `cat` it first if you may want it back.

```
mcharbor host service revert <unit> [flags]
```

### Examples

```
  mcharbor host service revert nginx.service --env prod-01 --confirm nginx.service
```

### Options

```
  -h, --help   help for revert
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

