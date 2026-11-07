## mcharbor agent releases delete

Delete a stored release (irreversible)

### Synopsis

The target and any build a connected agent runs are refused. A version with
more than one stored build needs --build to say which one goes.

```
mcharbor agent releases delete <version> [flags]
```

### Examples

```
  mcharbor agent releases delete 2026.09.01 --confirm 2026.09.01
  mcharbor agent releases delete 2026.10.01 --build 202610050818 --confirm 2026.10.01
```

### Options

```
      --build string   the stored build (yyyyMMddHHmm) to delete, when the version holds several
  -h, --help           help for delete
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

* [mcharbor agent releases](mcharbor_agent_releases.md)	 - Agent releases stored in McHarbor, which every install and update is served from

