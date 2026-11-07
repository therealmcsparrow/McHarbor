## mcharbor compliance

Evidence for a control framework: what was checked, what was not

### Synopsis

Compliance packs map a named control framework to the evidence McHarbor produces
and the checks it can run against a live host.

A pack is evidence for a framework, not a claim of compliance with one. A check that
could not run reports `unknown` with a reason rather than passing, and controls
McHarbor cannot see are listed as not covered rather than omitted.

Reads require compliance.view; `run` requires compliance.manage; `attest` requires
compliance.attest, which is deliberately separate so starting a check and signing off
on its result are two authorities.

```
mcharbor compliance [flags]
```

### Options

```
  -h, --help   help for compliance
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
* [mcharbor compliance attest](mcharbor_compliance_attest.md)	 - Record a sign-off on a pack run or on the access review
* [mcharbor compliance attestations](mcharbor_compliance_attestations.md)	 - Recorded sign-offs, newest first
* [mcharbor compliance packs](mcharbor_compliance_packs.md)	 - Every pack, its coverage and its most recent run
* [mcharbor compliance results](mcharbor_compliance_results.md)	 - The control verdicts one run recorded
* [mcharbor compliance run](mcharbor_compliance_run.md)	 - Start a check run against a live host
* [mcharbor compliance runs](mcharbor_compliance_runs.md)	 - Recorded runs for a framework, newest first

