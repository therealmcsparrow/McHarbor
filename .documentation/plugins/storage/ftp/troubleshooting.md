# FTP/FTPS: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| Test hangs until the timeout | Implicit FTPS configured on port 21, or a blocked passive port range | Use 990 for implicit FTPS, and open the server's passive range towards the McHarbor host. |
| `x509: certificate signed by unknown authority` | Self-signed server certificate | Trust the CA in the host image, or (on a trusted network only) enable **Skip TLS Verification**. |
| Client-certificate auth is refused | The key does not match the certificate, or the file is unreadable inside the container | Check both paths resolve inside the container and that the key is the pair of the certificate. |

Run **Test** on the destination after any change. It performs a full write, change and
delete round trip, so a green test also proves the credentials can clean up after
themselves, which is what retention needs.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i storage-ftp
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [FTP/FTPS](./README.md): what it is and how it works
