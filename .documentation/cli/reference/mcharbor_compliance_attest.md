## mcharbor compliance attest

Record a sign-off on a pack run or on the access review

### Synopsis

Records, under the API key's user and the current time, that the evidence was
reviewed and accepted. There is no edit and no delete: a later attestation supersedes
an earlier one and both stay on record.

<framework> is a pack key such as cis-docker-benchmark, or access-review, soc2 or
iso-27001. --run names the pack run being signed off; it must belong to the same
framework and have finished. Leave it out for the access review.

Requires the compliance.attest permission.

```
mcharbor compliance attest <framework> [flags]
```

### Examples

```
  mcharbor compliance attest cis-docker-benchmark --run d1tvq4mj8k9s --note "Accepted; 2.13 tracked in CHG-42"
  mcharbor compliance attest access-review --note "Q3 review: two dormant accounts disabled"
```

### Options

```
  -h, --help           help for attest
      --note string    what was reviewed and accepted (required)
      --run string     the pack run being signed off
      --scope string   an environment ID or short label the sign-off covers
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

