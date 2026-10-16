## mcharbor volume

Volumes in the selected environment

```
mcharbor volume [flags]
```

### Options

```
  -h, --help   help for volume
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
* [mcharbor volume create](mcharbor_volume_create.md)	 - Create a volume
* [mcharbor volume get](mcharbor_volume_get.md)	 - Show one volume
* [mcharbor volume list](mcharbor_volume_list.md)	 - List volumes
* [mcharbor volume remove](mcharbor_volume_remove.md)	 - Remove a volume and its data (irreversible)

