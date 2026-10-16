# Reporting: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A role holding at least this plugin's `view` key, or its sidebar section stays hidden.

## Where it appears

| Path | Label key | Permission |
|---|---|---|
| `/plugins/addon-reporting` | `menu.studio` | `reporting.generate` |
| `/plugins/addon-reporting/library` | `menu.library` | `reporting.read` |
| `/plugins/addon-reporting/schedules` | `menu.schedules` | `reporting.schedule` |
| `/plugins/addon-reporting/settings` | `menu.settings` | `reporting.manage` |

Sidebar entries are **hidden** when the permission is missing, unlike buttons, which
render disabled with a tooltip. A dead-end page is not informative.

## Routes

Every route is mounted under the plugin router and carries the permission declared in
the manifest. The host mounts only what the manifest declares.

| Method | Path | Permission | Audit action |
|---|---|---|---|
| `GET` | `/api/reporting/catalog` | `reporting.read` | — |
| `GET` | `/api/reporting/reports` | `reporting.read` | — |
| `POST` | `/api/reporting/reports` | `reporting.generate` | `plugin.reporting.report.generated` |
| `GET` | `/api/reporting/subjects` | `reporting.generate` | — |
| `POST` | `/api/reporting/jobs` | `reporting.generate` | `plugin.reporting.report.generated` |
| `GET` | `/api/reporting/jobs/{id}` | `reporting.generate` | — |
| `GET` | `/api/reporting/reports/{id}` | `reporting.read` | — |
| `DELETE` | `/api/reporting/reports/{id}` | `reporting.manage` | `plugin.reporting.report.deleted` |
| `GET` | `/api/reporting/reports/{id}/export` | `reporting.read` | — |
| `GET` | `/api/reporting/preview` | `reporting.generate` | — |
| `GET` | `/api/reporting/settings` | `reporting.read` | — |
| `PUT` | `/api/reporting/settings` | `reporting.manage` | `plugin.reporting.settings.updated` |
| `GET` | `/api/reporting/ai/connections` | `reporting.generate` | — |
| `GET` | `/api/reporting/schedules` | `reporting.schedule` | — |
| `POST` | `/api/reporting/schedules` | `reporting.schedule` | `plugin.reporting.schedule.created` |
| `GET` | `/api/reporting/schedules/{id}` | `reporting.schedule` | — |
| `PUT` | `/api/reporting/schedules/{id}` | `reporting.manage` | `plugin.reporting.schedule.updated` |
| `DELETE` | `/api/reporting/schedules/{id}` | `reporting.manage` | `plugin.reporting.schedule.deleted` |
| `POST` | `/api/reporting/schedules/{id}/run` | `reporting.generate` | `plugin.reporting.schedule.run_requested` |
| `GET` | `/api/reporting/schedules/{id}/run` | `reporting.schedule` | — |
| `GET` | `/api/reporting/schedules/{id}/run/export` | `reporting.schedule` | — |
| `GET` | `/api/reporting/delivery-targets` | `reporting.schedule` | — |

Secrets are encrypted with the host's AES-256-GCM service before they are written, and
the API never returns them once saved. Editing an existing entry leaves a stored secret
in place unless you replace it.

## Setting it up

- Install it from **App Store → Plugins**.
- Grant its permission keys to the roles that should see it.
- Open its section in the sidebar.

## Report branding

Every cover page carries a wordmark and an icon. What it uses, most specific first:

1. **Report settings → Branding**: a mark uploaded here, for estates whose reports
   carry a different mark from their console.
2. **[Settings → Customization](../settings/general/themes/README.md)**: the
   install's own application logo, **light** variant. This is the default, so a
   rebranded McHarbor produces rebranded reports with no configuration here at all.
3. The McHarbor mark compiled into the plugin.

Two things are worth knowing:

- **The light marks, not the dark ones.** A report is printed, photocopied and pasted
  into decks, so it is laid out on a light ground. That is also why its palette is
  light while the console is dark-first.
- **PNG, not SVG.** The PDF and Word writers embed raster images; neither rasterises
  vector artwork. The host rasterises the light marks in the browser when you upload
  them and publishes the rendition, so an SVG application logo still reaches a report.
  If a logo was set before this existed, re-upload it once in Settings → Customization
  to generate the rendition.

## Host scopes it requests

These are the capabilities the manifest asks the host for. The installer refuses a
manifest asking for a scope outside the known set.

| Scope token | What it allows |
|---|---|
| `menu.sidebar.add` | Add an entry to the sidebar. |
| `i18n.namespace.add:reporting` | Merge the `reporting` translation namespace into the host bundles. |
| `rbac.permission.register` | Reserve the permission keys it declares in `adds_permissions`. |
| `db.migrations.run` | Create and migrate its own tables at install time. |
| `audit.emit` | Write entries into the tamper-evident audit trail. |
| `alerting.adapter.register` | Contribute alert conditions the host evaluates on a schedule. |

## Related

- [Reporting](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
