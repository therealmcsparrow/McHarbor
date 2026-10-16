# Audit: Integrity

**Verify integrity**, in the audit page header.

Every sealed audit row carries an HMAC over its content and the hash of the row before it.
Verification walks the chain oldest first and recomputes each one.

## Running it

**Verify integrity** opens a dialog; **Run verification** calls `GET /api/audit/verify`
(gated by `audit.view`) and shows "Verifying the hash chain…" until it answers. On a large
trail this can take a while: it checks up to 1,000,000 rows. Each run writes an
`audit.verified` row. The same check also runs hourly on the cluster leader and writes
`audit.verified.scheduled`.

## The verdict

| Verdict | Meaning |
|---|---|
| **Chain verified** | Every sealed row checked out |
| **Verified with unsealed or unchecked rows** | Nothing is broken, but some rows were written with the hash chain off, or the trail is longer than one run checks |
| **Chain broken** | A row does not match, or does not link to the row before it. The dialog names the first broken row and the reason |
| "No HMAC key is configured" | `MCHARBOR_SECRET` is not set, so sealed rows cannot be checked |

## The figures

| Figure | Meaning |
|---|---|
| Rows checked | Rows the walk read |
| Sealed rows | Rows with a hash |
| Unsealed rows | Rows written while `audit.hash_chain.enabled` was off |
| Retention gaps crossed | Places where retention removed rows and a tombstone bridged the gap |
| Pruned rows | How many rows those gaps stand for |
| Tombstones with a bad seal | Tombstones whose seal does not verify. They are ignored, so a gap they would have covered reads as broken. Anything but 0 is an incident |

## Restore anchors

An instance restore writes an anchor row into the chain it replaced. Each one is listed with
who restored and when, and its details carry the terminal hash of the chain that was
replaced. A chain through a restore is legitimate, and it is always shown rather than folded
into a plain pass. "The chain passes through no instance restore." means there are none.

## Retention tombstones

Retention is the one sanctioned way rows leave the chain. For each run of consecutive
removed rows the pruner stores, in `audit_chain_tombstones`, the hash the run was entered
from, the hash of its last row, the row count, the first and last row ids and timestamps, a
per-severity count, and a seal: an HMAC under the same secret as the rows. When the walk
meets a row whose previous hash is not the last hash it saw, it may step across tombstones
from one to the other; it may not step across anything else.

A tombstone therefore proves that rows were removed by retention and how many, not what they
said. Deleting rows by hand, or writing a tombstone without the secret, still reads as a
broken chain.

Rows pruned before tombstones existed left no record. On such an install the first
verification after upgrading fails at the oldest surviving row with "previous hash
mismatch"; that is the old prune, not tampering, but nothing in the database can prove it.

## Related

- [Audit](./README.md): the screen as a whole
- [Retention](./retention.md): what creates tombstones
- [Troubleshooting](./troubleshooting.md): when verification fails
