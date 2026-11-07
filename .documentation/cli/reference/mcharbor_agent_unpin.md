## mcharbor agent unpin

Release an agent's pin so updates reach it again

### Synopsis

Removes the exclusion a return added. The agent stays on its current version
and updates reach it again. Needs updates.manage and environments.manage.

```
mcharbor agent unpin <environment-id> [flags]
```

### Examples

```
  mcharbor agent unpin 9m4e2mr0ui3e8a215n4g --confirm 9m4e2mr0ui3e8a215n4g
```

### Options

```
  -h, --help   help for unpin
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

* [mcharbor agent](mcharbor_agent.md)	 - Remote agents

