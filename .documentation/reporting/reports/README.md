# Reporting: Report catalog

Every report type the Report Studio can offer, where it comes from, and what it reads.

A built-in type ships with the Reporting plugin. A contributed type ships with the plugin that
owns its data and registers through the host's `reporting` adapter family. It appears in the
studio only while that plugin is installed. Both kinds are generated, scheduled, stored and
exported in the same way.

**Levels** are the altitudes a type can produce: **S** strategic, **T** tactical, **O**
operational. **Period** is the window the studio pre-selects. "Point in time" means the report
describes the present and the studio hides the period picker.

Generating any report needs `reporting.generate`. A built-in type reads the platform over the
host's own API **as the generating user**, so each data source it reads needs that user's own
permission. A source the user cannot read is listed as "not assessed" in the report's
Methodology annex instead of being scored. A report can never show a user more than the
interface already shows them.

## Built-in types

### For the board and custom

| Key | Name | Levels | Period |
|---|---|---|---|
| `executive-overview` | Executive Platform Overview | S T O | 30 days |
| `ad-hoc` | AI generated report | S T O | 30 days |

### Assurance

| Key | Name | Levels | Period |
|---|---|---|---|
| `security-compliance` | Security & Compliance | S T O | 30 days |
| `incident-problem` | Incident & Problem Review | S T O | 30 days |
| `backup-recovery` | Backup & Recovery | T O | 30 days |
| `access-review` | Access & Identity Review | T | 90 days |
| `user-activity` | User Activity | T O | 30 days |
| `permission-matrix` | Role & Permission Matrix | T O | Point in time |
| `credential-hygiene` | Credential Hygiene | T O | 90 days |
| `audit-assurance` | Audit Trail Assurance | T O | 30 days |
| `incident-narrative` | Incident Narrative Review | S T O | 30 days |
| `instance-dr-readiness` | Instance Disaster Recovery Readiness | S T O | Point in time |
| `change-approvals` | Change Approvals & Privileged Access | S T O | 30 days |
| `secrets-hygiene` | Secrets Hygiene | S T O | Point in time |
| `compliance-posture` | Compliance Posture | S T O | Point in time |
| `control-framework-evidence` | Control Framework Evidence | S T O | Point in time |
| `runbooks` | Runbooks | T O | Point in time |

### Service performance

| Key | Name | Levels | Period |
|---|---|---|---|
| `availability-slo` | Availability & Service Levels | S T | 30 days |
| `change-delivery` | Change & Delivery | S T | 30 days |
| `environment-comparison` | Environment Comparison | S T | 30 days |
| `data-protection-rpo` | Data Protection & RPO | S T O | 30 days |
| `itsm-integration` | ITSM Integration | T O | 30 days |

### Platform

| Key | Name | Levels | Period |
|---|---|---|---|
| `environment-health` | Environment Health | T O | 7 days |
| `capacity-utilisation` | Capacity & Utilisation | T O | 30 days |
| `image-supply-chain` | Image & Supply Chain | T O | 30 days |
| `workload-inventory` | Workload Inventory | O | Point in time |
| `platform-inventory` | Platform Inventory | O | Point in time |
| `ha-posture` | High Availability Posture | S T O | Point in time |

### Workload depth

| Key | Name | Levels | Period |
|---|---|---|---|
| `uptime-availability` | Uptime & Availability History | S T O | 30 days |
| `container-service` | Container Service Report | T O | 30 days |
| `healthcheck-quality` | Health Check Quality | T O | 7 days |
| `stability-flapping` | Stability & Flapping | T O | 30 days |
| `stack-portfolio` | Stack & Application Portfolio | T O | 30 days |
| `operational-readiness` | Operational Readiness | S T O | 30 days |

### Operations

| Key | Name | Levels | Period |
|---|---|---|---|
| `patch-currency` | Patch & Update Currency | S T O | 30 days |
| `automation-schedules` | Scheduled Work & Automation | T O | 30 days |
| `alerting-coverage` | Alerting Coverage | T O | 30 days |
| `agent-fleet` | Remote Agent Fleet | T O | Point in time |
| `network-connectivity` | Network & Connectivity | T O | Point in time |
| `volume-data-estate` | Volume & Data Estate | T O | Point in time |
| `drift-enforcement` | Configuration Drift & Enforcement | S T O | 30 days |
| `workflow-automation` | Workflow Automation | T O | 30 days |
| `webhook-delivery` | Webhook Delivery | T O | 7 days |

