# Google Drive: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| Consent succeeds, uploads 404 on the folder | `folder_id` names a folder the app cannot see under `drive.file` | Leave `folder_id` blank, let the plugin create in the root, or grant the app broader scope deliberately. |
| `invalid_grant` after some weeks | The OAuth client is still in *Testing* publishing status, where refresh tokens expire in 7 days | Move the consent screen to *In production*. |

Run **Test** on the destination after any change. It performs a full write, change and
delete round trip, so a green test also proves the credentials can clean up after
themselves, which is what retention needs.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i storage-google-drive
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Google Drive](./README.md): what it is and how it works
