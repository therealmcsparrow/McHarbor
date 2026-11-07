## mcharbor runbook

Runbooks, and the record of who followed them

### Synopsis

Read the runbooks attached to an alert type, a container, a stack or an
environment, and the runs recorded against them.

There is deliberately no `runbook run`: a person's step is authorised against
the operator pressing it. Unattended runs act as a service principal and are
started by the server on the runbook's alert or schedule; configure them with
`runbook automate` (runbooks.automate).

Reading requires the runbooks.view permission.

```
mcharbor runbook [flags]
```

### Options

```
  -h, --help   help for runbook
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
* [mcharbor runbook automate](mcharbor_runbook_automate.md)	 - Run a runbook unattended as a service principal, on its alert and/or a schedule
* [mcharbor runbook catalog](mcharbor_runbook_catalog.md)	 - List the actions and diagnostics a step may name, and the key each needs
* [mcharbor runbook get](mcharbor_runbook_get.md)	 - Show a runbook and its ordered steps
* [mcharbor runbook list](mcharbor_runbook_list.md)	 - List runbooks
* [mcharbor runbook principals](mcharbor_runbook_principals.md)	 - List the service principals a runbook can run as
* [mcharbor runbook runs](mcharbor_runbook_runs.md)	 - List runs: who responded, how it ended, where it stopped

