# Synology DSM: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| Login fails with the right password | Two-factor is enabled on the DSM account | Use an application password, or a dedicated account without 2FA. |
| Snapshot creation is rejected | The shared folder is on an ext4 volume | Shared-folder snapshots need Btrfs. |
| TLS errors | The default Synology certificate is self-signed | Enable **Ignore certificate errors**, or install a real certificate on DSM. |

**Test** probes the driver and reports how many targets it can see. If a snapshot cannot
be taken when a backup actually runs, the container is paused instead and the reason is
recorded on the run, so the backup falls back to a correct archive rather than a broken one.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i snapshot-synology
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Synology DSM](./README.md): what it is and how it works
