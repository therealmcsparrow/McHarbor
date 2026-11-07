## mcharbor stack env

Show a stack's environment variables, or change them with set and unset

### Synopsis

Show the environment variables stored for a stack. Values whose names look like credentials are
shown as ••••••••; --reveal asks the server for the clear values, which needs
stacks.manage on the stack and is recorded in the audit log. Values carrying McHarbor's own keys
stay masked even then.

```
mcharbor stack env <name> [flags]
```

### Options

```
  -h, --help     help for env
      --reveal   show credential-looking values in the clear (needs stacks.manage on the stack; audit-logged)
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

* [mcharbor stack](mcharbor_stack.md)	 - Compose stacks in the selected environment
* [mcharbor stack env set](mcharbor_stack_env_set.md)	 - Set one or more environment variables on a stack
* [mcharbor stack env unset](mcharbor_stack_env_unset.md)	 - Remove one or more environment variables from a stack

