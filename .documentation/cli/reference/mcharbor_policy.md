## mcharbor policy

Attribute-based access policies

```
mcharbor policy [flags]
```

### Options

```
  -h, --help   help for policy
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
* [mcharbor policy delete](mcharbor_policy_delete.md)	 - Delete a policy
* [mcharbor policy disable](mcharbor_policy_disable.md)	 - Turn a policy off
* [mcharbor policy enable](mcharbor_policy_enable.md)	 - Turn a policy on
* [mcharbor policy get](mcharbor_policy_get.md)	 - Show a policy
* [mcharbor policy list](mcharbor_policy_list.md)	 - List policies
* [mcharbor policy test](mcharbor_policy_test.md)	 - Evaluate a policy against sample attributes without changing anything

