## mcharbor identity provider

SSO providers (Entra ID, OIDC, Google, LDAP, SAML)

```
mcharbor identity provider [flags]
```

### Options

```
  -h, --help   help for provider
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

* [mcharbor identity](mcharbor_identity.md)	 - Identity providers
* [mcharbor identity provider delete](mcharbor_identity_provider_delete.md)	 - Delete a provider; its users can no longer sign in through it
* [mcharbor identity provider get](mcharbor_identity_provider_get.md)	 - Show a provider
* [mcharbor identity provider list](mcharbor_identity_provider_list.md)	 - List identity providers; secrets are never returned
* [mcharbor identity provider test](mcharbor_identity_provider_test.md)	 - Check the provider's configuration against the provider

