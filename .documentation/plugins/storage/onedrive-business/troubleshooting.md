# OneDrive for Business: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| `AADSTS65001` on consent | Admin consent has not been granted for the delegated permission | Grant admin consent in the Entra app registration, then retry consent. |
| `itemNotFound` on upload | `drive_id` belongs to another tenant, or `upn` is misspelled | Clear `drive_id`, confirm the UPN resolves in Graph Explorer, and retry. |
| Consent loops back to the login page | The redirect URI on the app registration does not match this host exactly | The URI must match scheme, host, port and path. |

Run **Test** on the destination after any change. It performs a full write, change and
delete round trip, so a green test also proves the credentials can clean up after
themselves, which is what retention needs.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i storage-onedrive-business
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [OneDrive for Business](./README.md): what it is and how it works
