## mcharbor audit export

Download the signed audit export

```
mcharbor audit export [flags]
```

### Options

```
      --action stringArray   action such as container.remove; repeat for several
      --entity-id string     one entity's id
      --entity-type string   entity type such as container, stack, api_key
      --exact                match --search exactly instead of as a substring
      --format string        csv or json (default "json")
      --from string          only entries at or after this RFC 3339 time
  -h, --help                 help for export
  -f, --output-file string   file to write; - for stdout (default "-")
      --search string        text to find in the user, entity name or details
      --since string         only entries newer than this, such as 24h or 7d
      --to string            only entries before this RFC 3339 time
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

* [mcharbor audit](mcharbor_audit.md)	 - The audit log

