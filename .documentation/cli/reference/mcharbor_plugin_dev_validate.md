## mcharbor plugin dev validate

Check a plugin's manifest, its route declarations and its build inputs

### Synopsis

validate reads the source tree and reports what would go wrong once the plugin is installed. The check worth running it for is route drift: a plugin that registers a non-public route its plugin.yaml never declares gets no permission middleware at all, because the loader returns an empty permission on a miss and an empty permission is a pass-through — so the route is reachable by any authenticated user whatever their role. It also refuses a CRLF go.mod, a dependency version that disagrees with the host's, a permission key the installer will reject, a Version symbol that has drifted from the manifest, a UMD bundle older than its sources, and a contributes.capabilities entry the host would drop.

With no slug it validates every plugin in the checkout. It exits 1 when any check is an error and 0 when the worst is a warning.

```
mcharbor plugin dev validate [slug...] [flags]
```

### Options

```
  -h, --help   help for validate
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

