# Reporting: Developer

How this plugin is put together, and what to change if you fork it.

## What it registers

It is a feature plugin: it registers routes, menu entries, widgets and workflow nodes
of its own rather than filling an adapter slot. See its manifest table below for what it
declares.

## Source layout

| File | Lines |
|---|---|
| `backend/adhoc_author.go` | 347 |
| `backend/adhoc_facets.go` | 147 |
| `backend/adhoc_plan.go` | 284 |
| `backend/adhoc_slice.go` | 679 |
| `backend/adhoc_verify.go` | 260 |
| `backend/alerts.go` | 261 |
| `backend/analyze.go` | 1311 |
| `backend/build_common.go` | 906 |
| `backend/build_facets.go` | 796 |
| `backend/build_reports.go` | 720 |
| `backend/build_reports2.go` | 686 |
| `backend/build_reports3.go` | 747 |
| `backend/build_reports4.go` | 988 |
| `backend/build_reports5.go` | 1201 |
| `backend/build_reports6.go` | 893 |
| `backend/catalog.go` | 529 |
| `backend/collect.go` | 1132 |
| `backend/embed.go` | 83 |
| `backend/export_docx.go` | 497 |
| `backend/export_pdf.go` | 1168 |
| `backend/facets.go` | 399 |
| `backend/facets_docker.go` | 292 |
| `backend/families.go` | 44 |
| `backend/family_governance.go` | 709 |
| `backend/family_governance_approvals.go` | 649 |
| `backend/family_governance_compliance.go` | 369 |
| `backend/family_governance_secrets.go` | 498 |
| `backend/family_governance_webhooks.go` | 397 |
| `backend/family_governance_workflows.go` | 372 |
| `backend/family_resilience.go` | 366 |
| `backend/family_resilience_dr.go` | 637 |
| `backend/family_resilience_drift.go` | 554 |
| `backend/family_resilience_ha.go` | 528 |
| `backend/family_resilience_incidents.go` | 537 |
| `backend/family_resilience_itsm.go` | 454 |
| `backend/format.go` | 404 |
| `backend/handler.go` | 744 |
| `backend/handler_schedules.go` | 669 |
| `backend/hostapi.go` | 258 |
| `backend/hostbranding.go` | 123 |
| `backend/jobs.go` | 222 |
| `backend/main.go` | 332 |
| `backend/model.go` | 370 |
| `backend/narrative.go` | 325 |
| `backend/nodes.go` | 432 |
| `backend/pagesize.go` | 45 |
| `backend/providers.go` | 578 |
| `backend/scheduler.go` | 643 |
| `backend/schedules.go` | 1069 |
| `backend/service.go` | 840 |
| `backend/store.go` | 538 |
| `backend/subject.go` | 515 |
| `backend/theme.go` | 228 |
| `backend/triggers.go` | 190 |

A `frontend/` bundle ships alongside the backend: a UMD build the host loads at
runtime. It supplies the UI this plugin declares.

## Adding a report type

There are four ways to add one. Choose by who owns the data.

**A facet.** This is for a platform that owns workloads and should appear in the shared reports
rather than in a report of its own. The platform plugin declares `Facets` on its
`ReportProviderSpec` and implements `ReportFacetContributor`. This plugin asks through
`ReportFacetHandle` in `facets.go`, answers for Docker itself in `facets_docker.go`, and decides
which built-in report prints which facet in the `reportFacetUses` table in `build_facets.go`. A
new report that should show a facet adds one line to that table. The contract is on
[Contribute facets to the shared reports](../../how-to/reporting/facets/README.md).

**A built-in type in the original set.** Add a catalog entry in `catalog.go`, a builder in one
of the `build_reports*.go` files, an entry in the `builders()` table in `build_common.go`, and
a title and subtitle in `reportTitle` / `reportSubtitle` in `service.go`. The studio labels it
from `type.<key>.name` and `type.<key>.description` in `i18n/<lang>/reporting.json`, in all
six locales.

**A report family.** This is for a group of host-feature reports that bring their own data
sources. `families.go` defines `reportFamily`, and each family lives in its own files:
`family_resilience*.go` (incidents, drift, instance DR, HA, ITSM) and `family_governance*.go`
(approvals and elevations, secrets, workflows, webhooks, compliance). A family declares its
catalog entries, builders, titles, the sources that vary by environment, its collection jobs,
and optional hooks for the headline KPIs, the findings and the summary's closing sentence. The
shared catalog, collector, summary and title code consult every family through
`reportFamilies()`. A new family adds one field to `Snapshot` and one line to
`reportFamilies()`, and touches none of the shared switch statements.

**A contributed type.** This is for data another plugin owns. That plugin registers a
`plugin.ReportProviderSpec` through `AdapterRegistry.Reporting()` and declares
`reporting.adapter.register` in its manifest. `providers.go` turns each declaration into a
catalog entry and converts the provider's neutral document into this plugin's blocks.
Conventions a provider should follow:

