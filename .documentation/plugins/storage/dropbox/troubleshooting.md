# Dropbox: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| `expired_access_token` in the backup log | The token was revoked or the app was switched to short-lived tokens | Generate a new token in the app console and re-save the destination. |
| Files land somewhere unexpected | The app is scoped to an app folder | Check the app's access type; for a scoped app, `base_path` starts inside `Apps/<app name>`. |

Run **Test** on the destination after any change. It performs a full write, change and
delete round trip, so a green test also proves the credentials can clean up after
themselves, which is what retention needs.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i storage-dropbox
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Dropbox](./README.md): what it is and how it works
