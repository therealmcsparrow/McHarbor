# Compliance

`/enterprise/compliance`, permission `compliance.view` (scope `global`)

One page that answers the questions an auditor asks, measured on this instance at the moment
you open it, so nobody has to assemble the answer from screenshots of ten settings pages.

## Where the data comes from

One read-only endpoint, `GET /api/compliance/posture`, behind `compliance.view`. It holds no
state of its own: every value is read from the tables and configuration the rest of McHarbor
already uses. A source that cannot be read (a missing table, a database error) turns its
section **Unknown** instead of failing the page; the server log names the cause.

| Query parameter | Meaning |
|---|---|
| `sections` | Comma-separated section IDs; empty means all. An unknown ID is a `400` with `data.reason = invalid_query` |
| `redact` | `1` blanks sensitive values. Defaults to `1` when `export=1`, otherwise `0` |
| `export` | `1` records a `compliance.report.export` audit entry naming the sections and whether it was redacted |

`compliance.view` is not in any role preset. Admin roles hold it through their wildcard; grant
it to an auditor role explicitly.

## Sections

| ID | Title | Measured | Verdict |
|---|---|---|---|
| `audit` | Audit retention and forwarding | Retention days and preset (Settings → Monitoring → Data retention), enabled audit log forwarders | Attention below 365 days (0 is "keep forever" and passes), or with no forwarder |
| `keys` | Encryption keys | Whether `ENCRYPTION_KEY` is set or the key was generated into `DATA_DIR`; whether a backup key is loaded, and its key ID | Attention for a generated key or no backup key; **Fail** for a generated key on a shared (non-SQLite) database |
| `instanceBackup` | Instance backup | Enabled schedules, last successful export, last run status, last verification | Fail when the last run failed; attention with no schedule, no success ever, a success older than 7 days, or a failed verification |
| `tls` | TLS | Whether TLS is on (the saved Settings → HTTPS value wins over `TLS_ENABLED`, as at start-up), certificate source, minimum version, expiry, subject | Fail when expired; attention when off, self-signed, unreadable, or expiring within 30 days |
| `passwords` | Password policy | Minimum length, character classes, maximum age, history size | Fail below 8 characters; attention below 12 or with no history |
| `mfa` | Multi-factor authentication | Active **local** accounts, how many have MFA enrolled, the share (rounded down), and accounts from identity providers | Fail at 0%, attention below 100%, info with no local accounts |
| `identitySync` | Identity and provisioning | Enabled identity providers, enabled SCIM endpoints, failing endpoints, last sync | Attention when an endpoint's last sync failed; info when neither is configured |
| `plugins` | Plugin signing | Installed plugins, and how many were accepted without a valid signature | Attention when any is unsigned |
| `ha` | High availability | Mode, database, plugin store, this node's role and readiness, node ID | Info on a single node; attention for active/passive with a local plugin store; fail for active/passive without `ENCRYPTION_KEY` |
| `selfEnvAccess` | Container access to McHarbor's host | The environments whose Docker daemon runs McHarbor itself, the roles holding `containers.manage` (or `*`) there through an all-environments or that-environment assignment, which of them lack a global `host_self.manage`, and the users who hold `containers.manage` there through a direct grant instead of a role | Attention when any role, or any user with a direct permission grant, holds `containers.manage` there without `host_self.manage`, because it can start a socket-mounting container and read McHarbor's keys and database, so it is admin-equivalent; info when no environment matches McHarbor's daemon |
| `supplyChain` | SBOM and image signing | Static: SPDX JSON SBOM, cosign keyless signing | Always info; it describes release artifacts, not this instance. See [Verifying releases](../../getting-started/verifying-releases/README.md) |

Thresholds are conservative defaults, not a claim about a specific framework; the raw values
are shown so an auditor can apply their own. The account placeholder `__legacy_owner__` that
migration 066 seeds is excluded from the MFA counts.

Each non-passing section has an **Open** button to the page where it is fixed.

## Downloading a report

**Refresh** and **Download report** sit in the page header. **Refresh** re-reads the posture;
**Download report** opens a dialog:

- **Sections**: tick the ones to include.
- **Format**: JSON (the endpoint's response as-is), **Word document (.docx)** or **PDF document
  (.pdf)**. The Word and PDF files carry the same content: the version and build, the generation
  time, whether the report is redacted, and per section its status, description, a fact/value
  table and its findings. Both are produced in the browser; nothing is rendered on the server.
  The PDF uses the standard Helvetica font, so a character it cannot draw (an arrow, for
  instance) is written as `->` or `?` rather than garbled.
- **Redact identifying values**: on by default. The dialog lists exactly which values will be
  replaced with `[redacted]`: backup key ID, node ID, certificate subject. Secrets
  (keys, passwords, tokens, DSNs) are never in the report, redacted or not.

A failed download keeps the dialog open and shows the server's message.

## What goes wrong

| Symptom | Cause |
|---|---|
| A section reads **Unknown** | Its table could not be read. The server log has `compliance: source unavailable` with the table name |
| TLS reads off although `TLS_ENABLED=true` | A value saved under Security → HTTPS overrides the variable at start-up; the page reports what the listener actually does |
| MFA shows 0 local accounts | Everyone signs in through an identity provider; their MFA is the provider's |
| Container access lists the Operator or Editor preset | Those presets carry `containers.manage`; assigned for all environments they reach the one McHarbor runs on. Assign them per environment, or add `host_self.manage` to a role you treat as administrator |
| Container access reads info with nothing listed | No environment's daemon holds McHarbor's own container (McHarbor runs elsewhere, or the daemon was unreachable at report time) |
