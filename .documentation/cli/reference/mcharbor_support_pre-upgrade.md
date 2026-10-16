## mcharbor support pre-upgrade

Check whether this instance can be upgraded, exiting 7 when it cannot

### Synopsis

pre-upgrade runs the upgrade gate and prints every check. It exits 7 when a check fails, so a CI job can gate a rollout on it; a warning still exits 0. It needs updates.view.

```
mcharbor support pre-upgrade [flags]
```

### Options

```
  -h, --help            help for pre-upgrade
      --target string   version to upgrade to; empty checks the running version
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

