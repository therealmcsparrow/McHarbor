# Gmail: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| `invalid_grant` after a week | The OAuth consent screen is still in *Testing* | Publish the consent screen; test-mode refresh tokens expire in 7 days. |
| `Delegation denied for ...` | `from_address` is not the authorising account or an alias of it | Send as the account itself, or add the alias in Gmail settings. |

**Send test notification** delivers a real message. There is no dry run: if the test
succeeds, someone received it.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i comm-gmail
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Gmail](./README.md): what it is and how it works