### Cost and efficiency

| Key | Name | Levels | Period |
|---|---|---|---|
| `resource-efficiency` | Resource Efficiency | S T O | 30 days |
| `cost-allocation` | Cost Allocation & Showback | S T O | 30 days |

## What the governance and resilience reports read

These ten types read features that each have their own permission. The table lists the key
the generating user needs for a complete report. A missing key never blocks generation: it
leaves that part "not assessed".

| Type | Reads | Needs |
|---|---|---|
| `incident-narrative` | `/api/incidents`, `/api/incidents/findings`, `/api/incidents/sources` | `incidents.view` |
| `drift-enforcement` | `/api/reconciler`, `/api/reconciler/report`, `/api/reconciler/proposals`, `/api/reconciler/settings` | `reconciler.view` |
| `instance-dr-readiness` | `/api/instance-backups`, `…/schedules`, `…/restore/status`, `…/restore/ledger` | `instance_backup.view` |
| `ha-posture` | `/api/cluster/status`, `/api/cluster/readiness`; failover history from `/api/audit` | `cluster.view`, `audit.view` |
| `itsm-integration` | `/api/itsm/links`, `/api/itsm/channels` | `alerts.view`, `communications.view` |
| `change-approvals` | `/api/approvals`, `…/rules`, `…/categories`; `/api/elevations/all` | `approvals.view`; `users.manage` for elevations |
| `secrets-hygiene` | `/api/secrets`, `…/{id}/versions`, `…/{id}/impact` | `secrets.view` |
| `workflow-automation` | `/api/workflows`, `/api/workflows/runs` | `workflows.view` |
| `webhook-delivery` | `/api/webhooks`, `/api/webhooks/{id}/deliveries` | `webhooks.view` |
| `compliance-posture` | `/api/compliance/posture?redact=1` | `compliance.view` |
| `control-framework-evidence` | `/api/compliance/packs`, `/api/compliance/packs/runs/{id}` | `compliance.view` |
| `runbooks` | `/api/runbooks`, `/api/runbooks/runs`, `/api/alerts` | `runbooks.view`, `alerts.view` |

What these reports never read or print:

- **Secrets Hygiene** reads metadata only: name, scope, age, versions and references. It never
  calls a reveal endpoint, so no secret value can reach a document.
- **Webhook Delivery** prints an endpoint as scheme and host only. It never prints the path,
  the query string, the signing secret, the payload or the response body.
- **Instance Disaster Recovery Readiness** never calls the export download or the key-file
  routes.
- **Configuration Drift & Enforcement** prints environment-variable values as "(withheld)".
- **Compliance Posture** asks for the redacted posture, and a fact marked sensitive always
  prints as "Redacted".
- **Workflow Automation** does not read workflow definitions, variables or run error text,
  because they can hold credentials.
- **Runbooks** prints a step's title, body and the key of the diagnostic or action it invokes.
  It never prints that step's action **parameters**, which can name secrets and container ids,
  and never the automation principal's id.

Per-webhook and per-secret reads run at most four at a time. Each group is recorded as one line
in the Methodology annex. A read capped at the host's page size says that it may be incomplete.

## Cross-platform sections in built-in types

Sixteen built-in types print an **Across platforms** section: one table with a Platform column
covering Docker and every installed platform plugin. The facets, which report takes which, and
what a missing platform looks like are on [Cross-platform facets](../facets/README.md).
`workload-inventory` was called `container-inventory` until 2026.09.01, and it lists every
platform's workloads now. Schedules and stored documents under the old key were moved over
when the Reporting plugin was updated.

## Contributed types

| Key | Name | Plugin | Shelf | Levels | Period |
|---|---|---|---|---|---|
| `kvm-backup-assurance` | Virtual machine backup assurance | `addon-kvm` | Assurance | S T O | 30 days |
| `kvm-guest-stability` | Virtual machine guest stability | `addon-kvm` | Operations | T O | 30 days |
| `kvm-snapshot-schedules` | Virtual machine snapshot schedules | `addon-kvm` | Assurance | T O | 30 days |
| `lxc-isolation` | System container isolation | `addon-lxc` | Assurance | S T | Point in time |
| `compute-readiness` | Compute host readiness | `addon-compute` | Assurance | O | Point in time |
| `compute-image-lease-hygiene` | Compute image and lease hygiene | `addon-compute` | Operations | T O | Point in time |
| `kubernetes-reliability` | Kubernetes Reliability Review | `addon-kubernetes` | Kubernetes | S T O | Point in time |
| `kubernetes-storage-estate` | Kubernetes Storage Estate | `addon-kubernetes` | Kubernetes | T O | Point in time |
| `kubernetes-network-exposure` | Kubernetes Network Exposure | `addon-kubernetes` | Kubernetes | S T O | Point in time |
| `swarm-capacity-placement` | Swarm Capacity and Placement | `addon-swarm` | Swarm | T O | 30 days |
| `logging-siem-delivery` | SIEM log forwarding delivery | `logging-siem` | Operations | S T O | 7 days |
| `logging-syslog-delivery` | Syslog forwarding delivery | `logging-syslog` | Operations | S T O | 7 days |

