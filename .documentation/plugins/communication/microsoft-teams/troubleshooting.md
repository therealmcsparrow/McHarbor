# Microsoft Teams: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| Sends stop working on a tenant that used to work | Office 365 connectors were disabled or retired for the tenant | Move to a Power Automate workflow URL, or another channel type. |
| The card renders as plain text | A Teams client older than Adaptive Cards support | Update the client; the payload is a standard Adaptive Card. |

**Send test notification** delivers a real message. There is no dry run: if the test
succeeds, someone received it.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i comm-teams
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Microsoft Teams](./README.md): what it is and how it works
