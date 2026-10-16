# Data retention: Reference

| Setting | Default | What it does |
|---|---|---|
| **Audit log retention (days)** | — | Audit entries. "Older entries are automatically purged" |
| **Activity log retention (days)** | — | Container activity events |
| **Lifecycle log retention** | — | Lifecycle events |
| **Backup log retention** | — | Backup-log rows: the per-phase record of what a backup did |
| **Security scan history retention** | — | Vulnerability scans *and their findings* |
| **Backup retention (days)** | — | Maximum age of successful backups per container |
| **Backup retention (count)** | — | Maximum number of successful backups per container |

Backup retention here is the app-wide default. Each backup plan can set its own, and the
plan wins.

Scan retention deletes the findings with the scan. A long scan history is how you show that
a vulnerability was introduced three months ago rather than yesterday, so this is usually
the wrong place to economise.

**Clear now** is explicit: "Permanently delete all audit log entries older than 90 days.
The deleted count will be shown after the operation. This cannot be undone." It is
unavailable while retention is 0 ("Set retention to a positive number to enable clearing"),
because keep-forever and purge-now are contradictory.

## When something does not behave

| Symptom | Usual cause | Fix |
|---|---|---|
| History is shorter than expected | A retention window is doing its job | Raise it, or forward the data out |
| **Clear now** is disabled | Retention is 0 (keep forever) | Set a positive number first |
| A purge removed more than expected | The window is measured in days from now | The deleted count is reported after the operation; there is no undo |
| Scan history disappeared | Scan retention purged the findings with the scans | Scan retention is usually the wrong place to economise |

## Related

- [Data retention](./README.md): what the section is for
- [Settings](../../README.md): the other groups
