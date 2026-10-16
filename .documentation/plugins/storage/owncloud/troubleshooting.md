# ownCloud: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| 401 with the right password | Two-factor is on for the account | Use an app password from **Personal → Security**. |
| Works in a browser, 404 from McHarbor | `host` carries a trailing slash or a path | Give the bare origin, e.g. `https://cloud.example.com`. |

Run **Test** on the destination after any change. It performs a full write, change and
delete round trip, so a green test also proves the credentials can clean up after
themselves, which is what retention needs.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i storage-owncloud
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [ownCloud](./README.md): what it is and how it works
