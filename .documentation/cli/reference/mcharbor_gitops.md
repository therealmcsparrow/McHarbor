## mcharbor gitops

GitOps pipelines and promotions

```
mcharbor gitops [flags]
```

### Options

```
  -h, --help   help for gitops
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
* [mcharbor gitops approval](mcharbor_gitops_approval.md)	 - Promotion approvals
* [mcharbor gitops pipeline](mcharbor_gitops_pipeline.md)	 - Promotion pipelines
* [mcharbor gitops preview](mcharbor_gitops_preview.md)	 - Ephemeral pull-request preview environments
* [mcharbor gitops webhook-secret](mcharbor_gitops_webhook-secret.md)	 - The secret that authenticates a repository's pull-request webhook

