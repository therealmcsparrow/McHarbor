## mcharbor runbook automate

Run a runbook unattended as a service principal, on its alert and/or a schedule

### Synopsis

Assign a runbook to a service principal and choose when it runs unattended:
--on-alert (when its alert type fires), --cron (a five-field schedule), or both.
Every step is checked against the principal's own roles, and a run stops at the
first instruction step, which needs a person. --off turns unattended runs off.

Requires the runbooks.automate permission.

```
mcharbor runbook automate <runbook-id> [flags]
```

### Examples

```
  mcharbor runbook automate d9s7mb9tccrak46lqdsg --principal d9s7mb9tccrak46lqdsh --on-alert
  mcharbor runbook automate d9s7mb9tccrak46lqdsg --principal d9s7mb9tccrak46lqdsh --cron '0 3 * * *'
  mcharbor runbook automate d9s7mb9tccrak46lqdsg --off
```

### Options

```
      --cron string        five-field cron schedule, in the server's time zone
  -h, --help               help for automate
      --off                turn unattended runs off
      --on-alert           run when the runbook's alert type fires
      --principal string   service principal id to run as
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

* [mcharbor runbook](mcharbor_runbook.md)	 - Runbooks, and the record of who followed them

