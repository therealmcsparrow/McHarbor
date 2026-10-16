## mcharbor stack

Compose stacks in the selected environment

```
mcharbor stack [flags]
```

### Options

```
  -h, --help   help for stack
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
* [mcharbor stack compose](mcharbor_stack_compose.md)	 - Print a stack's compose file
* [mcharbor stack down](mcharbor_stack_down.md)	 - Stop and remove a stack's containers; the stack definition stays
* [mcharbor stack get](mcharbor_stack_get.md)	 - Show one stack
* [mcharbor stack list](mcharbor_stack_list.md)	 - List stacks
* [mcharbor stack remove](mcharbor_stack_remove.md)	 - Delete a stack from McHarbor (irreversible)
* [mcharbor stack restart](mcharbor_stack_restart.md)	 - Restart a stack's containers
* [mcharbor stack stop](mcharbor_stack_stop.md)	 - Stop a stack's containers, keeping them
* [mcharbor stack up](mcharbor_stack_up.md)	 - Deploy a stack: create and start its services

