# Evidence integrity: what the audit log does and does not prove

Every compliance framework asks the same question about an audit trail: **can it be changed
without anyone noticing?** McHarbor's answer is "partly, and it depends entirely on how the
install is configured", and this page exists because summarising it as "hash-chained, verified,
signed, write-once, forwardable" would be misleading in five separate ways.

Read this page before citing McHarbor's audit log as evidence for a control. A pack that
asserted a capability the install does not have would be worse than no pack: it would move an
unanswered question into the "answered" column of somebody's audit.

## The summary table

| Commonly stated as | What the code actually does | Where |
|---|---|---|
| "SHA-256 hash-chained" | **HMAC-SHA256, keyed** with the instance secret. That is a message authentication code, not a bare digest: whoever holds the key can recompute any row's hash | `core/audit/hash_chain.go` `ComputeRowHash`; the chain columns are in `core/db/migrations/072_audit.sql` |
| implied always on | **Nothing is sealed at all when `MCHARBOR_SECRET` is unset.** The verifier reports `KeyConfigured: false`, escalates its own audit row to severity `error`, broadcasts "Audit hash-chain HMAC key is not configured", and counts `UnsealedRows` as a first-class number | `modules/audit/service.go` `VerifyHashChain`; `modules/audit/hash_verifier.go` |
| "verified on a schedule" | **Accurate.** Hourly, leader-gated, writes an `audit.verified.scheduled` row every time, and broadcasts a notification the first time a given failure appears | `modules/audit/hash_verifier.go`, `hashVerifyInterval = time.Hour` |
| "exportable signed" | **An HMAC under the instance's own hash key.** Anyone holding that key can forge the signature, and **an external auditor cannot verify it at all**, because they have no key. The export is also silently capped at 10,000 rows | `modules/audit/export.go` `signExportMetadata` → `core/encryption/encryption.go` `StableHash`; `maxExportRows = 10000` |
| "write-once" | **Opt-in and seeded `off`.** When on, it is enforced by two SQLite triggers, and the delete trigger has a documented bypass: setting `audit.worm_delete_allowed` to `1` permits a sanctioned delete | `core/db/migrations/072_audit.sql` seeds `audit.worm` as `'off'`; `core/audit/worm.go` `WithSanctionedDelete` |
| "forwardable to SIEM" | **True, but not host-owned.** The host keeps forwarder configuration and the delivery loop; the transports ship as the `logging-syslog` and `logging-siem` plugins, so **a plain install forwards nothing** | `modules/audit/export.go`, `modules/audit/routes.go`; `src/plugins/Plugins/logging-syslog/`, `logging-siem/` |

## What each of those means for a control

### The chain is keyed, and the key may be absent

The chain proves that rows have not been altered **by someone who does not hold
`MCHARBOR_SECRET`**. It does not prove anything against an attacker who does, and it proves
nothing at all when the secret is unset. In that case rows are written unsealed and the
verifier says so out loud rather than reporting success.

So the honest statement of the control is conditional, and the condition is checkable:

> Audit rows are HMAC-SHA256 chained and verified hourly **when `MCHARBOR_SECRET` is set**.
> When it is not, rows are unsealed, the hourly verification records severity `error`, and
> `UnsealedRows` counts them.

Before citing tamper-evidence, read the verification (`GET /api/audit/verify`, key
`audit.view`) and take three numbers from its result: `keyConfigured`, `unsealed` and
`verified`. All three belong in the evidence pack, not just the last one.

### The export signature is not third-party verifiable

`signExportMetadata` is an HMAC under the instance's own hash key. It detects a file altered in
transit *by somebody without the key* and it lets the instance re-verify its own export. It is
not a signature in the sense an auditor means: there is no public key, no certificate, and no
way for a recipient to check it. The metadata also carries a plain `ChecksumSHA256` of the
rows, which **is** independently checkable and is the more useful of the two for handing a file
to someone else.

