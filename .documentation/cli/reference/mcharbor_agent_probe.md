## mcharbor agent probe

Report what a host has for an agent over SSH, changing nothing

### Synopsis

Connects over SSH and lists the prerequisites (Docker, Compose, KVM, QEMU...) the
host has and lacks. Nothing is installed, stored or deployed.

```
mcharbor agent probe <host> [flags]
```

### Examples

```
  mcharbor agent probe 10.0.0.5 --user root --fingerprint SHA256:abc --key-file ~/.ssh/id_ed25519
  mcharbor agent probe lab1 --user ops --fingerprint SHA256:abc --password-stdin --scope vm < pw.txt
```

### Options

```
      --fingerprint string    SSH host key fingerprint to trust (required)
  -h, --help                  help for probe
      --key-file string       private key file to authenticate with
      --method string         deploy method the probe judges the host for: docker or binary (default "docker")
      --password-stdin        read the SSH password from the first line of stdin
      --port int              SSH port (default 22)
      --root-password-stdin   read the root password (su escalation) from the first line of stdin
      --scope string          narrow the report to one question: docker or vm (default: everything)
      --secret-id string      id of a vault secret holding the key or password
      --user string           SSH user (required)
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

* [mcharbor agent](mcharbor_agent.md)	 - Remote agents

