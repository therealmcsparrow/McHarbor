# TrueNAS: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| 401 from the middleware | The API key belongs to a user without snapshot rights | Use a key from an account that may manage snapshots. |
| Dataset not found | `dataset_path` was given as a mount path rather than a dataset name | Use `tank/backups`, not `/mnt/tank/backups`. |

**Test** probes the driver and reports how many targets it can see. If a snapshot cannot
be taken when a backup actually runs, the container is paused instead and the reason is
recorded on the run, so the backup falls back to a correct archive rather than a broken one.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i snapshot-truenas
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [TrueNAS](./README.md): what it is and how it works
