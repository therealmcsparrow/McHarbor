## mcharbor audit

The audit log

```
mcharbor audit [flags]
```

### Options

```
  -h, --help   help for audit
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
* [mcharbor audit export](mcharbor_audit_export.md)	 - Download the signed audit export
* [mcharbor audit list](mcharbor_audit_list.md)	 - List or search audit entries, newest first
* [mcharbor audit verify](mcharbor_audit_verify.md)	 - Re-walk the hash chain: verified, verified across restore anchors, or broken

