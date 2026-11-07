## mcharbor host updates

OS package updates of a host: check, apply, schedule

### Synopsis

Drives the package manager of an environment's host through the same routes as
the host's Updates tab. Name the host with --env, or pass --self for the machine
McHarbor itself runs on (a global grant of host_self.view or host_self.manage,
which an environment-scoped grant does not reach). apply is disruptive and asks
unless --yes.

```
mcharbor host updates [flags]
```

### Options

```
  -h, --help   help for updates
      --self   act on the machine McHarbor itself runs on instead of an environment
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

* [mcharbor host](mcharbor_host.md)	 - The host behind an environment: its systemd services, device nodes and OS updates
* [mcharbor host updates apply](mcharbor_host_updates_apply.md)	 - Start the package upgrade on the host (disruptive)
* [mcharbor host updates check](mcharbor_host_updates_check.md)	 - Run the package check now and list what can be upgraded
* [mcharbor host updates schedule](mcharbor_host_updates_schedule.md)	 - Change the automatic check schedule and the upgrade options
* [mcharbor host updates status](mcharbor_host_updates_status.md)	 - The last check's result and the stored schedule, without running a check

