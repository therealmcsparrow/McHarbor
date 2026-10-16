# Microsoft Entra ID: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| `AADSTS50011` redirect mismatch | The redirect URI on the app registration is not exactly this host | Scheme, host, port and path all have to match. |
| Login works, no roles are assigned | Group fetch is off, or the mapper matches names while Entra sends ids | Enable Graph scope, then map the object ids. |

Test a provider by signing in from a private browser window rather than by trusting the
form. A configuration that saves cleanly can still fail at the identity provider, and the
redirect is where that shows up.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i identity-entra-id
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Microsoft Entra ID](./README.md): what it is and how it works
