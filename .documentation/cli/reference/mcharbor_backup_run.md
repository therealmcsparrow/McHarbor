## mcharbor backup run

Backup and restore runs

```
mcharbor backup run [flags]
```

### Options

```
  -h, --help   help for run
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

* [mcharbor backup](mcharbor_backup.md)	 - Container backup plans, runs and restores
* [mcharbor backup run cancel](mcharbor_backup_run_cancel.md)	 - Cancel a running backup
* [mcharbor backup run get](mcharbor_backup_run_get.md)	 - Show one run
* [mcharbor backup run list](mcharbor_backup_run_list.md)	 - List recent runs

