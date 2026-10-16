# Syslog Forwarder

RFC5424 over UDP, TCP, or TLS to rsyslog, syslog-ng, QRadar, or ArcSight.

Part of the McHarbor **`logging-`** plugin family: audit log forwarders
that ship the audit trail off-box. Its sibling is **`logging-siem`**, which
does the same job over HTTP. Install whichever your log
pipeline speaks, or both.

## What it does

McHarbor writes every audited action to its own `audit_logs` table. This
plugin forwards those events to an external collector so the trail
survives the loss of the McHarbor host — the property most compliance
regimes actually care about.

Each event is emitted as an RFC5424 message:

```
<PRI>1 TIMESTAMP HOSTNAME APP-NAME PROCID MSGID - BOM{"type":"mcharbor.audit.event",...}
```

`PRI` is `facility * 8 + severity`, with the event's own audit severity
mapped onto the syslog scale — an `error` audit event arrives as syslog
severity 3, a routine `info` event as 6 — so your collector's existing
severity rules apply without extra configuration. Nothing maps below
severity 3: an audit forwarder should not be able to page someone.

On TCP and TLS the message is octet-counted per RFC6587 §3.4.1, because
stream transports have no message boundary of their own. On UDP it is a
bare datagram.

**A note on UDP:** delivery history still records a `success` for a UDP
send, but that only means the datagram left this host — UDP cannot
confirm receipt. Each history row shows the protocol it used so a
`success` over `udp` is visibly weaker evidence than one over `tls`.
Prefer TLS unless your collector cannot terminate it.

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
`plugin_logging_syslog_delivery_history`.

Events older than the moment forwarding was switched on are never sent.
Enabling this on a year-old install does not replay a year of history
into a fresh collector.

## Upgrading from the built-in forwarder

Before this plugin existed, McHarbor shipped syslog forwarding as
host code. Migration `143_audit_log_forwarders.sql` imports the old
configuration — endpoint, credentials, watermark — into the generic
`audit_log_forwarders` row this plugin reads, so an existing setup keeps
working with no re-entry. Old delivery history is not carried over: the
retry ceiling would otherwise count attempts made by code that no longer
exists.

## Build

```bash
cd src/plugins/Plugins/logging-syslog
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
