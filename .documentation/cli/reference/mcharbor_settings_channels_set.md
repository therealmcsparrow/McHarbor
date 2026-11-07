## mcharbor settings channels set

Turn prerelease switches on or off; switches you do not name stay as they are

### Synopsis

Only the flags you pass are sent, so a script cannot overwrite a switch it never
mentions. Pass a value with =, for example --agent-prereleases=false.
Turning an automatic option on is disruptive and asks unless --yes. You cannot go back to an older version afterwards.

```
mcharbor settings channels set [flags]
```

### Examples

```
  mcharbor settings channels set --mcharbor-prereleases=true
  mcharbor channels set --agent-prereleases=false --agent-auto-prereleases=false --yes
```

### Options

```
      --agent-auto-prereleases      also update agents to prereleases automatically (needs agent-prereleases and agent auto-update)
      --agent-prereleases           offer agent prereleases as update targets
  -h, --help                        help for set
      --mcharbor-auto-prereleases   also install McHarbor prereleases automatically (needs mcharbor-prereleases and automatic updates)
      --mcharbor-prereleases        offer McHarbor prereleases as updates
      --plugin-prereleases          offer plugin prereleases for every plugin
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

* [mcharbor settings channels](mcharbor_settings_channels.md)	 - Prerelease switches for McHarbor, agents and plugins

