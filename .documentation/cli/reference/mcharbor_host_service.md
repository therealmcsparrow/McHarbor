## mcharbor host service

Systemd services on a native agent host

### Synopsis

Lists and drives the .service units on the host of a native agent environment.
--env is required. stop and disable are refused on mcharbor-agent*, ssh/sshd,
systemd-*, dbus*, docker and containerd, by the server and again by the agent.

```
mcharbor host service [flags]
```

### Options

```
  -h, --help   help for service
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
* [mcharbor host service cat](mcharbor_host_service_cat.md)	 - Print the effective unit (systemctl cat) and its McHarbor override
* [mcharbor host service daemon-reload](mcharbor_host_service_daemon-reload.md)	 - Run systemctl daemon-reload on the host
* [mcharbor host service disable](mcharbor_host_service_disable.md)	 - Disable a unit at boot
* [mcharbor host service edit](mcharbor_host_service_edit.md)	 - Replace the unit's drop-in override with a file, like systemctl edit
* [mcharbor host service enable](mcharbor_host_service_enable.md)	 - Enable a unit at boot
* [mcharbor host service list](mcharbor_host_service_list.md)	 - Every service unit with its state
* [mcharbor host service restart](mcharbor_host_service_restart.md)	 - Restart a unit; restarting the agent's own unit drops its connection until it reconnects
* [mcharbor host service revert](mcharbor_host_service_revert.md)	 - Delete the unit's McHarbor override and reload systemd (irreversible)
* [mcharbor host service start](mcharbor_host_service_start.md)	 - Start a unit
* [mcharbor host service stop](mcharbor_host_service_stop.md)	 - Stop a unit

