# Generic OpenID Connect: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| Discovery fails | The issuer URL has a trailing slash or a path the IdP does not serve discovery on | Use the exact `issuer` value from the IdP's own discovery document. |
| No groups in McHarbor | The IdP is not emitting a groups claim | Add the claim at the IdP; this adapter cannot call a directory API to fetch them. |

Test a provider by signing in from a private browser window rather than by trusting the
form. A configuration that saves cleanly can still fail at the identity provider, and the
redirect is where that shows up.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i identity-generic-oidc
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Generic OpenID Connect](./README.md): what it is and how it works
