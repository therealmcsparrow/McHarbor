## mcharbor plugin dev sign

Sign the built tarballs in the local catalog directory

### Synopsis

sign runs the host's own signer over a directory of tarballs and catalog entries, so the signature the store verifies and the signature written here cannot differ. It prefers the prebuilt src/backend/pluginsigner.exe, falls back to go run ./cmd/pluginsigner, and only then to src/plugins/scripts/sign-plugin.py, which needs PyNaCl. The slug is accepted for symmetry with the other verbs and narrows nothing: the signer works on a directory.

```
mcharbor plugin dev sign [slug] [flags]
```

### Options

```
      --dir string     directory holding the tarballs and catalog entries (default <repo>/mnt/plugin-catalog)
  -h, --help           help for sign
      --key string     ed25519 private key (default <repo>/signing/plugin-signing.key)
      --trust string   ed25519 public key to verify each signature against
```

### Options inherited from parent commands

```
      --ca-file string     PEM root to trust for the server certificate ($MCHARBOR_CA_FILE)
      --confirm string     name the target of an irreversible change to confirm it non-interactively
      --context string     context to use instead of the current one ($MCHARBOR_CONTEXT)
      --dry-run            print the command that would run instead of running it
      --env string         environment id for scoped commands ($MCHARBOR_ENV)
      --insecure           skip certificate verification; for labs only ($MCHARBOR_INSECURE=true)
      --json               shorthand for --output json
  -o, --output string      output format: table, json, yaml or tsv (default "table")
      --repo string        McHarbor checkout to work in; found from the working directory when omitted ($MCHARBOR_REPO_ROOT)
      --timeout duration   give up on a request after this long; 0 waits forever (default 1m0s)
      --url string         McHarbor URL, overriding the context ($MCHARBOR_URL)
      --yaml               shorthand for --output yaml
      --yes                skip the prompt for disruptive but reversible changes
```

### SEE ALSO

* [mcharbor plugin dev](mcharbor_plugin_dev.md)	 - Author a plugin locally: scaffold, build, package, sign and validate

