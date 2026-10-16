## mcharbor login

Save a server and an API key as a context, after checking the key works

### Synopsis

login stores the server as a context and its API key in the OS keyring, or in an owner-only file where there is no keyring (a headless server is the normal case). Create the key under Security → API keys with only the permissions the CLI needs.

With --device there is no key to paste: login prints a short code, you approve it in the browser at /device on that server, choosing what the key may do, and the new key comes back to the terminal once.

In CI, skip login entirely and set MCHARBOR_URL and MCHARBOR_API_KEY.

```
mcharbor login <url> [flags]
```

### Options

```
      --api-key-stdin   read the API key from stdin instead of prompting
      --device          sign in by approving a code in the browser instead of pasting a key
  -h, --help            help for login
      --name string     context name; defaults to the server's host name
      --no-browser      with --device, only print the link instead of opening it
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

