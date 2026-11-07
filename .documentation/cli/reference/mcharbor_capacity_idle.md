## mcharbor capacity idle

Containers idle long enough to be worth reclaiming

### Synopsis

Containers that have been stopped, or running with no CPU and no traffic, for
longer than the threshold, with what each one holds.

A size of "unknown" is not zero: writable-layer sizes are not collected for agent
environments, so the HOLDS column reports the reason rather than a wrong number.

This lists and never removes. Reclaim with `mcharbor docker prune`.

```
mcharbor capacity idle [flags]
```

### Examples

```
  mcharbor capacity idle
  mcharbor capacity idle --days 60 -o json
```

### Options

```
      --days int   how long a container must have been idle (1-365) (default 14)
  -h, --help       help for idle
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

* [mcharbor capacity](mcharbor_capacity.md)	 - Disk and storage headroom, and what can be reclaimed

