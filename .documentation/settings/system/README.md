# System

What McHarbor is running as.

| Section | Covers | Gated by |
|---|---|---|
| [Database](./database/README.md) | Storage engine, size, and migrating between backends | `database.view` / `database.manage` |
| [High availability](./high-availability/README.md) | Active/passive nodes on PostgreSQL, failover, drain, plugin package stores, probes | `cluster.view` / `cluster.manage` |
| [Instance backup](./instance-backup/README.md) | Encrypted export of this instance, restore, schedules and restore rehearsals | `instance_backup.view` / `.manage` / `.restore` / `.restore_audit` |
| [Demo mode](./demo-mode/README.md) | A simulated estate on a separate database (environments, stacks, VMs, clusters, Git and GitOps), with every outbound path kept offline | `system.demo` |
| [Supportability](./support/README.md) | Support bundles, diagnostics, the `DATA_DIR` state inventory, the pre-upgrade check and the encryption-key warning | `support.view` / `support.manage`; `updates.view` for the pre-upgrade check |
| [Agent](./agent/README.md) | Defaults and diagnostics for the remote agent transport | `settings.manage` |
| [Licence](./license/README.md) | Licence key, edition, seats and entitlement status | `license.view` / `license.manage` |
| [About](./about/README.md) | Version, dependencies, update checking, and factory reset | `settings.manage` to act |

## The two that can disconnect you

**Database** migration restarts McHarbor into the new database, and **About → Update now**
replaces the container. Both say so before they act, and both keep the previous state: the
old database is never touched, and the previous image is still in the registry.

**Factory reset**, also on About, keeps nothing McHarbor owns. Containers, images and
volumes on your hosts are untouched.

## Licensing does not gate anything

Whatever the licence section says (over licence, expired, suspended, revoked, offline),
nothing is switched off. Every environment keeps running.

## Related

- [Settings](../README.md): the other five groups
