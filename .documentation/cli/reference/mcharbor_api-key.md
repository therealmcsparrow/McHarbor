## mcharbor api-key

API keys

```
mcharbor api-key [flags]
```

### Options

```
  -h, --help   help for api-key
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
* [mcharbor api-key create](mcharbor_api-key_create.md)	 - Create an API key and print it once
* [mcharbor api-key get](mcharbor_api-key_get.md)	 - Show an API key's grants and usage
* [mcharbor api-key list](mcharbor_api-key_list.md)	 - List API keys; the key material is never returned
* [mcharbor api-key revoke](mcharbor_api-key_revoke.md)	 - Revoke an API key; anything using it stops working at once

