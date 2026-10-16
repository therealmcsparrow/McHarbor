## mcharbor env

Environments: the Docker hosts McHarbor manages

```
mcharbor env [flags]
```

### Options

```
  -h, --help   help for env
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
* [mcharbor env delete](mcharbor_env_delete.md)	 - Delete an environment from McHarbor (irreversible)
* [mcharbor env get](mcharbor_env_get.md)	 - Show one environment
* [mcharbor env list](mcharbor_env_list.md)	 - List environments
* [mcharbor env test](mcharbor_env_test.md)	 - Test the connection to an environment
* [mcharbor env use](mcharbor_env_use.md)	 - Make an environment the default for the current context

