## mcharbor container

Containers in the selected environment

```
mcharbor container [flags]
```

### Options

```
  -h, --help   help for container
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
* [mcharbor container exec](mcharbor_container_exec.md)	 - Run a command in a container, or open an interactive shell
* [mcharbor container get](mcharbor_container_get.md)	 - Show one container
* [mcharbor container kill](mcharbor_container_kill.md)	 - Send a signal to a container
* [mcharbor container list](mcharbor_container_list.md)	 - List containers
* [mcharbor container logs](mcharbor_container_logs.md)	 - Print, or with -f follow, a container's log
* [mcharbor container pause](mcharbor_container_pause.md)	 - Pause a container
* [mcharbor container remove](mcharbor_container_remove.md)	 - Remove a container (irreversible)
* [mcharbor container restart](mcharbor_container_restart.md)	 - Restart a container
* [mcharbor container start](mcharbor_container_start.md)	 - Start a container
* [mcharbor container stop](mcharbor_container_stop.md)	 - Stop a container
* [mcharbor container unpause](mcharbor_container_unpause.md)	 - Unpause a container

