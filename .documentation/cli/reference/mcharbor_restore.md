## mcharbor restore

Restore an instance export on this host, for an instance whose interface will not start

### Synopsis

restore --local runs the server's own restore against the data directory with McHarbor stopped. It is authorised by access to that directory and the export's key, and is recorded as the OS user and host. It refuses while McHarbor answers on this host unless --while-running names the instance. Run it where the server binary and data directory are, for example: docker run --rm -it -v mcharbor-data:/app/data <image> mcharbor restore --local /app/data/instance.mchx --passphrase-stdin

```
mcharbor restore --local <export.mchx> [flags]
```

### Examples

```
  mcharbor restore --local backup.mchx --key-file instance.key --plan
  mcharbor restore --local backup.mchx --passphrase-stdin --mode selective --categories access,settings
```

### Options

```
      --categories string        selective restore: comma-separated categories (access, environments, automation, stacks, secrets, settings, plugins, other)
      --conflict string          selective restore: replace (default) or keep existing rows
  -h, --help                     help for restore
      --include-audit            also restore audit history; the chain continues through a restore anchor
      --key-file string          the export's key file
      --local                    restore on this host, with McHarbor stopped
      --mode string              full, migration or selective (default "full")
      --passphrase-file string   file holding the export's passphrase
      --passphrase-stdin         read the passphrase from standard input
      --plan                     verify the export and print what would change, then stop
      --server-binary string     path to the McHarbor server binary (default "/app/mcharbor")
      --while-running string     restore even though McHarbor answers here; must name the instance id it prints
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

