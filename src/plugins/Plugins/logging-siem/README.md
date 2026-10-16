# SIEM HTTP Forwarder

Signed JSON over HTTPS to Splunk HEC, Sentinel, Elastic, or any JSON collector.

Part of the McHarbor **`logging-`** plugin family: audit log forwarders
that ship the audit trail off-box. Its sibling is **`logging-syslog`**, which
does the same job over RFC5424 syslog. Install whichever your log
pipeline speaks, or both.

## What it does

McHarbor writes every audited action to its own `audit_logs` table. This
plugin forwards those events to an external collector so the trail
survives the loss of the McHarbor host — the property most compliance
regimes actually care about.

Each event is POSTed as a JSON document:

```
POST <endpoint_url>
Content-Type: application/json
X-McHarbor-Checksum-SHA256: <sha256 of body>
X-McHarbor-Signature: sha256=<hmac-sha256 of body, keyed by your secret>
X-McHarbor-Signature-Version: v1

{"type":"mcharbor.audit.event","entry":{...}}
```

Any 2xx is treated as success. The signature lets the collector verify
both integrity and origin; the wire format is byte-identical to the
host-owned forwarder this plugin replaces, so existing collector-side
verification rules keep working.

## Configure it

**Settings → Integrations → Logging.** The form is rendered from the
adapter's declared config schema, so the fields you see are exactly the
fields the adapter uses. **Send test** performs a real delivery, not a
config check: if it succeeds, your endpoint, credentials, and network
path all work.

## How delivery works

The host runs one forwarding worker on a five-second ticker and, in an
HA cluster, only on the elected leader — so an N-node cluster delivers
each event once. Each cycle this plugin selects audit events that have
no successful delivery and fewer than `max_retries` attempts, sends
them, and records the outcome in
`plugin_logging_siem_delivery_history`.

Events older than the moment forwarding was switched on are never sent.
Enabling this on a year-old install does not replay a year of history
into a fresh collector.

## Upgrading from the built-in forwarder

Before this plugin existed, McHarbor shipped SIEM HTTP forwarding as
host code. Migration `143_audit_log_forwarders.sql` imports the old
configuration — endpoint, credentials, watermark — into the generic
`audit_log_forwarders` row this plugin reads, so an existing setup keeps
working with no re-entry. Old delivery history is not carried over: the
retry ceiling would otherwise count attempts made by code that no longer
exists.

## Build

```bash
cd src/plugins/Plugins/logging-siem
go vet ./...          # type-check (buildmode=plugin needs Linux)
```

The container build compiles the plugin alongside the host in one
`go work sync` shell — see `docker/Dockerfile`. That is required, not
incidental: host and plugin must share per-package ABI hashes or
`plugin.Open` rejects the `.so`.

```bash
docker compose build && docker compose up -d
```

## Permissions

This plugin declares none. The Logging tab that configures it is gated
by the host's `settings.manage`, and forwarding runs from the host's
worker rather than being invoked per-operator, so there is no discrete
capability to grant — the same reasoning the storage-adapter family
uses.

It declares `consumes_permissions: audit.read` because it reads the
host's `audit_logs` table: "not yet delivered" is a join between the
host's event table and this plugin's own delivery history, and neither
side can compute it alone.
