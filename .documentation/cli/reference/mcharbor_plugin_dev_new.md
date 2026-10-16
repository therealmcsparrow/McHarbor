## mcharbor plugin dev new

Scaffold a new plugin from the template built into this binary

### Synopsis

new writes a minimal working plugin into src/plugins/Plugins/<slug> and rewrites the slug in every file that names it: the manifest, the Go module path, the exported Version symbol, the frontend bundle name and the npm package name. The template is embedded in this binary, so it cannot drift out of the checkout.

Start a plugin this way rather than by copying a sibling. A sibling carries its vendor quirks, its bugs and whatever pattern was current when it was written, which is how one client implementation reached fourteen plugins.

It refuses to overwrite an existing plugin. The slug must start with one of the families the packager accepts: addon-, ai-provider-, comm-, identity-, logging-, scanner-, snapshot-, storage-.

```
mcharbor plugin dev new <slug> [flags]
```

### Options

```
  -h, --help   help for new
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

