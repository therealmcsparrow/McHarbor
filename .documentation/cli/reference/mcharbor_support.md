## mcharbor support

Diagnostics, inventory, pre-upgrade checks and support bundles

```
mcharbor support [flags]
```

### Options

```
  -h, --help   help for support
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
* [mcharbor support bundle](mcharbor_support_bundle.md)	 - Support bundles
* [mcharbor support diagnostics](mcharbor_support_diagnostics.md)	 - Run the diagnostic checks and print each verdict
* [mcharbor support encryption-key](mcharbor_support_encryption-key.md)	 - Say where the data-encryption key came from
* [mcharbor support inventory](mcharbor_support_inventory.md)	 - Show what the instance keeps on disk and whether it is backed up
* [mcharbor support pre-upgrade](mcharbor_support_pre-upgrade.md)	 - Check whether this instance can be upgraded, exiting 7 when it cannot

