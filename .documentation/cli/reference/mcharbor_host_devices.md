## mcharbor host devices

Device nodes on the host that a container can map

### Synopsis

Lists the character and block devices under /dev on the environment's host:
serial adapters, GPUs, audio, video, input, /dev/net/tun, /dev/fuse, /dev/kvm,
USB bus nodes and disks. STABLE is the by-id or by-path name that survives a
replug; use it with --device. Block devices are marked DANGEROUS. --env is required.

```
mcharbor host devices [flags]
```

### Examples

```
  mcharbor host devices --env prod-01
  mcharbor host devices --env prod-01 -o json
```

### Options

```
  -h, --help   help for devices
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

* [mcharbor host](mcharbor_host.md)	 - The host behind an environment: its systemd services, device nodes and OS updates

