## mcharbor capacity

Disk and storage headroom, and what can be reclaimed

### Synopsis

Projections off McHarbor's retained hourly capacity series.

A subject with fewer than seven days of history, too noisy a series or no upward
trend reports its refusal reason instead of a date. That is deliberate: a date
extrapolated from three samples is worse than no date, because somebody acts on it.

Requires the host.view permission.

```
mcharbor capacity [flags]
```

### Options

```
  -h, --help   help for capacity
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

* [mcharbor](mcharbor.md)	 - Operate McHarbor from a terminal or a CI job
* [mcharbor capacity idle](mcharbor_capacity_idle.md)	 - Containers idle long enough to be worth reclaiming
* [mcharbor capacity list](mcharbor_capacity_list.md)	 - Projected exhaustion per filesystem and storage destination

