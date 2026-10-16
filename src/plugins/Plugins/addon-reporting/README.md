# McHarbor Reporting

Turns the platform's own records into board-presentable documents.

Twelve report types, organised by management altitude, readable inside
McHarbor and exportable to **PDF** and **Word (.docx)** with the
product's own typography and marks.

---

## What it produces

Every report is split by the altitude it is written for. A type carries
one, two or all three levels — declared in the catalog, not hard-coded:

| Level | Audience | Answers |
|---|---|---|
| **Strategic** | Board, CIO | Are we safe, and where should money go? |
| **Tactical** | Platform lead, service owner | Are we hitting our targets, and what is drifting? |
| **Operational** | Engineers on shift | What exactly is happening right now? |

Every report opens with a one-page executive summary — posture
scorecard, what changed, matters requiring attention, recommended
actions — and closes with a Methodology annex that lists every source
consulted, whether it answered, and the exact rules behind each score.

### Report types

| Key | Levels |
|---|---|
| `executive-overview` — Executive Platform Overview | S T O |
| `security-compliance` — Security & Compliance | S T O |
| `incident-problem` — Incident & Problem Review | S T O |
| `availability-slo` — Availability & Service Levels | S T |
| `change-delivery` — Change & Delivery (DORA) | S T |
| `environment-health` — Environment Health | T O |
| `capacity-utilisation` — Capacity & Utilisation | T O |
| `backup-recovery` — Backup & Recovery | T O |
| `image-supply-chain` — Image & Supply Chain | T O |
| `access-review` — Access & Identity Review | T |
| `workload-inventory` — Workload Inventory (every platform) | O |
| `platform-inventory` — Platform Inventory | O |

---

## How it reads the platform

Report data is collected by calling **McHarbor's own REST API over
loopback, forwarding the requesting user's session cookie**.

That is the whole security model, and it is deliberate: a user can never
produce a report containing data they could not already see in the
interface. Reading the database directly would bypass roles, ABAC rules
and environment scoping — in a product where "who can see production" is
a real question, that is not a trade worth making.

Consequences worth knowing:

- A module that is disabled, or that the user lacks permission for,
  returns 403/404. That is **not** an error: the source is recorded as
  *not available*, the affected domain is reported as **Not assessed**,
  and it is excluded from the overall score. An absent scanner never
  reads as a clean bill of health.
- Collection runs concurrently with a 30-second deadline per call. A
  full executive report touches around twenty endpoints.
- Secrets never enter the plugin. Socket paths, TLS material,
  kubeconfig, bearer tokens, agent tokens, registry credentials and API
  key prefixes are dropped at the JSON boundary by decoding into narrow
  structs.

---

## Cross-platform facets

Six questions — inventory, capacity, protection, health, exposure and
cost — are answered by every platform in one row shape. This plugin
answers for Docker from its own collection; each installed platform
plugin answers for itself through the host's reporting family. The
reports that take a facet print one table with a Platform column, a
per-platform breakdown, and each platform's own passage under its name.
A platform that is not installed is absent; one that fails is listed in
the Methodology annex as not available, never counted as zero.

---

## Scoring

Six domains, each scored 0–100 from published rules, combined as a
weighted mean over the domains that could be assessed:

| Domain | Weight |
|---|---|
| Security | 25 % |
| Availability | 20 % |
| Resilience | 20 % |
| Stability | 15 % |
| Capacity | 10 % |
| Delivery | 10 % |

Bands: **85+** strong · **70–84** stable · **55–69** watch ·
**35–54** at risk · **below 35** critical.

The rules and the weighting are printed in every report's Methodology
annex, so a reader who disputes a number can see exactly how it was
reached. Delivery uses the DORA benchmarks for deployment frequency and
change failure rate; lead time and failed-deployment recovery are
reported as *not measured* rather than estimated from data the platform
does not hold.

---

## Exports

Both exporters are written inside this plugin against the standard
library alone. **The plugin has no third-party Go dependencies.**

Every plugin build failure in this repository so far has been a
dependency problem — `go.sum` gaps, transitive modules, ABI drift under
`go work sync`. Removing dependencies removes that entire class of
failure, and the subset of PDF and OOXML a report needs is small and
completely specified.

**PDF** (`backend/pdf`) — PDF 1.7, Flate-compressed streams, TrueType
embedded as `Type0` / `Identity-H` / `CIDFontType2` with a `ToUnicode`
CMap so the text stays selectable and searchable. Charts are drawn as
vectors, so they stay crisp at any zoom and print cleanly. Two-pass
layout produces a contents list with real page numbers, plus a PDF
outline for sidebar navigation.

**Word** (`backend/docx`) — hand-built WordprocessingML. Named styles
that land in Word's style gallery, real tables with repeating header
rows, a live `TOC` field, header/footer parts with `PAGE`/`NUMPAGES`,
and the brand fonts embedded as obfuscated `.odttf` so the file looks
identical on a machine that has never heard of Inter Tight.

Charts in the Word export are rendered from the *same geometry* the PDF
draws (`backend/charts`), rasterised at 4× by the glyph rasteriser in
`backend/fontkit`. Word has no vector primitive we can emit without
embedding a spreadsheet part, and a crisp raster is the right trade for
a document that gets printed.

