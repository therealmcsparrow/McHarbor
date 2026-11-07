## mcharbor settings channels

Prerelease switches for McHarbor, agents and plugins

### Synopsis

A prerelease is a test build that is not final. Every switch is off until an
operator turns it on, and a stable release is always offered either way.

```
mcharbor settings channels [flags]
```

### Options

```
  -h, --help   help for channels
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

* [mcharbor settings](mcharbor_settings.md)	 - Read and change application settings by key, and read the security posture
* [mcharbor settings channels set](mcharbor_settings_channels_set.md)	 - Turn prerelease switches on or off; switches you do not name stay as they are
* [mcharbor settings channels show](mcharbor_settings_channels_show.md)	 - Show which prerelease switches are on

