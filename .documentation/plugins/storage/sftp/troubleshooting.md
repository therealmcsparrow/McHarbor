# SFTP: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| `ssh: handshake failed: knownhosts` | The pinned fingerprint does not match the server | Re-read the fingerprint with `ssh-keyscan -t rsa host \| ssh-keygen -lf -`, and only update it if the change is expected. |
| `ssh: unable to authenticate` with a valid key | The key file is unreadable inside the container, or it is a PuTTY key | Mount it in and convert `.ppk` to OpenSSH format. |
| Agent auth does nothing | Windows named pipes are not supported | Run `ssh-agent` on a UNIX socket and mount the socket into the container. |

Run **Test** on the destination after any change. It performs a full write, change and
delete round trip, so a green test also proves the credentials can clean up after
themselves, which is what retention needs.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i storage-sftp
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [SFTP](./README.md): what it is and how it works
