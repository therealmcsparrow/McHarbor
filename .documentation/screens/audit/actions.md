# Audit: Actions

What you can do with the audit trail (browse the catalogue of canonical actions, open an
entry, verify the hash chain, take a signed export, and purge past the retention window)
and the permission each one needs.

| Action | Gated by | What it does |
|---|---|---|
| **Browse actions** | `audit.view` | Opens the catalogue of canonical actions to filter by, instead of asking you to know the string |
| **View details** | `audit.view` | The full entry |
| **Export** | `audit.export` | A signed export of the trail. The **Columns** picker trims it to chosen fields; the actor and severity filters from the Events tab apply. The signature covers the columns and every filter, so a trimmed or narrowed file cannot pass as the full trail |
| **Export** (Coverage tab) | `audit.export` | Downloads the coverage report itself as `mcharbor-audit-coverage.json`, beside the percentage badge. The download is audited as an export |
| **Verify integrity** | `audit.view` | Runs `GET /api/audit/verify` on demand and shows the verdict, the counts, the retention gaps it crossed and every restore anchor. See [Integrity](./integrity.md) |
| **Save retention** | `settings.manage` (and `audit.view` to reach the tab) | Stores per-severity retention. Takes effect on the next hourly prune. See [Retention](./retention.md) |
| **Open user** (RBAC denials) | `audit.view` | Opens the denied user's page under Security → Users |
| **Purge** | `settings.manage` | Deletes entries past their retention, per severity where one is set, the default otherwise. "This cannot be undone" |

## Confirmation

Destructive actions confirm first and say how many items they will affect. A confirmation
that names a consequence ("this cannot be undone", "all its data") is quoting the
product, not padding.

## Related

- [Audit](./README.md): what the screen is and what it shows
- [Fields](./fields.md): the inputs these actions read
- [Troubleshooting](./troubleshooting.md): when an action does not do what you expect