- Keys and subject kinds are prefixed with the plugin family (`kvm-backup-assurance`,
  `siem-forwarder`). A subject kind without a `-` is ignored.
- `LabelKey` / `DescriptionKey` are namespaced, for example `vm:reports.protection.label`. The
  namespace before the colon is also where the studio looks for option labels, under
  `reports.options.<key>`.
- Cell, KPI and chart statuses are `good`, `warn`, `bad`, `info` or `muted`. `success`,
  `warning`, `danger` and `error` are accepted as aliases; anything else renders untinted.
- Every source consulted comes back as a `SourceStatus`, so the Methodology annex can name what
  was read and what failed.

The full list of types, and what each one reads, is in the [report catalog](../../reporting/reports/README.md).

## Manifest

`plugin.yaml` is the source of truth. The loader mounts only what it declares, so code
that is registered but not declared is unreachable.

| Field | Value |
|---|---|
| `slug` | `addon-reporting` |
| `version` | 2026.09.01 |
| `mcharbor` | `>=2026.09.01` (the host range the installer enforces) |
| `go` | `>=1.23` |
| `license` | Apache-2.0 |
| `table_prefix` | `plugin_reporting_` |
| `permissions` | `menu.sidebar.add`, `i18n.namespace.add:reporting`, `rbac.permission.register`, `db.migrations.run`, `audit.emit`, `alerting.adapter.register` |
| `adds_permissions` | `reporting.read` (env), `reporting.generate` (env), `reporting.manage` (env), `reporting.schedule` (env) |
| `contributes.i18n_namespaces` | `reporting` |
| `contributes.forms` | _none_ |
| `contributes.widgets` | `recent-reports`, `report-schedules` |
| `contributes.nodes` | `report-generate`, `report-export`, `report-schedule-run`, `report-generated`, `report-schedule-ran` |
| `migrations` | `0001_init.sql`, `0002_adhoc.sql`, `0003_ai_connection.sql`, `0004_schedules.sql`, `0005_schedule_target_path.sql`, `0006_schedule_target_recipient.sql` |

## Building it

There are no per-plugin Makefiles in this repository. `src/plugins/Makefile` iterates
`Plugins/*/Makefile`, which matches nothing today, so its `build` and `sign` targets are
no-ops. Build it the way the image does instead:

```bash
cd src/plugins/Plugins/addon-reporting
CGO_ENABLED=1 go build -trimpath -buildmode=plugin -o backend/addon-reporting.so ./backend
```

The `.so` has to be built against the **same** Go toolchain and module graph as the host
binary, or `plugin.Open` rejects it with `plugin was built with a different version of
package …`. That is why `docker/Dockerfile` compiles the host binary and every bundled
plugin inside one `RUN`, sharing one module cache and one build cache.

For a plugin that ships in the image, the only reliable way to get a rebuild into a
running McHarbor is to rebuild the image:

```bash
docker compose build && docker compose up -d
```

The entrypoint then copies the bundled `.so` over the installed one and re-extracts the
matching tarball payload, so the frontend bundle, i18n and icons move with it.

## Signing it

Only needed to distribute outside the image. Signing uses ed25519 through PyNaCl:

```bash
python -m venv .venv
.venv/bin/pip install -r src/plugins/scripts/requirements-signing.txt
.venv/bin/python src/plugins/scripts/sign-plugin.py \
  --tarball dist/mcharbor-plugin-addon-reporting-1.0.0.tar.gz \
  --key signing/plugin-signing.key
```

It writes a base64 ed25519 signature next to the tarball as `.sig`, which is exactly what
the installer verifies. Keep the private half out of the repository, and add the public
half to the host's trust list through `MCHARBOR_PLUGIN_TRUST_KEYS`.

Install the tarball from **App Store → Plugins**, or drop it into a catalog directory
the host reads.

## Forking it

1. Copy the folder to a new slug and change `slug`, `name` and `homepage` in
   `plugin.yaml`. The slug has to match the directory name.
2. Change the route prefix and the sidebar section id so it does not
   collide with this plugin's. Two plugins cannot claim the same key.
3. Rename the permission keys in `adds_permissions`. A plugin cannot shadow another
   plugin's key, and the host skips a duplicate.
4. Update `ConfigSchema` for the fields your service needs, then the client that reads
   them.
5. Rename the i18n namespace (`reporting`) and the `i18n/` folder to match.
6. Rebuild, re-sign with your own key, and add that key to the host's trust list
   (`MCHARBOR_PLUGIN_TRUST_KEYS`).

## Related

- [Reporting](../../reporting/README.md): what it is and how it works
- [Plugin architecture](../plugins/architecture.md): the loader, the ABI check, signing and trust
- [Developing a plugin](../plugins/developing.md): the SDK, the manifest reference and the developer loop
