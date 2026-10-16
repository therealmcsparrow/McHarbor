## mcharbor user

User accounts

```
mcharbor user [flags]
```

### Options

```
  -h, --help   help for user
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
* [mcharbor user assign-role](mcharbor_user_assign-role.md)	 - Give a user a role, globally or scoped to an environment or stack
* [mcharbor user create](mcharbor_user_create.md)	 - Create a local user; the password is read without echo, never from an argument
* [mcharbor user delete](mcharbor_user_delete.md)	 - Delete a user
* [mcharbor user disable](mcharbor_user_disable.md)	 - Disable a user; their sessions end and they cannot sign in
* [mcharbor user enable](mcharbor_user_enable.md)	 - Re-enable a disabled user
* [mcharbor user get](mcharbor_user_get.md)	 - Show a user
* [mcharbor user list](mcharbor_user_list.md)	 - List users
* [mcharbor user roles](mcharbor_user_roles.md)	 - List a user's role assignments
* [mcharbor user unassign-role](mcharbor_user_unassign-role.md)	 - Remove one role assignment (see user roles)
* [mcharbor user unlock](mcharbor_user_unlock.md)	 - Clear a lockout after failed sign-ins

