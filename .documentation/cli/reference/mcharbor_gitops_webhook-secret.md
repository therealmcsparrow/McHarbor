## mcharbor gitops webhook-secret

The secret that authenticates a repository's pull-request webhook

```
mcharbor gitops webhook-secret [flags]
```

### Options

```
  -h, --help   help for webhook-secret
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

* [mcharbor gitops](mcharbor_gitops.md)	 - GitOps pipelines and promotions
* [mcharbor gitops webhook-secret remove](mcharbor_gitops_webhook-secret_remove.md)	 - Remove a repository's webhook secret; its deliveries are refused until a new one is set
* [mcharbor gitops webhook-secret rotate](mcharbor_gitops_webhook-secret_rotate.md)	 - Generate a new webhook secret and print it once
* [mcharbor gitops webhook-secret show](mcharbor_gitops_webhook-secret_show.md)	 - Show whether a repository has a webhook secret; the secret itself is never shown

