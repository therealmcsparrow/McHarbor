## mcharbor cluster failover

Hand the active role to another node (active/passive only)

### Synopsis

Marks node-id as the preferred active. The current active releases the active lock on its next tick and node-id takes it, so the change is a handover rather than a race. Refused during a split brain or when node-id is not live.

```
mcharbor cluster failover <node-id> [flags]
```

### Options

```
  -h, --help   help for failover
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

* [mcharbor cluster](mcharbor_cluster.md)	 - Inspect a multi-node installation

