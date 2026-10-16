## mcharbor scim

SCIM provisioning connections

```
mcharbor scim [flags]
```

### Options

```
  -h, --help   help for scim
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
* [mcharbor scim delete](mcharbor_scim_delete.md)	 - Delete a connection
* [mcharbor scim get](mcharbor_scim_get.md)	 - Show a connection
* [mcharbor scim list](mcharbor_scim_list.md)	 - List SCIM connections; tokens are never returned
* [mcharbor scim sync](mcharbor_scim_sync.md)	 - Synchronise now instead of waiting for the schedule

