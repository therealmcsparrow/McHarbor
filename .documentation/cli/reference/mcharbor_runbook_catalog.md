## mcharbor runbook catalog

List the actions and diagnostics a step may name, and the key each needs

### Synopsis

List the allow-list a runbook step is written from: every host action and
diagnostic, and those of the plugins that are loaded right now (virtual
machines, system containers, compute, swarm, Kubernetes). PLUGIN is empty for
a host entry. NEEDS is the permission checked, per step, against whoever runs
it — a person, or the service principal of an unattended run.

Reading requires the runbooks.view permission.

```
mcharbor runbook catalog [flags]
```

### Examples

```
  mcharbor runbook catalog
  mcharbor runbook catalog -o json
```

### Options

```
  -h, --help   help for catalog
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

* [mcharbor runbook](mcharbor_runbook.md)	 - Runbooks, and the record of who followed them

