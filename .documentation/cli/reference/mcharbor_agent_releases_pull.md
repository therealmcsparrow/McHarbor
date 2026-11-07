## mcharbor agent releases pull

Download and verify a release into McHarbor's store

### Synopsis

Starts the download and returns; the release reads "pulling" until it is
verified and stored or refused. A release whose signature or checksums do not
verify is never stored, and `list` shows why. A verified download becomes the
target; builds stored before it stay, so `target --build` can step back.
--build picks one of the builds `check` lists; one older than the newest is pinned.

```
mcharbor agent releases pull <version> [flags]
```

### Examples

```
  mcharbor agent releases pull 2026.09.02
  mcharbor agent releases pull 2026.10.01 --build 202610050818
```

### Options

```
      --build string   pull this build (yyyyMMddHHmm) instead of the newest; an older build is pinned until another is pulled
  -h, --help           help for pull
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

