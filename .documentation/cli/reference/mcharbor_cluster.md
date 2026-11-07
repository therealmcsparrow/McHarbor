## mcharbor cluster

Inspect a multi-node installation

```
mcharbor cluster [flags]
```

### Options

```
  -h, --help   help for cluster
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
* [mcharbor cluster agent-routing](mcharbor_cluster_agent-routing.md)	 - Show which node holds each remote agent and what the last cross-node call proved
* [mcharbor cluster drain](mcharbor_cluster_drain.md)	 - Stop sending a node new work; an active node hands over first
* [mcharbor cluster failover](mcharbor_cluster_failover.md)	 - Hand the active role to another node (active/passive only)
* [mcharbor cluster readiness](mcharbor_cluster_readiness.md)	 - Show every gate for an HA target and whether this node may run it
* [mcharbor cluster resume](mcharbor_cluster_resume.md)	 - Lift a drain so the node serves again and may become active
* [mcharbor cluster routing-check](mcharbor_cluster_routing-check.md)	 - Forward a real call to every agent another node holds and report each result
* [mcharbor cluster status](mcharbor_cluster_status.md)	 - Show this node, the database driver and which node leads each background job

