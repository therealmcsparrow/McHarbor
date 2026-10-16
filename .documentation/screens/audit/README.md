# Audit

`/audit`

Who asked McHarbor to do what, in a trail designed to show whether it has been tampered
with.

## Permissions

| Key | What it unlocks |
|---|---|
| `audit.view` | Read the trail, the RBAC denials, the AI action trail and the retention policy; run **Verify integrity** |
| `audit.export` | Export it, and export the coverage report |
| `settings.manage` | Purge, configure forwarding, and change retention by severity (together with `audit.view`) |

Everywhere except navigation, a control you lack the key for renders **disabled with a
tooltip naming it**, rather than disappearing. Navigation is the exception: a sidebar
entry you cannot use is hidden, because a dead-end page is not informative.

## What it shows

- Five tabs: **Events**, **RBAC denials**, **AI action trail**, **Retention** and **Coverage**.
  The tab is kept in the address as `?view=events|denials|ai|retention|coverage`.
- Events: timestamp, severity, user, canonical action, entity, environment and IP address,
  filterable by actor and severity as well as action, entity type, date range and text.
- RBAC denials: every request a role permission or an ABAC policy refused. See
  [RBAC denials](./rbac-denials.md).
- AI action trail: each assistant action from preview through consent to execution. See
  [AI action trail](./ai-action-trail.md).
- Retention: how long each severity is kept. See [Retention](./retention.md).
- Coverage: how much of the product emits canonical audit actions, how much relies on fallback
  middleware, and what each plugin has recorded, including plugins that are gone.

## Integrity

The trail is hash-chained and write-once: each entry commits to the one before it, so
removing or editing an entry breaks the chain from that point on. A scheduled verifier
checks it hourly, **Verify integrity** in the page header checks it on demand, and exports
can be signed. Verification is what makes the trail evidence rather than a log file.

Retention is the one sanctioned way rows leave the chain, and it leaves a sealed tombstone
behind so the chain still verifies across the gap. See [Integrity](./integrity.md).

## The trail outlives what it names

An audit row has to stay readable after the thing it describes is deleted, or it is not an
audit trail. Two things make that true:

- **The environment's name and the entity's name are written onto the row** at the moment
  the event is recorded, rather than looked up against the current list when you read it.
  Deleting an environment no longer degrades every historical row that mentions it to a
  truncated id. Free-text search matches the name **as it was when the event happened**, so
  renaming something does not retroactively rewrite history.
- **`audit_logs` is a governed table.** Its rows are supposed to point at users and
  environments that no longer exist. The
  [database health check](../../settings/system/database/health-check.md) reports those
  dangling references so you can see them, but files them as *Info* with the reason
  *governed audit record*, offers no remedy, and refuses the table outright if asked to
  prune it. Audit retention is a compliance decision and is set under
  [Data retention](../../settings/monitoring/data-retention/README.md), not by a
  space-reclaiming sweep.

## Coverage

| Figure | Meaning |
|---|---|
| **&lt;n&gt;% covered** | Mapped as a share of required; the badge turns amber as soon as one required route is unmapped |
| **Routes requiring audit** | Routes the register marks `audit_required` |
| **Mapped to an action** | Of those, the ones that name a canonical action |
| **Unmapped** | The difference: holes in the record, and the figure the badge follows |

The register behind these numbers is `audit_action_route_map`, a list of the sensitive
routes with the action each should emit and a one-line rationale for why it is on the list.

A mutating route with no explicit audit row still produces an entry. Fallback middleware
looks the route up in the same register and, failing that, files a less specific
`<module>.route.created`. So an unmapped route is under-described, not silent.

### Plugin activity

Under the figures is every plugin that has ever written an audit event, with its slug, when
it last wrote one, and how many it has written.

**A plugin that is no longer installed keeps its row, marked "Uninstalled".** This is
deliberate: an uninstall is itself a plugin audit event, so dropping the slug when the
plugin goes would delete the record of its removal, exactly the evidence an audit trail
exists to keep. Read a marked row as history: the last-event date says when it was last
active, and the count is what it did while it was here. A plugin that is installed and
should be auditing but appears nowhere is the interesting case.

## Getting it out

Export is a snapshot taken by someone with access. **Forwarding** is the stronger control:
it puts the trail somewhere that access does not reach. See
[Settings → Integrations → Logging](../../settings/integrations/logging/README.md).

## In this folder

| Page | What it covers |
|---|---|
| [Actions](./actions.md) | Everything you can do here, what gates it, and what it really does |
| [Fields](./fields.md) | Every column, filter and form field the screen exposes |
| [RBAC denials](./rbac-denials.md) | The denial log: what is recorded, the filters, and the throttle |
| [AI action trail](./ai-action-trail.md) | AI actions grouped by action id, and the redaction applied |
| [Retention](./retention.md) | Retention by severity and how it interacts with the default |
| [Integrity](./integrity.md) | Verify integrity, restore anchors, and retention tombstones |
| [Troubleshooting](./troubleshooting.md) | Symptom, cause and fix |

## Related

- [Activity](../activity/README.md) and [Lifecycle log](../lifecycle/README.md): what happened, rather than who asked
- [Settings → Integrations → Logging](../../settings/integrations/logging/README.md): forwarding the trail out
- [Settings → Monitoring → Data retention](../../settings/monitoring/data-retention/README.md): how long it is kept
