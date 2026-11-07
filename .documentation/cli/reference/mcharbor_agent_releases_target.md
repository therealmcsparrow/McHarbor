## mcharbor agent releases target

Make a stored release the one agents are served

### Synopsis

Installs, deploys and update sweeps hand out the target from then on. It does
not restart any agent by itself; run the update sweep for that. --clear goes back
to the locally built or image-bundled agent. Without --build the newest stored
build of the version is chosen.

```
mcharbor agent releases target [<version>] [flags]
```

### Examples

```
  mcharbor agent releases target 2026.09.02
  mcharbor agent releases target 2026.10.01 --build 202610050818
  mcharbor agent releases target --clear
```

### Options

```
      --build string   the stored build (yyyyMMddHHmm) to target instead of the newest
      --clear          clear the target and serve the built-in agent
  -h, --help           help for target
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

* [mcharbor agent releases](mcharbor_agent_releases.md)	 - Agent releases stored in McHarbor, which every install and update is served from