The 10,000-row cap matters for the same reason: an export covering a period with more than
10,000 audit rows is a **sample**, and nothing in the file says which rows were dropped. For a
full period, export in narrower windows and keep every file.

### WORM is off unless someone turned it on

`audit.worm` is seeded `'off'`. Until it is `'on'`, the audit table accepts updates and deletes
like any other table, and the only thing standing between a row and deletion is the permission
model. With it on, two SQLite triggers refuse updates outright and refuse deletes unless
`audit.worm_delete_allowed` is `'1'`, which exists so retention pruning and a sanctioned
correction remain possible.

That bypass is a deliberate, documented escape hatch and not a defect, but it is the difference
between "append-only" and "append-only except when an administrator says otherwise". State it
that way.

Two further limits worth knowing: the triggers are **SQLite** triggers, so on a PostgreSQL or
MySQL deployment the equivalent protection depends on what the dialect translation produced.
Check it rather than assuming. And a trigger protects the table, not the file: anyone with
filesystem access to the database has neither trigger nor permission in their way.

### Forwarding is a plugin, so a default install forwards nothing

If a control depends on audit events reaching a system outside McHarbor (and most
"independent review" and "centralised logging" controls do), then the evidence is not that
McHarbor *can* forward. It is that a forwarder is **installed, configured and enabled**. The
compliance posture already treats this correctly: it warns on any install with no enabled
forwarder rather than describing the capability.

Check `audit_log_forwarders` for an enabled row, and check that the relevant plugin is
installed and enabled. Both, because either alone is not delivery.

## Recording that a human reviewed something

Separate from tamper-evidence, and the one an access-review control runs into immediately.

**Evidenced** through `POST /api/compliance/attestations` (`compliance.attest`, global): a
named person signs off on a framework (the access review, a pack run, or a mapping as a whole)
at a recorded time, with a required note. `GET /api/compliance/attestations`
(`compliance.view`) reads every sign-off back, superseded ones included. There is no edit
route and no delete route, and each sign-off also writes a `compliance.attestation.create`
audit row whose actor column is the person who signed. See
[Attestations](../attestations/README.md).

Two limits worth stating before citing one:

- An attestation names a pack **run**, which is a fixed snapshot, but the access review is
  computed live. It records that the review happened and was accepted, not the permissions the
  reviewer saw. Export the catalog at review time and name the export in the note.
- `compliance_attestations` is an ordinary table. It is as tamper-evident as the audit row
  written beside it, which is to say exactly as much as the rest of this page allows.

Role assignment and group membership still have no attribution or history. `granted_by` on
`user_permissions` is the only grant-attribution column in the schema; everything else leaves
only generic `audit_logs` rows.

## The checklist to run before citing the audit log

| Ask | Where to look | What a good answer looks like |
|---|---|---|
| Is the chain keyed? | `MCHARBOR_SECRET` in the environment; `keyConfigured` in the verify result | Set; `keyConfigured: true` |
| Are there unsealed rows? | `unsealed` in the verify result | `0` |
| Did the last hourly verification pass? | `audit.verified.scheduled` rows in the audit log | Severity `success` |
| Is WORM on? | `audit.worm` setting | `on` |
| Is the delete bypass off? | `audit.worm_delete_allowed` setting | `0` |
| Is anything forwarding? | `audit_log_forwarders` for an enabled row, plus the plugin | One enabled row, plugin installed and enabled |
| Does the export cover the whole period? | Row count against the 10,000 cap | Under the cap, or several narrower exports |
| Can the recipient verify the file? | `ChecksumSHA256` in the export metadata | Checksum recorded and checked independently; the HMAC signature is instance-internal |

## Related

- [Compliance packs](../README.md): the index, the verdict vocabulary and the permission keys
- [SOC 2](../soc2/README.md) and [ISO/IEC 27001](../iso-27001/README.md): both mappings cite
  this page rather than restating it
