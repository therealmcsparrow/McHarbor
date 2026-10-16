## mcharbor policy test

Evaluate a policy against sample attributes without changing anything

### Synopsis

test is read-only, which makes it safe in CI: keep the attributes of the cases a policy must allow or deny in files and fail the job when the answer changes.

```
mcharbor policy test <id> [flags]
```

### Options

```
      --attributes-file string   JSON with user, env, resource and now attributes
  -h, --help                     help for test
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

* [mcharbor policy](mcharbor_policy.md)	 - Attribute-based access policies

