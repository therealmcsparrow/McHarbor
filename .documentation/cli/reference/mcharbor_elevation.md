## mcharbor elevation

Temporary permission elevation

```
mcharbor elevation [flags]
```

### Options

```
  -h, --help   help for elevation
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
* [mcharbor elevation approve](mcharbor_elevation_approve.md)	 - Approve a pending request
* [mcharbor elevation deny](mcharbor_elevation_deny.md)	 - Deny a pending request
* [mcharbor elevation get](mcharbor_elevation_get.md)	 - Show one request
* [mcharbor elevation list](mcharbor_elevation_list.md)	 - List your own requests, those waiting for you, or all
* [mcharbor elevation request](mcharbor_elevation_request.md)	 - Ask for a permission for a bounded time
* [mcharbor elevation withdraw](mcharbor_elevation_withdraw.md)	 - Withdraw your own pending request

