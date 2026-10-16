# Snapshots: Reference

| Setting | Default | What it does |
|---|---|---|
| **Name** | — | Your label, such as "Nightly volume snapshots" |
| **Driver** | — | One of the detected or plugin-provided drivers |
| **Driver settings** | — | Whatever that driver declares. A field the driver marks secret is encrypted before it is stored and is never returned again; when you edit a configuration the field reads "Stored - leave empty to keep", and leaving it empty keeps the stored value |
| **Enabled** | On | Whether backups may use it |
| **Raw configuration (JSON)** | `{}` | Under **Advanced**, for options the form does not model |

**Test** probes the driver and reports either "Snapshot driver reachable (4 targets)" or
the failure, and the configuration remembers when it was last checked, or "never checked".

Deleting one is not silent about the consequence: "*Nightly volume snapshots* is removed.
**Backups using it fall back to copying the live path.**"

## When something does not behave

| Symptom | Usual cause | Fix |
|---|---|---|
| Snapshot cannot be selected on a backup plan | No enabled configuration on that environment | Create one here first |
| "No snapshot drivers detected" | The filesystem offers none (ext4, for example) | Use a snapshot-capable filesystem, or a NAS driver plugin |
| Backups silently pause the container | The snapshot failed at run time | The reason is recorded on the run; that fallback is intended |
| A driver shows Unavailable | Missing tool or permission on the host | The detection panel names what it looked for; **Rescan** after fixing it |
| Saving is refused with "The plugin is not installed" (`err.plugin.not_installed`) | The driver comes from a plugin that is not installed or not loaded, so McHarbor cannot tell which of its fields are secret | Install or enable the plugin, then save again |

## Related

- [Snapshots](./README.md): what the section is for
- [Settings](../../README.md): the other groups
