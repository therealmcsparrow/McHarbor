# Nextcloud: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| 401 with a correct password | The account has two-factor authentication enabled | Two-factor blocks plain login over WebDAV; generate an app password instead. |
| 404 on every upload | `apiPath` or `username` does not match the real WebDAV root | Open `{host}/remote.php/dav/files/{username}/` in a browser and use the path that answers. |
| Large uploads fail halfway | A reverse proxy is capping the request body | Raise `client_max_body_size` (nginx) or the equivalent on whatever sits in front of Nextcloud. |

Run **Test** on the destination after any change. It performs a full write, change and
delete round trip, so a green test also proves the credentials can clean up after
themselves, which is what retention needs.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i storage-nextcloud
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Nextcloud](./README.md): what it is and how it works
