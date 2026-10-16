## mcharbor group

User groups

```
mcharbor group [flags]
```

### Options

```
  -h, --help   help for group
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
* [mcharbor group add-member](mcharbor_group_add-member.md)	 - Add a user to a group
* [mcharbor group create](mcharbor_group_create.md)	 - Create a group
* [mcharbor group delete](mcharbor_group_delete.md)	 - Delete a group
* [mcharbor group get](mcharbor_group_get.md)	 - Show a group
* [mcharbor group list](mcharbor_group_list.md)	 - List groups
* [mcharbor group remove-member](mcharbor_group_remove-member.md)	 - Remove a user from a group

