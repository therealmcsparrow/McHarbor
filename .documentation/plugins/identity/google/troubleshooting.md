# Google: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| Anyone with a Gmail address can log in | **Workspace Domain** is blank | Set it; `hd=` is what restricts the tenant. |
| Group fetch returns nothing | Domain-wide delegation was not authorised for the scope | Authorise the service account's client id for `admin.directory.group.readonly` in the Workspace admin console. |

Test a provider by signing in from a private browser window rather than by trusting the
form. A configuration that saves cleanly can still fail at the identity provider, and the
redirect is where that shows up.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i identity-google
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Google](./README.md): what it is and how it works
