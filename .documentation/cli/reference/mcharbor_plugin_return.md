## mcharbor plugin return

Return a plugin from a prerelease to the newest stable build; its data is kept

### Synopsis

--check asks the server whether the return could happen now and changes nothing.
It does not unpack a package that has no signature; it reports that instead.
A return is irreversible and needs --confirm <slug>. A package no trusted key
verifies is refused unless you pass --allow-unsigned, which runs it as native
code with full host privileges. Needs plugins.manage.

```
mcharbor plugin return <slug> [flags]
```

### Examples

```
  mcharbor plugin return addon-kvm --check
  mcharbor plugin return addon-kvm --confirm addon-kvm
```

### Options

```
      --allow-unsigned   return even when no trusted key verifies the package (it runs with full host privileges)
      --check            only report whether the return could happen now; changes nothing
  -h, --help             help for return
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

