# SharePoint Online: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| `accessDenied` on the library | Only `Files.ReadWrite` was consented, not `Sites.ReadWrite.All` | Add the site permission and re-consent, because the token is issued with the scopes consented at the time. |
| Uploads land in the wrong library | The site has several document libraries and the default is not the intended one | Resolve the specific drive with `GET /sites/{site_id}/drives` and set `drive_id`. |

Run **Test** on the destination after any change. It performs a full write, change and
delete round trip, so a green test also proves the credentials can clean up after
themselves, which is what retention needs.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i storage-sharepoint-online
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [SharePoint Online](./README.md): what it is and how it works
