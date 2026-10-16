# OpenCloud: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| 401 on every call | The credential is a login password, not an app password, on an account with 2FA | Generate an app password under **Personal → Security**. |
| 404 on upload | The WebDAV prefix differs from the Nextcloud default | Set `apiPath` to whatever path answers a `PROPFIND` in a browser. |

Run **Test** on the destination after any change. It performs a full write, change and
delete round trip, so a green test also proves the credentials can clean up after
themselves, which is what retention needs.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i storage-opencloud
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [OpenCloud](./README.md): what it is and how it works
