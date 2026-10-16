# Logging

**Settings → Integrations → Logging** · `?tab=logging`

Forward audit and application logs to an external log platform.

## Permissions

| Key | What it unlocks |
|---|---|
| `audit.view` **and** `settings.manage` | List forwarders; the response names every sink the trail is shipped to |
| `settings.manage` | Create, test and delete one |

A section you lack the key for renders **disabled with a tooltip naming it**, rather than
opening onto an empty panel.

## What it governs

- Named forwarders, each shipping the audit trail to one collector.
- Whether each is enabled, and its delivery history.

## Why forward at all

An audit trail stored only inside the system it audits is only as trustworthy as that
system. Forwarding puts the trail somewhere a McHarbor administrator cannot reach, which is
what most compliance regimes are actually asking for. The SIEM forwarder’s HMAC signature
lets the collector verify each event came from this McHarbor and was not modified on the
way.

Retention then becomes the collector’s problem, which is also the point: retention inside
McHarbor can be short without losing history.

## Clustering

Both shipped forwarders require leader election. In a multi-node install only the elected
leader forwards, so an event is shipped once rather than once per node. A node that is not
the leader forwarding nothing is correct behaviour.

## In this folder

| Page | What it covers |
|---|---|
| [Reference](./reference.md) | Every setting, its default, and what changing it costs |

## Related

- [Audit log forwarder plugins](../../../plugins/logging/README.md): both, with their fields
- [Audit](../../../screens/audit/README.md): the trail being forwarded
- [Monitoring → Data retention](../../monitoring/data-retention/README.md): how long McHarbor keeps its own copy
