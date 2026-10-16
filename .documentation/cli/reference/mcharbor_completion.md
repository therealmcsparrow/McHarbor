## mcharbor completion

Generate the autocompletion script for the specified shell

### Synopsis

Generate the autocompletion script for mcharbor for the specified shell.
See each sub-command's help for details on how to use the generated script.


### Options

```
  -h, --help   help for completion
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
* [mcharbor completion bash](mcharbor_completion_bash.md)	 - Generate the autocompletion script for bash
* [mcharbor completion fish](mcharbor_completion_fish.md)	 - Generate the autocompletion script for fish
* [mcharbor completion powershell](mcharbor_completion_powershell.md)	 - Generate the autocompletion script for powershell
* [mcharbor completion zsh](mcharbor_completion_zsh.md)	 - Generate the autocompletion script for zsh

