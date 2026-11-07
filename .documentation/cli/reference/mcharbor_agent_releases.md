## mcharbor agent releases

Agent releases stored in McHarbor, which every install and update is served from

### Synopsis

McHarbor pulls agent releases from the McHarbor-Catalog repository, a mirror
or GitHub, verifies each against its signed SHA256SUMS, and serves agents only
from what it stored.

The target is the release installs, deploys and update sweeps hand out. With no
target, agents get the locally built agent, else the one the image shipped with.

```
mcharbor agent releases [flags]
```

### Options

```
  -h, --help   help for releases
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

* [mcharbor agent](mcharbor_agent.md)	 - Remote agents
* [mcharbor agent releases check](mcharbor_agent_releases_check.md)	 - Ask the configured sources which agent version is newest
* [mcharbor agent releases delete](mcharbor_agent_releases_delete.md)	 - Delete a stored release (irreversible)
* [mcharbor agent releases list](mcharbor_agent_releases_list.md)	 - Stored releases, the target and the last check
* [mcharbor agent releases pull](mcharbor_agent_releases_pull.md)	 - Download and verify a release into McHarbor's store
* [mcharbor agent releases target](mcharbor_agent_releases_target.md)	 - Make a stored release the one agents are served

