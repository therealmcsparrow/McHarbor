# Storage: Reference

| Setting | Default | What it does |
|---|---|---|
| **Name** | — | Your label, such as "Nightly backups" |
| **Storage type** | — | One tile per installed storage plugin; local paths need no plugin |
| **Enabled** | On | Whether plans may write to it |
| **Per-destination fields** | — | Declared by the plugin; see the plugin’s own configuration page |
| **Backup encryption key** | none | Generate, install an existing key, or regenerate. Shown once and never stored |
| **Compose project path** | inferred | Under **Advanced** on install, only needed when McHarbor cannot infer where the Compose project lives |
| **Capacity bar** | — | Read-only. Used of total space per enabled destination; amber past 75%, red past 90% |

**Test** performs a real write, change and delete round trip at the destination. A green
test therefore also proves the credentials can clean up after themselves, which is what
retention needs.

Deleting a location "will permanently delete this storage location configuration"; backup
plans pointing at it keep the reference and start failing.

## When something does not behave

| Symptom | Usual cause | Fix |
|---|---|---|
| Backups fail with an encryption error | No backup key installed | Generate or install one; encrypted backups will not run without it |
| An old archive will not restore | It was written under a different key | Supply the original secret when restore asks for it |
| Test fails on delete but passes on upload | The credential can write but not remove | Retention needs delete; fix the permission at the provider |
| An encrypted upload ends as failed although the destination looked healthy | The destination stopped reading part-way through the archive, so the stream was abandoned rather than left waiting | Read the error on the run, fix the destination, and run the plan again; a short upload is never recorded as a success |
| A local destination's test fails with `read-only file system` on a native install made with `--no-run-as-root` | The systemd unit's `ProtectSystem=strict` leaves only `/var/lib/mcharbor` writable to McHarbor | Add the path with a `ReadWritePaths=` drop-in and give the `mcharbor` user write access: [Native Linux install → Troubleshooting](../../../getting-started/installation/native-linux.md#8-troubleshooting), item 3 |
| A destination disappeared from a plan | It was deleted here | Recreate it, or repoint the plan |
| A destination shows no capacity bar | Its provider reports no quota, or the protocol has none (FTP) | Nothing to fix: capacity is unknown, not zero |
| Capacity says it could not be read | The credential works for writes but the quota call failed | Check the provider account's permissions; the destination still works for backups |

## Related

- [Storage](./README.md): what the section is for
- [Settings](../../README.md): the other groups
