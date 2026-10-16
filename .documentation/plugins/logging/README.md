# Audit Log Forwarders

An audit-log-forwarder plugin ships McHarbor's audit trail somewhere else: a SIEM, a
syslog collector, whatever the organisation keeps its logs in. The host owns the trail,
the forwarder rows and the admin screen; the plugin owns delivery.

| Plugin | Forwarder type | Transport | What it forwards to |
|---|---|---|---|
| [SIEM](./siem/README.md) | `siem` | HTTPS JSON POST, HMAC-signed | Forward McHarbor audit events to an HTTP SIEM collector (Splunk HEC, Microsoft Sentinel, Elastic, Datadog). |
| [Syslog](./syslog/README.md) | `syslog` | RFC5424 over UDP, TCP or TLS | Forward McHarbor audit events to a syslog collector (rsyslog, syslog-ng, QRadar, ArcSight) as RFC5424 messages over UDP, TCP, or TCP+TLS. |

Each plugin has a folder of its own: a README, the configuration reference, a
troubleshooting table, and a developer page covering the adapter it registers and how to
build, sign and fork it.

## What they have in common

- They register through `AdapterRegistry.Logging()` and are configured under
  **Settings → Integrations → Logging**.
- Both run their own migrations and keep **delivery history**, so a failed forward is a
  visible row rather than a silent gap.
- Both require **leader election**: in a clustered install only the leader forwards, so an
  event is not written once per node.
- Reading the forwarder list needs `audit.view` *and* `settings.manage`, because the response
  names every sink the audit trail is being shipped to, which is administrative
  configuration rather than audit data.
- The `/audit/logging/*` subtree is host-owned even though every forwarder behind it is a
  plugin. That is deliberate: one shape of form for the job, instead of one per plugin.

## Choosing

Use **SIEM** when the collector speaks HTTP and you want per-event integrity you can
verify. Use **Syslog** when the collector is a syslog daemon, and choose TLS over UDP
there, because UDP cannot tell you it failed.

## Related

- [Plugin catalog](../catalog.md): every shipped plugin in one table
- [Plugins](../README.md): the section index
