## mcharbor host updates schedule

Change the automatic check schedule and the upgrade options

### Synopsis

Reads the stored schedule and writes it back with only the flags you pass changed,
so --time alone does not reset the days. --days takes a comma-separated list of
mon,tue,wed,thu,fri,sat,sun; --time is a 24-hour HH:MM on the server clock. The
option flags set the defaults an apply uses when it is not given its own.

```
mcharbor host updates schedule [flags]
```

### Examples

```
  mcharbor host updates schedule --env prod-01 --enable --time 03:30 --days sun
  mcharbor host updates schedule --self --disable
  mcharbor host updates schedule --env prod-01 --remove-unused=false
```

### Options

```
      --days string         comma-separated days: mon,tue,wed,thu,fri,sat,sun
      --disable             turn the automatic check off
      --enable              turn the automatic check on
  -h, --help                help for schedule
      --remove-unused       run autoremove after a successful upgrade (apt, dnf, yum, pacman)
      --skip-unavailable    carry on past a package its mirror no longer serves (apt, dnf, yum)
      --time string         24-hour HH:MM on the server clock
      --with-new-packages   let the package manager install new dependencies, which a kernel ABI bump needs
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
      --self               act on the machine McHarbor itself runs on instead of an environment
      --timeout duration   give up on a request after this long; 0 waits forever (default 1m0s)
      --url string         McHarbor URL, overriding the context ($MCHARBOR_URL)
      --yaml               shorthand for --output yaml
      --yes                skip the prompt for disruptive but reversible changes
```

### SEE ALSO

* [mcharbor host updates](mcharbor_host_updates.md)	 - OS package updates of a host: check, apply, schedule

