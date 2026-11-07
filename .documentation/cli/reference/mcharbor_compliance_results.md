## mcharbor compliance results

The control verdicts one run recorded

### Synopsis

One row per control, including the controls McHarbor cannot check — those carry
the reason code notCoveredByMcHarbor rather than being left out.

```
mcharbor compliance results <run-id> [flags]
```

### Examples

```
  mcharbor compliance results d1tvq4mj8k9s -o json
```

### Options

```
  -h, --help   help for results
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

* [mcharbor compliance](mcharbor_compliance.md)	 - Evidence for a control framework: what was checked, what was not

