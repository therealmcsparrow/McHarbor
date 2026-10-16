# NFS: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| `operation not permitted` when mounting | The container cannot mount | The McHarbor container needs `SYS_ADMIN` (or equivalent) for kernel NFS mounts; a host-level mount passed in as a bind is the alternative. |
| Writes fail as `nobody` | UID squashing under `sec=sys` | Match the export's `anonuid`/`anongid`, or use a Kerberos flavour where identity is not UID-derived. |
| Kerberos mount fails with `KRB5KDC_ERR_S_PRINCIPAL_UNKNOWN` | The keytab does not hold `nfs/<host>` for the exact hostname used | Regenerate the keytab against the FQDN the client connects with. |

Run **Test** on the destination after any change. It performs a full write, change and
delete round trip, so a green test also proves the credentials can clean up after
themselves, which is what retention needs.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i storage-nfs
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [NFS](./README.md): what it is and how it works
