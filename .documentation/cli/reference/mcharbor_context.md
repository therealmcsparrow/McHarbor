## mcharbor context

Manage the McHarbor servers this CLI knows

```
mcharbor context [flags]
```

### Options

```
  -h, --help   help for context
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
* [mcharbor context current](mcharbor_context_current.md)	 - Print the current context name
* [mcharbor context delete](mcharbor_context_delete.md)	 - Delete a context and its stored key
* [mcharbor context list](mcharbor_context_list.md)	 - List contexts; the current one is marked
* [mcharbor context rename](mcharbor_context_rename.md)	 - Rename a context and move its stored key
* [mcharbor context set](mcharbor_context_set.md)	 - Create or change a context from --url, --env, --ca-file and --insecure
* [mcharbor context use](mcharbor_context_use.md)	 - Make a context current

