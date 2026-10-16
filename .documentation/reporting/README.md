# Reporting

Generates board-presentable reports on environments, workloads, incidents, security, capacity, delivery and recovery. Reports are organised by management altitude (strategic, tactical, operational), readable inside McHarbor, and exportable to PDF and Word with the product's own typography.

- **Slug** `addon-reporting`, **version** 2026.09.01
- **Host range** `>=2026.09.01`
- **Where it appears** its own section in the sidebar, and its own pages under `/plugins/<slug>`
- **Gated by** the permission keys it declares (see below)

## How it works

- This is the largest plugin in the repository by a wide margin, and the only one that both consumes and hosts an adapter family: other plugins contribute report sections to it through `AdapterRegistry.Reporting()`, and it contributes alert conditions of its own through `AdapterRegistry.Alerting()`.
- Seven migrations build up its schema under the `plugin_reporting_` prefix: reports, ad-hoc authoring, the AI connection, three rounds of schedule and delivery-target changes, and the one that moved the removed platform report types onto the shared reports.
- Scheduled reports deliver through the host's communication channels, so a channel plugin (SMTP, for example) is what turns a schedule into something that arrives in an inbox.
- Report types come from three places: the original built-in set, two built-in *families* that read host features (incidents, drift, instance DR, HA and ITSM; approvals, secrets, workflows, webhooks and compliance), and types contributed by the plugins that own the data: KVM, LXC, Compute, Kubernetes, Swarm and the two log forwarders. The [report catalog](./reports/README.md) lists every one.
- What runs on the estate is not a per-platform report any more. Docker, KVM, LXC, Compute, Kubernetes and Swarm each answer six cross-platform questions (*facets*), and sixteen shared reports print the answers in one table with a Platform column. See [Cross-platform facets](./facets/README.md).
- Four permission keys split the work deliberately: `reporting.read` to open one, `reporting.generate` to run one, `reporting.schedule` to automate it, `reporting.manage` for the definitions.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `HasTarget` | Yes |  |
| `SupportsFiles` | Yes |  |

## What it contributes

| | |
|---|---|
| Routes | 22 |
| Sidebar entries | 4 |
| Dashboard widgets | `recent-reports`, `report-schedules` |
| Workflow nodes | `report-generate`, `report-export`, `report-schedule-run`, `report-generated`, `report-schedule-ran` |
| Migrations | `0001_init.sql`, `0002_adhoc.sql`, `0003_ai_connection.sql`, `0004_schedules.sql`, `0005_schedule_target_path.sql`, `0006_schedule_target_recipient.sql`, `0007_facet_report_keys.sql` |
| Backend | 51 Go files, ~27,560 lines |

## Permissions it adds

| Key | Scope | What it allows |
|---|---|---|
| `reporting.read` | env | View reports |
| `reporting.generate` | env | Generate reports |
| `reporting.manage` | env | Manage reporting |
| `reporting.schedule` | env | Schedule reports |

## In this section

| Page | What it covers |
|---|---|
| [Report catalog](./reports/README.md) | Every report type, its levels and period, what it reads and which permission it needs |
| [Cross-platform facets](./facets/README.md) | The six facets every platform answers, which shared reports print them, what a missing platform looks like, and the twelve removed report types |
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |
| [Developer](../development/add-ons/reporting.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Add-ons](../plugins/addons.md): what the six have in common, and what the plugin runtime does with them
- [Plugin catalog](../plugins/catalog.md): every shipped plugin
