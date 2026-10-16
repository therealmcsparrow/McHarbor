# Compliance packs

A compliance pack is **a mapping from a named control framework to the evidence McHarbor
already produces, plus the checks it can run mechanically.** It is evidence for a framework.
It is not a claim of compliance with one, and nothing in McHarbor certifies, attests to or
audits an install against any standard. Where a control is met is a judgement for you and
your auditor; a pack's job is to make the evidence for that judgement easy to produce, and to
be honest about what it cannot see.

Read that as a hard boundary, not modesty. Three of the packs' most-asked-about controls turn
out to be weaker in a default install than the marketing shape of "hash-chained, signed,
write-once audit log" suggests, and [Evidence integrity](./evidence-integrity/README.md)
spells out exactly which and why. A pack that glossed over those would be worse than no pack.

## The three parts

A pack is deliberately three separate things:

| Part | What it is | Where |
|---|---|---|
| **The mapping** | One row per control: what it requires, which McHarbor evidence answers it, and whether that answer is checked, evidenced or absent | The pages in this folder |
| **The check runner** | The mechanical half: checks that read live state and record a verdict, on a schedule and on demand | `modules/compliancepacks` |
| **The report** | The pack as a document, with the not-assessed controls named on the first page | The Reporting add-on |

Only the CIS Docker Benchmark has a check runner. SOC 2 and ISO 27001 have no mechanical
state to read. They are evidence-collection exercises about who has access, who approved
what and when it was reviewed, so shipping a stub check suite for either would imply a
verdict McHarbor is not producing.

## The packs

| Pack | Check runner | Covers |
|---|---|---|
| [CIS Docker Benchmark](./cis-docker-benchmark/README.md) | Yes: 31 controls checked against a live daemon | Daemon configuration, container runtime settings, Swarm state |
| [SOC 2](./soc2/README.md) | No: evidence mapping only | Logical access, change approval, monitoring, availability |
| [ISO/IEC 27001](./iso-27001/README.md) | No: evidence mapping only | Annex A controls McHarbor holds evidence for |
| [Evidence integrity](./evidence-integrity/README.md) | — | What the audit log's tamper-evidence does and does not prove, per configuration |
| [Attestations](./attestations/README.md) | — | The sign-off record: who accepted which framework's evidence, on which run, when |

## The three verdicts a control can carry in a mapping

Every row in every mapping is one of these, and the third is the reason the documents are
worth reading.

- **Checked mechanically**: a check reads live state and records `pass`, `warn`, `fail`,
  `info` or `unknown`. A passing check means **the check passed**, not that the control is
  met.
- **Evidenced from recorded data**: McHarbor holds the evidence an auditor would ask for and
  names where it is, but does not judge it.
- **Not covered**: McHarbor cannot see the control at all. Host filesystem permissions,
  auditd rules on the Docker host, and every organisational process control fall here.

## Verdict vocabulary

The same five verdicts as the compliance posture, and for the same reason: one vocabulary
across the dashboard, the packs and the reports.

| Verdict | Means |
|---|---|
| `pass` | The check ran and found nothing to report |
| `warn` | The check found something worth attention |
| `fail` | The check found a violation |
| `info` | The control describes rather than judges, or McHarbor cannot cover it |
| `unknown` | The check should have had an answer and did not; the evidence could not be gathered |

**`unknown` is not a pass.** It outranks `info` in the rollup, so a run with nine passing
checks and one unreachable one does not report as passing. A check that cannot run reports
`unknown` with a reason code; it never falls back to `pass`.

## Permissions

| Key | Grants |
|---|---|
| `compliance.view` | Reading the packs, their mappings, every recorded run and its control results |
| `compliance.manage` | Starting a check run |
| `compliance.attest` | Recording a sign-off on a pack run, the access review or a mapping |

`compliance.view` is the same key the compliance posture dashboard uses. `compliance.manage`
is separate because a run reaches out to a live Docker daemon, which reading a computed
posture never does. `compliance.attest` is separate from both, so that starting a check and
accepting its result are two authorities. All three are `global` scope: a pack is an
instance-wide posture even when one of its checks runs per environment.

## Routes

As mounted:

| Route | Key | Returns |
|---|---|---|
| `GET /api/compliance/packs` | `compliance.view` | Every pack with its controls, its evidence sources and its latest run |
| `GET /api/compliance/packs/{framework}/runs` | `compliance.view` | Recorded runs, newest first, paginated |
| `GET /api/compliance/packs/runs/{id}` | `compliance.view` | One run with its per-control results |
| `POST /api/compliance/packs/{framework}/run?env={id}` | `compliance.manage` | Starts a run and returns its ID |
| `GET /api/compliance/attestations?framework={key}` | `compliance.view` | Sign-offs, newest first, paginated; superseded ones included |
| `POST /api/compliance/attestations` | `compliance.attest` | Records a sign-off; there is no edit or delete route |

A run is recorded before any check executes and the sweep continues after the response, so
the answer to `POST` is a run ID to watch rather than a wait. A run that crashes stays visible
as `running`; a run that fails records its message.

## Retention

Runs and their results are kept for **365 days** and pruned by the scheduled sweep. The
window is a year rather than the days a metrics series keeps, because the question a pack
exists to answer ("was this check passing last month") needs history.

## What a pack does not do

- It does not certify anything, and no string in the product says it does.
- It does not remediate. A failing check names the control and the containers involved; the
  change is yours to make.
- It does not read the Docker host's filesystem, so no control about file ownership,
  permissions or partitioning can be answered. Those rows say *not covered*.
- It does not decide that a control is met. A person does, and records it as an
  [attestation](./attestations/README.md) under their own name, with `compliance.attest`,
  which is deliberately a different key from the one that starts a run.