### Known limitation

The embedded Inter Tight covers Latin, Latin-Extended, Greek and
Cyrillic. **CJK and Arabic text in a PDF export will not render**; the
interface remains fully localised, and the Word export falls back to the
reader's own system fonts for those scripts. Shipping Noto CJK would add
roughly 10 MB to the plugin binary for a case most installs never hit.

---

## Layout

```
reporting/
├── plugin.yaml                 manifest: routes, menu, permissions, migrations
├── migrations/0001_init.sql    plugin_reporting_* tables
├── icons/                      sidebar icons
├── i18n/<12 locales>/reporting.json
├── backend/
│   ├── main.go                 Plugin symbol and lifecycle
│   ├── handler.go              HTTP layer only
│   ├── service.go              collect → analyse → build → persist
│   ├── store.go                SQL over plugin_reporting_*
│   ├── hostapi.go              loopback client, session passthrough
│   ├── collect.go              data collectors → Snapshot
│   ├── analyze.go              facts, scoring, bands
│   ├── narrative.go            deterministic prose (AI polish optional)
│   ├── catalog.go              the twelve report types
│   ├── model.go                Document / Section / Block
│   ├── build_*.go              per-type document builders
│   ├── export_pdf.go           Document → PDF
│   ├── export_docx.go          Document → DOCX
│   ├── pdf/ docx/ charts/ fontkit/
│   └── assets/                 fonts (OFL) + rasterised marks
└── frontend/src/               Studio, Library, Viewer, Settings
```

---

## Workflow nodes

The plugin contributes three nodes to the workflow editor, listed in the
**Action** group of the palette once the plugin is loaded. Every node
has an `output` and an `error` port; a failure answers on `error` with a
short reason in `msg.payload.error` rather than aborting the run.

| Node | Palette gate | Config | Result in `msg.payload` |
|---|---|---|---|
| Reporting: Generate Report | `reporting.generate` | report type, environment, period (days), levels, output (`summary` / `pdf` / `docx`), save to library | the report summary (`id`, `title`, `overallScore`, `overallBand`, `headline`, ...); with a file output also `fileName`, `contentType`, `size` and base64 `data` |
| Reporting: Export Report | `reporting.read` | report id (empty = `msg.payload.id`), format | `{ id, format, fileName, contentType, size, data }` |
| Reporting: Run Schedule | `reporting.schedule` | schedule (id or exact name) | `{ scheduleId, scheduleName, reportType, started }` — the run itself is asynchronous and lands in the schedule's run history |

**Triggers.** *Reporting: Report Generated* (`reporting.read`) starts a
workflow whenever a report is generated, by a person, a schedule or a
workflow node, with the report summary in `msg.payload`; filter by report
type, environment, who generated it, and whether it was saved to the
library. *Reporting: Schedule Ran* (`reporting.schedule`) starts one when a
saved schedule finishes a run, succeeded or failed, with the run outcome
and delivery results in `msg.payload`; filter by schedule and outcome.

The base64 file payload is shaped for the storage-upload and
communication nodes that follow it in a flow. The ad-hoc (AI) type is
deliberately not offered: it needs a prompt and an AI connection, which
is a studio interaction.

**Who the report runs as.** Collection reads the platform over the host
API as a user (see *How it reads the platform*). A run started by hand
from the editor carries the account that pressed Run, and the generate
node mints a session for it the way a schedule does for its author.
Scheduled, webhook and event-triggered runs carry nobody; with
authentication enabled, sources that need a session then report "not
permitted" in the Methodology annex. For unattended reporting, save a
schedule and drive it with *Run Schedule* instead — that always runs as
the schedule's author, with the schedule's delivery targets.

## Building

```bash
# Backend — typechecks against the host module via go.work
cd src/plugins/Plugins/reporting
go vet ./backend/...

# Frontend — emits ../frontend/reporting.umd.cjs
cd frontend
npm install --legacy-peer-deps
npm run build

# Full image (the Dockerfile builds and packages the plugin)
cd ../../../../..
docker compose build && docker compose up -d
```

Then install it from **App Store → Plugins → Reporting**.

---

## Permissions

| Key | Grants |
|---|---|
| `reporting.read` | View the catalog, the library and generated reports |
| `reporting.generate` | Generate new reports |
| `reporting.manage` | Delete reports, change reporting settings |

`reporting.generate` does not widen what a user can see. The report is
collected as that user; the permission only controls whether they may
ask for one.

---

## Settings

Organisation name and classification label (both appear on the cover),
default period, how many reports to retain, and an opt-in AI narrative.

The AI narrative is **off by default**. When enabled it may only improve
the wording of the opening paragraph: the prompt forbids changing any
number, and the result is rejected if a figure from the deterministic
text does not survive verbatim. Any failure falls back to the standard
wording. A board report has to be reproducible, and the deterministic
text always is.

---

## Licences

Plugin code: Apache-2.0 (see `LICENSE`).

Bundled fonts, both SIL Open Font License 1.1 — see
`backend/assets/fonts/OFL.txt`:

- **Inter Tight** — Copyright (c) The Inter Project Authors
- **JetBrains Mono** — Copyright (c) The JetBrains Mono Project Authors

The OFL permits embedding in documents and bundling with software.
