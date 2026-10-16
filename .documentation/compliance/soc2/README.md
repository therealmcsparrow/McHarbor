# Evidence for SOC 2

This page maps the SOC 2 Trust Services Criteria that McHarbor holds evidence for to **where
that evidence is**: the exact route, table or setting. It is evidence for an audit. It is not
a SOC 2 report, McHarbor is not a service auditor, and nothing here constitutes an opinion on
whether a control is operating effectively.

**There is no check runner for SOC 2.** Every criterion below is an evidence-collection
exercise about who has access, who approved what, and when it was reviewed; there is no live
daemon state to read a verdict from. Shipping a stub check suite would imply a verdict McHarbor
is not producing. The CIS Docker Benchmark pack is the one with mechanical checks.

## Where the evidence sits

Criteria are cited by their common Trust Services reference. Routes are written as mounted, and
every one names the permission key it needs.

### CC6: Logical and physical access

| Criterion | What an auditor asks | Evidence in McHarbor | Verdict |
|---|---|---|---|
| CC6.1 | Who can do what, and how is it granted | `GET /api/access-review/effective-permissions` (`permissions.view`, global) computes a user's effective permissions with provenance: the role, group and environment each one came from. `GET /api/access-review/permission-catalog/export` exports the catalog with the roles holding each key | Evidenced |
| CC6.1 | The permission model itself | `GET /api/auth/permissions/catalog` (`permissions.view`) returns every key with its category, action, description and scope. The catalog is code-first and mirrored into `permission_catalog` at startup | Evidenced |
| CC6.2 | Access is registered and authorised before it is granted | `approval_rules`, `approval_requests` and `approval_decisions` record a held change, who decided it and when. `GET /api/approvals` (`approvals.view`) reads them | Evidenced |
| CC6.2 | When access was granted, and by whom | **Thin.** `user_permissions.granted_by` is the only grant-attribution column in the schema. **Role assignment and group membership have no attribution and no history**; the only trail is generic `audit_logs` rows | Partly evidenced |
| CC6.3 | Access is modified and removed on a timely basis | `audit_logs` records the change. SCIM deprovisioning is recorded per endpoint in `scim_endpoints` | Evidenced |
| CC6.3 | Access is **reviewed** periodically, and the review is evidenced | `POST /api/compliance/attestations` (`compliance.attest`, global) records a sign-off on the access review (framework `access-review`, the reviewer, the time and a required note) in `compliance_attestations`. `GET /api/compliance/attestations?framework=access-review` (`compliance.view`) reads every sign-off back, superseded ones included. See [Attestations](../attestations/README.md) | Evidenced |
| CC6.6 | Authentication strength | `GET /api/compliance/posture` (`compliance.view`) reports the password policy and MFA adoption across active local accounts, with the facts behind each verdict | Evidenced |
| CC6.6 | Transport protection | The posture's TLS section reports whether TLS is on, whether the certificate is self-signed or uploaded, its expiry and the minimum protocol version | Evidenced |
| CC6.7 | Credential and secret handling | `secrets` and `secret_versions` record rotation age per secret; the posture reports encryption-key provenance and whether a backup key is configured | Evidenced |

### CC7: System operations

| Criterion | What an auditor asks | Evidence in McHarbor | Verdict |
|---|---|---|---|
| CC7.1 | Configuration is monitored for unauthorised change | The reconciler records desired state and drift per object | Evidenced |
| CC7.1 | Infrastructure is checked against a hardening standard | `compliance_pack_results`: the [CIS Docker Benchmark pack](../cis-docker-benchmark/README.md), which is mechanically checked | Checked mechanically |
| CC7.2 | Anomalies are detected and evaluated | `alert_rules` and their firing history; `incidents` with correlated evidence per failing container | Evidenced |
| CC7.2 | Vulnerabilities in what runs are identified | `scans`: per-image scan results from the installed scanner plugin | Evidenced |
| CC7.3 | Incidents are evaluated to determine whether a failure occurred | `incidents` with post-incident timelines | Evidenced |
| CC7.4 | Incidents are responded to and resolved | `incidents` status transitions, plus ticket links in `itsm_links` where an ITSM plugin is installed | Evidenced |
| CC7.5 | Recovery from an incident is verified | `instance_backup_exports` records each export's status **and its verification status**, so a restore rehearsal is evidence and not an assertion | Evidenced |

### CC8: Change management

| Criterion | What an auditor asks | Evidence in McHarbor | Verdict |
|---|---|---|---|
| CC8.1 | Changes are authorised before they are made | `approval_requests` and `approval_decisions`, with `approver_id` indexed. A held destructive request is replayed through the same chain a live request takes, so the approval and the action are the same event | Evidenced |
| CC8.1 | Changes are recorded | `audit_logs` for every mutating action, with the actor | Evidenced |
| CC8.1 | Emergency change is controlled and reviewed | `elevations` records each temporary privilege elevation, its scope and its window | Evidenced |

### A1: Availability

| Criterion | What an auditor asks | Evidence in McHarbor | Verdict |
|---|---|---|---|
| A1.2 | Backups are taken and recoverable | `instance_backup_schedules` for the schedule, `instance_backup_exports` for each run's status and verify status | Evidenced |
| A1.2 | Capacity is monitored | Retained metrics history per host and environment | Evidenced |
| A1.3 | Recovery procedures are tested | `instance_backup_exports.verify_status`; a verified export is a tested one | Evidenced |

## The audit-trail criteria need the integrity page

Several criteria above rest on `audit_logs` being tamper-evident. That claim is conditional on
configuration in five separate ways, and **[Evidence integrity](../evidence-integrity/README.md)
states each one**. In particular: the chain is an HMAC and seals nothing when `MCHARBOR_SECRET`
is unset, the export signature cannot be verified by an external auditor, write-once ships off,
and a plain install forwards nothing to a SIEM.

Cite that page alongside any audit-trail criterion rather than citing the log alone.

## CC6.3: the review's input and its record

The product computes what an access review needs to see (effective permissions with their
provenance), and reports measure sensible proxies: dormant accounts past 90 days, stale keys,
whether there are more than three administrators.

The **record** of the review is an attestation: a named person, signing off on the access
review at a recorded time, with a note saying what they accepted. It is written from
**Security → Effective access → Access review sign-off**, or with `mcharbor compliance attest
access-review --note …`, and it cannot be edited or deleted. A later review supersedes it and
both stay on record. [Attestations](../attestations/README.md) describes the record in full.

What an attestation does not do: it does not freeze the permissions the reviewer looked at.
The access review is computed live, so export the catalog (`GET
/api/access-review/permission-catalog/export`) at review time and name the export in the note
if the auditor wants the snapshot itself. Role assignment and group membership still have no
attribution or history beyond generic `audit_logs` rows, which is CC6.2's gap above.

## Permissions this pack's evidence needs

| Key | Reaches |
|---|---|
| `permissions.view` | The effective-permission review and the catalog export |
| `approvals.view` | Approval requests and decisions |
| `audit.view` | The audit log and the hash-chain verification result |
| `audit.export` | The audit export and its metadata |
| `compliance.view` | The posture, and every compliance pack's mapping and recorded runs |
| `compliance.manage` | Starting a CIS Docker Benchmark check run |
| `compliance.attest` | Recording a sign-off on a pack run or on the access review |
