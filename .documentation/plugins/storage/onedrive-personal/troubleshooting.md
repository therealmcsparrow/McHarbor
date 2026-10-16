# OneDrive (Personal): Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| Consent rejects a work account | The `consumers` endpoint only accepts personal accounts | Use [OneDrive for Business](../onedrive-business/README.md) for a work or school account. |
| `unauthorized_client` with a custom `client_id` | The registration is single-tenant, or has no consumer support | Set the registration to *Personal Microsoft accounts* (or both), or clear `client_id` to use the built-in app. |

Run **Test** on the destination after any change. It performs a full write, change and
delete round trip, so a green test also proves the credentials can clean up after
themselves, which is what retention needs.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i storage-onedrive-personal
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [OneDrive (Personal)](./README.md): what it is and how it works
