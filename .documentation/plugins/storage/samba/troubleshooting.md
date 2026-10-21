# Samba: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| `STATUS_LOGON_FAILURE` | NTLMv1 is disabled on the server, or the domain field is empty for a domain account | Use `domain` auth and fill in the NetBIOS domain name. |
| `STATUS_BAD_NETWORK_NAME` | The share name is wrong or case-mismatched | Copy the share name exactly as the server publishes it. |
| Connection refused | SMB1-only server, or 445 blocked | The client speaks SMB2/3 only; enable SMB2 on the server. |
| "storage-samba: the SMB server address is not an allowed destination" | The server is, or resolves to, a link-local, metadata, multicast or unspecified address | Use the server's routable address. A NAS on your LAN is allowed. |

Run **Test** on the destination after any change. It performs a full write, change and
delete round trip, so a green test also proves the credentials can clean up after
themselves, which is what retention needs.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i storage-samba
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Samba](./README.md): what it is and how it works
