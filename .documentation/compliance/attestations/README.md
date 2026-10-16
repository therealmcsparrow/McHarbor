# Attestations

An attestation is **a person's sign-off on a body of evidence**: who reviewed it, which
framework it was for, which pack run it accepted, when, and a note saying what they accepted.
It is the record that SOC 2 CC6.3, ISO/IEC 27001 A.5.18 and clauses 9.2 and 9.3, and most
customer security questionnaires ask for: "when was this last reviewed, and by whom".

McHarbor computes the evidence. It does not decide that a control is met and it does not sign
anything off itself. An attestation is always a named user's act, written under their own
account, and it says only what that person wrote in the note.

## Where to record one

| Surface | Framework it records |
|---|---|
| **Enterprise → Compliance packs → Sign-off** | The selected pack, naming its latest run |
| **Security → Effective access → Access review sign-off** | `access-review`, with no run |
| `mcharbor compliance attest <framework> [--run <id>] --note "…"` | Any of the frameworks below |
| `POST /api/compliance/attestations` | Any of the frameworks below |

The dialog shows the framework and the run being accepted before anything is sent, and will
not confirm without a note. The latest sign-off for the framework is shown beside the button,
with who, when and the note.

## What one records

| Field | Meaning |
|---|---|
| `framework` | A pack key (`cis-docker-benchmark`), `access-review`, `soc2` or `iso-27001` |
| `runId` | The pack run accepted. Empty for the access review and for a mapping as a whole |
| `scope` | Optional: an environment ID or a short label the sign-off covers, up to 128 characters |
| `attestedBy` | The user ID of the person who signed, taken from the session or API key, never from the request body |
| `attestedByName` | Their username, read at query time |
| `attestedAt` | When it was recorded, UTC |
| `note` | Required, up to 4,000 characters: what was reviewed, what was accepted, and where the decision is recorded |

A named run must exist, must belong to the same framework, and must have finished. A sign-off
on a run that is still in progress is refused, because its result is not yet a snapshot.

## It cannot be changed afterwards

There is **no edit route and no delete route**. A sign-off that turns out to be wrong is
superseded by a newer one; both read back, newest first. That is deliberate: the history an
auditor reads is the one that happened.

Every sign-off also writes an audit row, `compliance.attestation.create`, whose actor column is
the person who signed and whose details carry the framework, run, scope and note.

## Routes

As mounted:

| Route | Key | Returns |
|---|---|---|
| `GET /api/compliance/attestations?framework={key}&page=&per_page=` | `compliance.view` | Sign-offs newest first, paginated; superseded ones included |
| `POST /api/compliance/attestations` | `compliance.attest` | The recorded attestation, `201` |

The `POST` body is `{ "framework": "…", "runId": "…", "scope": "…", "note": "…" }`. A refusal is
a `400` with `err.invalid_body` and a `reason` of `unknown_framework`, `note_required`,
`scope_too_long`, `run_mismatch` or `run_not_finished`.

## Permissions

| Key | Scope | Grants |
|---|---|---|
| `compliance.view` | global | Reading attestations |
| `compliance.attest` | global | Recording one |

`compliance.attest` is its own key rather than part of `compliance.manage`, so that starting a
check and accepting its result are two authorities, which is the separation an auditor looks for. No
role preset carries it; grant it to the people whose sign-off you want on record.

## What it does not do

- **It does not freeze the access review.** The access review is computed live, so an
  `access-review` attestation records that the review happened and was accepted, not the
  permissions the reviewer saw. Export the catalog at review time
  (`GET /api/access-review/permission-catalog/export`) and name the file in the note.
- **It does not make the table tamper-evident.** `compliance_attestations` is an ordinary
  table; the audit row beside each sign-off is as tamper-evident as
  [Evidence integrity](../evidence-integrity/README.md) says the audit log is, and no more.
- **It does not attribute role assignments.** Role assignment and group membership still have
  no history beyond generic audit rows.

## Reporting

The Reporting add-on offers attestations as the `compliance_attestations` source to its ad-hoc
report planner, with the columns framework, run, scope, attested by, attested at and note.

## Related

- [Compliance packs](../README.md): the index, the verdict vocabulary and the other keys
- [SOC 2](../soc2/README.md): CC6.3
- [ISO/IEC 27001](../iso-27001/README.md): A.5.18, clauses 9.2 and 9.3
- [CLI](../../cli/README.md): `compliance attest` and `compliance attestations`
