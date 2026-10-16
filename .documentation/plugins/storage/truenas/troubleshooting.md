# TrueNAS: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| 401 despite correct credentials | The WebDAV service is set to a different authentication mode | Set the WebDAV share to Basic authentication in the TrueNAS UI. |
| TLS errors against the default certificate | TrueNAS ships a self-signed certificate | Install a trusted certificate on TrueNAS, or terminate TLS at a proxy that has one. |

Run **Test** on the destination after any change. It performs a full write, change and
delete round trip, so a green test also proves the credentials can clean up after
themselves, which is what retention needs.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i storage-truenas
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [TrueNAS](./README.md): what it is and how it works
