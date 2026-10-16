## mcharbor plugin dev package

Package and sign one plugin without touching the catalog index

### Synopsis

package drives scripts/development/build-plugin.sh for one plugin: the frontend bundle, the packager stage, the signature and the catalog entry, but not index.json. Use it while iterating on one plugin and plugin dev build when the catalog has to be consistent again.

```
mcharbor plugin dev package <slug> [flags]
```

### Options

```
      --catalog string   catalog URL baked into the entry's download_url (default file:///mnt/plugin-catalog)
  -h, --help             help for package
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

