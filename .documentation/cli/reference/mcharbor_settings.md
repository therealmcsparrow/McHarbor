## mcharbor settings

Read and change application settings by key, and read the security posture

### Synopsis

Reads and writes one row of the settings table through the same routes the interface uses.

The server validates every value it knows the shape of — security.mfa_required accepts
off, admins or all and refuses anything else with err.settings.mfa_requirement_invalid —
so the CLI adds no validation of its own. Keys that hold key material are not readable.

Requires settings.view to read and settings.manage to change.

```
mcharbor settings [flags]
```

### Options

```
  -h, --help   help for settings
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

* [mcharbor](mcharbor.md)	 - Operate McHarbor from a terminal or a CI job
* [mcharbor settings channels](mcharbor_settings_channels.md)	 - Prerelease switches for McHarbor, agents and plugins
* [mcharbor settings get](mcharbor_settings_get.md)	 - Show one setting
* [mcharbor settings posture](mcharbor_settings_posture.md)	 - List the security posture findings the server logged at boot
* [mcharbor settings set](mcharbor_settings_set.md)	 - Change one setting; a security.* key asks for confirmation

