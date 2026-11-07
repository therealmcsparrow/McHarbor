## mcharbor host updates status

The last check's result and the stored schedule, without running a check

```
mcharbor host updates status [flags]
```

### Examples

```
  mcharbor host updates status --env prod-01
```

### Options

```
  -h, --help   help for status
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
      --self               act on the machine McHarbor itself runs on instead of an environment
      --timeout duration   give up on a request after this long; 0 waits forever (default 1m0s)
      --url string         McHarbor URL, overriding the context ($MCHARBOR_URL)
      --yaml               shorthand for --output yaml
      --yes                skip the prompt for disruptive but reversible changes
```

### SEE ALSO

* [mcharbor host updates](mcharbor_host_updates.md)	 - OS package updates of a host: check, apply, schedule

