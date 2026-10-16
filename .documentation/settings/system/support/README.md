# Supportability

**Enterprise → Supportability** · `/enterprise`

Four read-mostly tools for finding out what is wrong with an instance and for handing support
what they need without handing them a credential.

| Page | Route | Covers | Gated by |
|---|---|---|---|
| [Support bundle](./support-bundle.md) | `/enterprise/support-bundle` | A redacted `.zip` of configuration, logs, plugins, cluster, database, environments, denials and diagnostics | `support.view` to preview and list, `support.manage` to generate, download and delete |
| [Diagnostics](./diagnostics.md) | `/enterprise/diagnostics` | Twelve pass / warning / fail checks with remediation, runnable one at a time | `support.view` |
| [State inventory](./state-inventory.md) | `/enterprise/state-inventory` | Everything under `DATA_DIR`: size, writability, backup inclusion, cleanup eligibility | `support.view` |
| [Pre-upgrade check](./pre-upgrade-check.md) | Settings → Updates → **Pre-upgrade check** | Plugin compatibility, disk, Docker, running work, migrations and rollback guidance before an upgrade | `updates.view` |

The **Supportability** card on the Enterprise overview links to the first three.

## Permissions

| Key | Scope | What it unlocks |
|---|---|---|
| `support.view` | global | Diagnostics, the state inventory, the bundle history and the redaction preview. Also shows the encryption-key banner |
| `support.manage` | global | Generating, downloading and deleting support bundles |

Neither key is in a role preset. An administrator role with `*` holds both.

## The encryption-key warning

When `ENCRYPTION_KEY` is not set, McHarbor generates a key into `DATA_DIR/.encryption_key` on
first start. Every stored secret is encrypted with it, so losing that file (a volume recreated,
a restore to a fresh host) makes every secret unreadable, and a second high-availability node
would generate a different key.

Users holding `support.view` see a warning banner at the top of every page while that is the
case, with a link to Diagnostics. **Dismiss** hides it in that browser only; the
**Data encryption key** check in Diagnostics keeps reporting it as a warning until the key is
moved into `ENCRYPTION_KEY`:

1. Read the file: `base64 < DATA_DIR/.encryption_key`.
2. Set that value as `ENCRYPTION_KEY` in the container's environment.
3. Recreate the container. The banner no longer appears.

## Related

- [High availability](../high-availability/README.md): why every node needs the same key
- [Instance backup](../instance-backup/README.md): what the inventory's *Instance backup* column cross-checks
