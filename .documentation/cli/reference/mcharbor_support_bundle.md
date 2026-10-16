## mcharbor support bundle

Support bundles

```
mcharbor support bundle [flags]
```

### Options

```
  -h, --help   help for bundle
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

* [mcharbor support](mcharbor_support.md)	 - Diagnostics, inventory, pre-upgrade checks and support bundles
* [mcharbor support bundle create](mcharbor_support_bundle_create.md)	 - Write a support bundle and follow it to completion
* [mcharbor support bundle delete](mcharbor_support_bundle_delete.md)	 - Delete a bundle and its archive (irreversible)
* [mcharbor support bundle download](mcharbor_support_bundle_download.md)	 - Download a bundle archive to a file
* [mcharbor support bundle get](mcharbor_support_bundle_get.md)	 - Show one bundle
* [mcharbor support bundle list](mcharbor_support_bundle_list.md)	 - List the bundles the server is keeping
* [mcharbor support bundle preview](mcharbor_support_bundle_preview.md)	 - Show what a bundle would contain and how large it would be, before writing one

