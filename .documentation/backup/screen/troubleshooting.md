# Backups: Troubleshooting

What goes wrong with backups and restores. The two that cost the most are consistency mode
(Live is not safe for anything with a write-ahead log) and a key rotation that leaves
old archives encrypted under the old secret.

| Symptom | Usual cause | Fix |
|---|---|---|
| A restored database is corrupt | The backup ran in Live mode | Use Pause or Snapshot for anything with a write-ahead log |
| Snapshot cannot be selected | No enabled snapshot configuration on that environment | Configure one under Settings → Integrations → Snapshots |
| Saving a snapshot configuration says "The plugin is not installed" | The driver (TrueNAS, Synology) comes from a plugin that is not installed or not loaded | Install or enable the plugin, then save again |
| A plan reports failure with no obvious reason | The failure is in a phase, not the run row | Open the info button beside the failed status: it names the phase and carries the underlying error. Signed storage URLs and embedded credentials are redacted out of it. The backup log tab has the same run phase by phase |
| An encrypted upload to a destination fails, where it used to sit at 100% and never finish | The destination stopped reading before the whole archive was sent, for example a full quota or a dropped connection | The run now ends as failed, with the destination's own error, instead of waiting forever. Fix the destination, run **Test**, and run the plan again |
| Restore asks for a secret key | The archive was encrypted under a different backup key | Supply the original key; rotating the key does not re-encrypt old archives |
| Archives are not being pruned at the destination | Retention is zero, or the destination cannot delete | Check both retention values and run **Test** on the destination |
| A container's backups and restores stop linking to it after it was recreated | Its environment does not track container events, so McHarbor did not see the new container | Turn on **Track container events** for the environment, or open the Backups page or press **Re-link stale containers**, which re-resolve the link by name |
| Backups only exist locally | No storage destination is configured | Local is the default; remote destinations need a storage plugin |

## When a control is disabled

A disabled control with a tooltip naming a permission is RBAC, not a fault; the tooltip
names the exact key to ask for. A disabled control with no tooltip is usually state: the
action does not apply to the current status of the thing you selected.

## Related

- [Backups](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what each action really does
