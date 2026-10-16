# Build a reporting plugin

A reporting plugin turns the platform's own records into documents someone can hand to a
board. It is the most demanding plugin shape in the product. It reads across every module,
schedules itself, renders to PDF and Word, delivers to channels and storage, and contributes
alert conditions, which makes the shipped reporting add-on the best worked example of nearly
every plugin capability at once.

This page is that example, read as a set of decisions you will face in any plugin that
aggregates platform data.

## What it produces

Twelve report types, each declaring which **management altitudes** it carries:

| Level | Audience | Answers |
|---|---|---|
| Strategic | Board, CIO | Are we safe, and where should money go? |
| Tactical | Platform lead, service owner | Are we hitting our targets, and what is drifting? |
| Operational | Engineers on shift | What exactly is happening right now? |

The levels are declared in the catalog, not hard-coded; a type carries one, two or all three.
Every report opens with a one-page executive summary and closes with a methodology annex
listing every source consulted, whether it answered, and the rules behind each score.

That annex is not decoration. A report whose numbers cannot be traced is a report nobody can
defend in the meeting it was written for.

## Decision 1: how to read the platform

This is the decision that defines the plugin, and the shipped one takes the harder path
deliberately.

**It calls McHarbor's own REST API over loopback, forwarding the requesting user's session
cookie, or the Authorization header when the request used an API key.**

The consequence is the whole security model: a user can never produce a report containing data
they could not already see in the interface. Reading the database directly would bypass roles,
ABAC rules and environment scoping, and in a product where "who can see production" is a real
question, that is not a trade worth making.

Two consequences to design around:

- **A 403 or 404 from a module is not an error.** It means the user lacks permission, or the
  module is disabled. Record the source as unavailable and say so in the annex; do not fail
  the report.
- **A scheduled report is collected as the user who created the schedule**, through a session
  the host issues for them. So the scheduling permission grants the ability to *automate* a
  report, never the ability to see more in one.

If your plugin aggregates platform data, copy this. If it aggregates its own data, it does not
apply.

## Decision 2: how to split the permissions

Four keys, and the reasoning behind each is more instructive than the list:

| Key | Presets | Gates |
|---|---|---|
| `reporting.read` | viewer, editor, operator | The catalog, the library, reading a finished report |
| `reporting.generate` | editor, operator | Producing a report, the expensive, data-touching act |
| `reporting.schedule` | editor, operator | Viewing schedules, creating them, reading a schedule's last run |
| `reporting.manage` | operator | Deleting reports, editing and deleting schedules, changing settings |

Three things worth copying:

**Generation is separated from reading.** They are different costs and different risks.

**Some reads are gated above `read`.** The schedule routes are gated on `reporting.schedule`
rather than `reporting.read`, because a schedule's last run carries a whole report document,
and the delivery-target listing names the channels and storage locations the installation has
configured. The right question is never "is this a read" but "what does this read expose".

**Running a schedule on demand takes the generation key**, not the scheduling one, because the
capability being exercised is generation. Editing and deleting a schedule take `manage`, the
same key that already governs deleting a report.

See [Contract permissions](../permissions/README.md).

## Decision 3: where the menu entry points

All three menu entries share a `parent` section id, so the sidebar renders a **Reporting**
section of its own rather than three loose items under Plugins.

The studio entry is gated on `reporting.generate`, not `reporting.read`, because every route
the studio calls needs `generate`. Gating it on `read` let a read-only user browse the
catalogue and only meet the wall at the Generate button. Reading finished reports is the
library's job, and the library entry is the one gated on `read`.

Gate a menu entry on what its **first action** needs.

## Decision 4: generation is a job, not a request

Report generation is moved off the request path:

```
POST /api/reporting/jobs      start a generation job
GET  /api/reporting/jobs/{id} poll it
```

That is what lets the studio show real progress rather than a spinner, and it is the right
shape for anything that fans out across many modules. The companion endpoints follow the same
logic: `GET /subjects` lists only what the calling user can already see, and `GET /preview`
renders a sample without persisting anything.

## Decision 5: audit every mutation, in the manifest

Every mutating route declares its audit metadata alongside its permission:

```yaml
- method: POST
  path: "/api/reporting/reports"
  requires_permission: "reporting.generate"
  audit_required:    true
  audit_action:      "plugin.reporting.report.generated"
  audit_entity_type: "report"
```

The convention `plugin.<slug>.<entity>.<verb>` is what makes the audit log filterable by
plugin and by entity later. Pick it before you have forty routes, not after.

## Decision 6: contribute alert conditions

The plugin registers three conditions with the host's alert engine: a schedule whose last run
failed, a schedule that has stopped producing anything at all, and a failed generation.

The division of labour is the point. **The plugin owns the condition; the engine owns
everything else**: the thresholds, the evaluation schedule, the firing state, the cooldown
and the delivery. A plugin that also implemented notification delivery would be reimplementing
a subsystem, badly, and would not appear in the alert rule editor at all.

The scope token is `alerting.adapter.register`.

## Decision 7: rendering

Exports go to PDF and Word with the product's own typography and marks. Practical notes for
anyone doing the same:

- **Fonts have to be embedded.** McHarbor is frequently deployed air-gapped; a document that
  depends on a font being present renders differently everywhere it opens.
- **Rendering is CPU-bound and slow.** That is the other reason generation is a job.
- **Charts are rendered server-side**, because the export path has no browser to render them
  in.
- **Page size is a setting**, not an assumption. A4 and Letter both have to work.

## Decision 8: scheduling and delivery

A schedule names a report, a scope, a cadence and a set of delivery targets. Two rules make it
safe:

- A run is collected as the schedule's creator, so its contents stay bounded by that person's
  permissions; automation never becomes escalation.
- Delivery targets are the host's existing communication channels and storage locations, not
  a private list. The plugin lists them through a permission-gated endpoint rather than
  configuring transports of its own.

## The manifest, in summary

```yaml
permissions:
  - menu.sidebar.add
  - i18n.namespace.add:reporting
  - rbac.permission.register
  - db.migrations.run
  - audit.emit
  - alerting.adapter.register

table_prefix: "plugin_reporting_"
```

The table prefix is declared explicitly because the plugin was renamed and its tables kept
their original names. Uninstall drops tables by prefix, so the prefix, not the slug,
has to be the truth.

## Related

- [Build a plugin](../plugin/README.md): the end-to-end path
- [Contribute facets to the shared reports](./facets/README.md): how a platform plugin answers inventory, capacity, protection, health, exposure and cost
- [Plugin manifest](../plugin/manifest.md): audit metadata, sections, prefixes
- [Contract permissions](../permissions/README.md)
- [Reporting plugin](../../reporting/README.md): the shipped add-on from an operator's view
- [Monitoring endpoints](../../api/endpoints/monitoring.md): the alert engine it contributes to
