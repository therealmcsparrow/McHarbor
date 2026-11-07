## mcharbor network create

Create a network

### Synopsis

Creates a network. --gateway and --ip-range apply to the first --subnet.
--host-link-name also creates a host interface on a macvlan network's parent so the host can reach the containers on it;
give it an address with --host-link-ipv4 and/or --host-link-ipv6 (a CIDR inside a --subnet, or "auto" for SLAAC with --ipv6).

```
mcharbor network create <name> [flags]
```

### Options

```
      --attachable              allow standalone containers to attach
      --driver string           network driver; bridge when empty
      --gateway string          gateway of the first subnet
  -h, --help                    help for create
      --host-link-ipv4 string   IPv4 address of the host interface, in CIDR notation
      --host-link-ipv6 string   IPv6 address of the host interface in CIDR notation, or "auto" for SLAAC
      --host-link-name string   create a host interface with this name on the macvlan parent
      --internal                no external connectivity
      --ip-range string         address range of the first subnet, in CIDR notation
      --ipv6                    enable IPv6 on the network
      --label stringArray       label as key=value; repeatable
      --opt stringArray         driver option as key=value, such as parent=eth0; repeatable
      --subnet stringArray      IPAM subnet in CIDR notation; repeatable
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

* [mcharbor network](mcharbor_network.md)	 - Networks in the selected environment

