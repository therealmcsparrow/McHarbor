## mcharbor plugin dev build

Build, sign and index a plugin into the local catalog

### Synopsis

build drives scripts/development/catalog.sh, which rebuilds the frontend UMD bundle, compiles the .so inside the host's Go module context through the packager stage, signs the tarball and regenerates index.json. With no slug it builds what the last sweep left stale.

Editing anything under src/backend/core/plugin/** invalidates every .so, which no stamp can tell you: use --all after that. A built plugin reaches a running instance only after docker compose up -d --force-recreate mcharbor.

```
mcharbor plugin dev build [slug...] [flags]
```

### Options

```
      --all              build every plugin in plugins.list; the only safe choice after editing core/plugin/**
      --catalog string   catalog URL baked into each entry's download_url
  -h, --help             help for build
      --keep-going       do not stop after the first failure
      --stale            build only what the last sweep left behind (the default)
```

### Options inherited from parent commands

```
      --ca-file string     PEM root to trust for the server certificate ($MCHARBOR_CA_FILE)
      --confirm string     name the target of an irreversible change to confirm it non-interactively
      --context string     context to use instead of the current one ($MCHARBOR_CONTEXT)
      --dry-run            print the command that would run instead of running it
      --env string         environment id for scoped commands ($MCHARBOR_ENV)
      --insecure           skip certificate verification; for labs only ($MCHARBOR_INSECURE=true)
      --json               shorthand for --output json
  -o, --output string      output format: table, json, yaml or tsv (default "table")
      --repo string        McHarbor checkout to work in; found from the working directory when omitted ($MCHARBOR_REPO_ROOT)
      --timeout duration   give up on a request after this long; 0 waits forever (default 1m0s)
      --url string         McHarbor URL, overriding the context ($MCHARBOR_URL)
      --yaml               shorthand for --output yaml
      --yes                skip the prompt for disruptive but reversible changes
```

### SEE ALSO

* [mcharbor plugin dev](mcharbor_plugin_dev.md)	 - Author a plugin locally: scaffold, build, package, sign and validate