Twelve contributed types that duplicated the shared reports no longer exist (`kvm-inventory`,
`lxc-inventory`, `compute-inventory`, `kubernetes-workload-inventory`,
`swarm-service-inventory`, `kvm-protection`, `lxc-protection`, `compute-capacity`,
`lxc-utilisation`, `kvm-allocation`, `kubernetes-cluster-health`, `swarm-cluster-health`). The
platforms now answer the shared reports through facets, and the table on
[Cross-platform facets](../facets/README.md) names the replacement for each.

A contributed type reads the plugin's own data, not the host API. The two log-forwarding
reports check the same pair of keys that guard the Logging tab's delivery history,
`audit.view` and `settings.manage`, so a report cannot be used to get around them.

### What the newer contributed types cover

- **Virtual machine backup assurance**: plan coverage per guest and guests with no plan, run
  success rate in the period, the age of the last good backup, copies each destination received
  or missed, and the health of each disk's incremental chain.
- **Virtual machine guest stability**: crashes, unexpected stops and resets, guests that flap
  (five unintended downs in the period, or three within 30 minutes), guest-agent health, and
  lifecycle actions that failed or have been pending for more than 30 minutes. Event history is
  kept for 30 days, so a longer period says the start is missing.
- **Virtual machine snapshot schedules**: firings due by cron against snapshots actually taken,
  missed and failed firings, and schedules keeping more snapshots than their limit. A disabled
  schedule owes nothing, because the time it was disabled is not recorded.
- **Compute image and lease hygiene**: image cache state per host (ready, in progress, stuck
  for more than 6 hours, failed), images no host holds a ready copy of, and DHCP leases that are
  stale (no update for 7 days), expired or orphaned. A failed download's raw error is not
  printed, because it can name source URLs and host paths.
- **SIEM / Syslog forwarding delivery**: delivered against failed per forwarder, success rate
  (a finding below 95%, high below 80%), the longest gap with no successful delivery,
  forwarders silent for more than 24 hours, and recurring failure reasons grouped. The syslog
  report warns that UDP success is not proof of delivery.

## Option labels

A report's options appear as checkboxes in the studio and the schedule editor. A contributed
type's option is labelled from `reports.options.<key>` in the contributing plugin's own i18n
namespace. That namespace is the prefix of its `LabelKey` (`vm:` for `addon-kvm`), not its slug.
An option with no label there falls back to the Reporting plugin's shared `option.<key>`, which
is how `include_detail` and `include_glossary` are labelled for Kubernetes and Swarm.


## Runbooks, and printing one on its own

The `runbooks` type answers two questions with one document, and which one depends on its
scope.

**Unscoped** it is the coverage picture: how many runbooks exist, what they attach to, which
run unattended, and (the part worth reading first) **which alert conditions can fire with no
runbook behind them**. An alert that wakes somebody with nothing written down is the gap this
report was added to make visible.

**Scoped to one runbook** it is that runbook as a printable procedure: what it is for, what it
attaches to, whether it runs unattended and on what, then every step in order with its body
and the diagnostic or action it invokes, followed by that runbook's recent runs.

Runbooks → the row's export control offers **Export as PDF** and **Export as Word document**
beside the JSON export, and both go through this report. The two document formats appear only
while the Reporting plugin is installed and loaded and the operator holds
`reporting.generate`; without either the control is the JSON export alone, because a menu
entry that cannot work is worse than a menu with one item on it.

A document exported this way is a normal report: it is kept in the report library, and it can
be scheduled and delivered like any other.

## Related

- [Reporting](../README.md): the plugin, its permissions and its pages
- [Developer](../../development/add-ons/reporting.md): how a built-in family and a contributed type are added
- [Build a reporting plugin](../../how-to/reporting/README.md)
