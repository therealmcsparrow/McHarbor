## mcharbor api-key create

Create an API key and print it once

### Synopsis

create prints the new key on stdout exactly once and never again; pipe it straight into a secret store rather than a terminal log. Grants can never exceed your own permissions.

```
mcharbor api-key create <name> [flags]
```

### Options

```
      --env-scope stringArray   limit the key to this environment id; repeat for more
      --expires-in duration     lifetime such as 720h; no expiry when omitted
  -h, --help                    help for create
      --role stringArray        role id whose permissions the key may use; repeat for more
      --scope stringArray       permission key the key may use; repeat for more
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

* [mcharbor api-key](mcharbor_api-key.md)	 - API keys

