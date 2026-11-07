## mcharbor settings set

Change one setting; a security.* key asks for confirmation

### Synopsis

Writes one setting. The row's existing category is kept unless --category is given,
because the server stores whatever category the request carries and several readers
select their settings by category.

Changing a security.* key is Disruptive: it prompts on a terminal and --yes skips the prompt.

```
mcharbor settings set <key> <value> [flags]
```

### Examples

```
  mcharbor settings set security.mfa_required admins --yes
  mcharbor settings set security.lockout_ip_threshold 20 --yes
```

### Options

```
      --category string   category to store the setting under; defaults to the existing row's
  -h, --help              help for set
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

* [mcharbor settings](mcharbor_settings.md)	 - Read and change application settings by key, and read the security posture

