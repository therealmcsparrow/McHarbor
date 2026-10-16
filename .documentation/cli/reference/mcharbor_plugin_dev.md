## mcharbor plugin dev

Author a plugin locally: scaffold, build, package, sign and validate

### Synopsis

plugin dev is the plugin author's local toolchain. Unlike every other mcharbor command it talks to no server: it reads and writes your checkout and drives the build scripts and the signer. There is no API key, no context and no permission key involved, and nothing it does is audited, because nothing it does reaches a McHarbor instance.

The sibling commands — plugin list, catalog, install, update, uninstall — are the runtime half, and those do talk to a server and do need plugins.view / plugins.manage.

```
mcharbor plugin dev [flags]
```

### Options

```
      --dry-run       print the command that would run instead of running it
  -h, --help          help for dev
      --repo string   McHarbor checkout to work in; found from the working directory when omitted ($MCHARBOR_REPO_ROOT)
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

* [mcharbor plugin](mcharbor_plugin.md)	 - Installed plugins and the plugin store
* [mcharbor plugin dev build](mcharbor_plugin_dev_build.md)	 - Build, sign and index a plugin into the local catalog
* [mcharbor plugin dev new](mcharbor_plugin_dev_new.md)	 - Scaffold a new plugin from the example skeleton
* [mcharbor plugin dev package](mcharbor_plugin_dev_package.md)	 - Package and sign one plugin without touching the catalog index
* [mcharbor plugin dev sign](mcharbor_plugin_dev_sign.md)	 - Sign the built tarballs in the local catalog directory
* [mcharbor plugin dev validate](mcharbor_plugin_dev_validate.md)	 - Check a plugin's manifest, its route declarations and its build inputs

