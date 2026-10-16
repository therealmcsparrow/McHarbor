# Evidence for ISO/IEC 27001

This page maps the ISO/IEC 27001:2022 Annex A controls that McHarbor holds evidence for to
**where that evidence is**: the exact route, table or setting. It is evidence for a
certification audit. It is not a certification, McHarbor is not a certification body, and
nothing here is an opinion on whether a control is implemented effectively.

**There is no check runner for ISO 27001.** Annex A is a control set about policy, people,
process and technology, and the technology half that McHarbor can see overlaps heavily with
what the [CIS Docker Benchmark pack](../cis-docker-benchmark/README.md) checks mechanically.
Rather than duplicate that, this page cites it.

Annex A control numbers are the 2022 revision's.

## Organisational controls (A.5)

| Control | What an auditor asks | Evidence in McHarbor | Verdict |
|---|---|---|---|
| A.5.15 Access control | Access is granted on a need-to-know basis, per a defined rule | `GET /api/auth/permissions/catalog` (`permissions.view`): every permission key with its category, action, description and scope. Role definitions in `roles`; assignments per user and group | Evidenced |
| A.5.16 Identity management | Identities are managed through their full lifecycle | `users`; `identity_providers` for federated identity; `scim_endpoints` for provisioning and deprovisioning, with each endpoint's last sync status | Evidenced |
| A.5.17 Authentication information | Credentials are handled securely | Password policy and MFA adoption from `GET /api/compliance/posture` (`compliance.view`); secrets are AES-256-GCM encrypted at rest, with rotation age per secret in `secret_versions` | Evidenced |
| A.5.18 Access rights | Access rights are provisioned, reviewed and revoked | `GET /api/access-review/effective-permissions` (`permissions.view`, global) for provisioning and revocation. The review is recorded by `POST /api/compliance/attestations` (`compliance.attest`, global) with framework `access-review`, and read back by `GET /api/compliance/attestations` (`compliance.view`). See [Attestations](../attestations/README.md) | Evidenced |
| A.5.23 Cloud services security | Use of cloud services is governed | Configured environments and their connection kind; storage locations and their destinations | Evidenced |
| A.5.28 Collection of evidence | Evidence is collected and preserved | `audit_logs` with the retention policy in effect; `compliance_pack_results`. **Read [Evidence integrity](../evidence-integrity/README.md) before citing preservation** | Partly evidenced |
| A.5.30 ICT readiness for continuity | Continuity is planned and tested | `instance_backup_schedules`, and `instance_backup_exports` with each run's status **and verification status** | Evidenced |

## People controls (A.6)

| Control | What an auditor asks | Evidence in McHarbor | Verdict |
|---|---|---|---|
| A.6.x | Screening, terms, awareness, discipline | **Not covered.** These are HR process controls with no technical state in McHarbor. Deprovisioning on termination is A.5.16 and is evidenced there | **Not covered** |

## Physical controls (A.7)

| Control | What an auditor asks | Evidence in McHarbor | Verdict |
|---|---|---|---|
| A.7.x | Perimeters, entry, equipment, media | **Not covered.** McHarbor has no view of a facility, and no view of its own host's filesystem or hardware custody | **Not covered** |

## Technological controls (A.8)

| Control | What an auditor asks | Evidence in McHarbor | Verdict |
|---|---|---|---|
| A.8.2 Privileged access rights | Privileged access is restricted and controlled | `elevations` records each temporary elevation, its scope and window; `approval_requests` records what was held pending a decision. The posture reports the administrator count | Evidenced |
| A.8.5 Secure authentication | Authentication is appropriate to the risk | MFA adoption and the password policy from the posture; `identity_providers` for federated authentication | Evidenced |
| A.8.7 Protection against malware | Malicious code is detected | `scans`: per-image vulnerability scan results from the installed scanner plugin. **Image scanning is not malware detection**; state which of the two the control needs | Partly evidenced |
| A.8.8 Management of technical vulnerabilities | Vulnerabilities are identified and remediated | `scans` per image, with severity counts; the reconciler's drift records for configuration | Evidenced |
| A.8.9 Configuration management | Configurations are established, documented and reviewed | `compliance_pack_results`: the CIS Docker Benchmark pack, mechanically checked against each daemon. Desired state and drift in the reconciler | Checked mechanically |
| A.8.12 Data leakage prevention | Data leaving the system is controlled | **Not covered** as a technical control. Secrets are encrypted at rest and never returned in API responses, and socket paths, kubeconfigs, bearer tokens and agent tokens are redacted, but that is control of McHarbor's own surface, not DLP | **Not covered** |
| A.8.13 Information backup | Backups are taken and tested per policy | `instance_backup_schedules` and `instance_backup_exports`, including `verify_status` | Evidenced |
| A.8.15 Logging | Events are logged, kept and protected | `audit_logs` with the active retention policy. **Protection is conditional**: [Evidence integrity](../evidence-integrity/README.md) states every condition | Partly evidenced |
| A.8.16 Monitoring activities | Systems are monitored for anomalous behaviour | `alert_rules` and their firing history; `incidents` with correlated evidence; retained metrics history | Evidenced |
| A.8.20 Networks security | Networks are managed and controlled | The CIS pack's 2.1 (inter-container communication on the default bridge) and 5.9 (host network namespace), both checked mechanically | Checked mechanically |
| A.8.22 Segregation of networks | Networks are segregated | Docker networks per environment, and CIS 2.1 | Partly evidenced |
| A.8.25 Secure development lifecycle | Development follows a secure lifecycle | **Not covered** for your own software. For McHarbor's own artifacts, the posture's supply-chain section records the SBOM format and signing method | **Not covered** |
| A.8.28 Secure coding | Code is written securely | **Not covered.** A process control | **Not covered** |
| A.8.31 Separation of environments | Development, test and production are separated | Configured environments, and RBAC scoped per environment. Whether the separation is meaningful is a judgement about your configuration, not something McHarbor can verify | Partly evidenced |
| A.8.32 Change management | Changes are controlled | `approval_requests` and `approval_decisions`; `audit_logs` for every mutating action, with the actor | Evidenced |

## Clause 9.2 and 9.3: internal audit and management review

**Evidenced** through `POST /api/compliance/attestations` (`compliance.attest`, global), and
read back through `GET /api/compliance/attestations` (`compliance.view`). An attestation
records who signed off, on which framework and (for a pack) which run, when, and with what
note. Use framework `iso-27001` for a management-review sign-off on this mapping as a whole,
`access-review` for an access review, or a pack key such as `cis-docker-benchmark` with the
run it accepts. Attestations cannot be edited or deleted: a later one supersedes an earlier
one, and both read back. See [Attestations](../attestations/README.md).

McHarbor supplies the **input** to an internal audit (effective permissions with provenance,
the catalog export, the audit trail, the pack results) and now the **record** that a named
person reviewed it. What it still does not supply is the audit programme itself, and role
assignment and group membership still carry no attribution or history beyond generic
`audit_logs` rows.

## Permissions this pack's evidence needs

| Key | Reaches |
|---|---|
| `permissions.view` | The effective-permission review and the catalog export |
| `approvals.view` | Approval requests and decisions |
| `audit.view` | The audit log and the hash-chain verification result |
| `audit.export` | The audit export and its metadata |
| `scans.view` | Image scan results |
| `compliance.view` | The posture, and every compliance pack's mapping and recorded runs |
| `compliance.manage` | Starting a CIS Docker Benchmark check run |
| `compliance.attest` | Recording a sign-off on a pack run, the access review or this mapping |
