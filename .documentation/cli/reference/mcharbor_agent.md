## mcharbor agent

Remote agents

```
mcharbor agent [flags]
```

### Options

```
  -h, --help   help for agent
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
* [mcharbor agent install-prereqs](mcharbor_agent_install-prereqs.md)	 - Install the named prerequisites on a host over SSH
* [mcharbor agent list](mcharbor_agent_list.md)	 - List agents with their connection health
* [mcharbor agent probe](mcharbor_agent_probe.md)	 - Report what a host has for an agent over SSH, changing nothing
* [mcharbor agent releases](mcharbor_agent_releases.md)	 - Agent releases stored in McHarbor, which every install and update is served from
* [mcharbor agent return](mcharbor_agent_return.md)	 - Return an agent from a prerelease to stable and pin it
* [mcharbor agent unpin](mcharbor_agent_unpin.md)	 - Release an agent's pin so updates reach it again

