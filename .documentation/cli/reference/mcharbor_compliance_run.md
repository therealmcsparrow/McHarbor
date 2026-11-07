## mcharbor compliance run

Start a check run against a live host

### Synopsis

Reads the Docker daemon and every container on the environment and records a new
run. It changes nothing on the host, so it is Reversible rather than Disruptive —
but it is many API calls against a live daemon, so it prompts on nothing and is
safe in a cron entry.

The command returns as soon as the run is recorded; the sweep continues server-side.
Read the verdicts with `mcharbor compliance results <run-id>`.

Requires the compliance.manage permission.

```
mcharbor compliance run <framework> [flags]
```

### Examples

```
  mcharbor compliance run cis-docker-benchmark
  mcharbor compliance run cis-docker-benchmark --env prod -o json
```

### Options

```
  -h, --help   help for run
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

* [mcharbor compliance](mcharbor_compliance.md)	 - Evidence for a control framework: what was checked, what was not

