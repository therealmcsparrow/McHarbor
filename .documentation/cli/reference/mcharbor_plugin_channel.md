## mcharbor plugin channel

Offer prereleases for one plugin, or follow the Plugin Store switch

### Synopsis

on and off set this plugin's own override; inherit clears it. The override only
adds: while the all-plugins switch is on, every plugin is offered prereleases
whatever its own setting says. Needs plugins.manage.

```
mcharbor plugin channel <slug> <on|off|inherit> [flags]
```

### Examples

```
  mcharbor plugin channel addon-kvm on
  mcharbor plugin channel addon-kvm inherit
```

### Options

```
  -h, --help   help for channel
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

