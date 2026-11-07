## mcharbor stack env set

Set one or more environment variables on a stack

### Synopsis

Set variables on a stack. The stack's variables are read, changed and written back as a whole,
so two people changing the same stack at once can overwrite each other. The change is stored;
a running stack picks it up the next time it is deployed (mcharbor stack up). Needs stacks.manage.

```
mcharbor stack env set <name> KEY=VALUE... [flags]
```

### Options

```
  -h, --help   help for set
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

* [mcharbor stack env](mcharbor_stack_env.md)	 - Show a stack's environment variables, or change them with set and unset

