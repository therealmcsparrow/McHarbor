# Syslog

Forwards audit events to a syslog collector (rsyslog, syslog-ng, QRadar, ArcSight) as RFC5424 messages over UDP, TCP, or TCP with TLS.

- **Slug** `logging-syslog`, **version** 2026.09.01, **forwarder type** `syslog`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Logging, as a forwarder type
- **Gated by** `audit.view` **and** `settings.manage` to list forwarders; `settings.manage` alone to create, test, or delete one

## How it works

- TLS is the default and the only protocol that both encrypts the trail in transit and confirms the collector accepted it. UDP confirms nothing: a dropped datagram looks exactly like a delivered one.
- `tag` is the RFC5424 APP-NAME and defaults to `mcharbor`. Keep it stable; collector-side filters match on it.
- `facility` decides which ruleset or file the collector routes to, default `local0`.
- Like the SIEM forwarder, it keeps delivery history in its own tables and forwards only from the elected leader.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `EncryptedCredentials` | No | The configuration holds a secret, encrypted at rest with AES-256-GCM. |
| `SupportsDeliveryHistory` | Yes | Every attempt is recorded, so a failed forward is visible rather than silent. |
| `RequiresLeaderElection` | Yes | Only the elected leader forwards, so a clustered install does not send each event several times. |

## In this folder

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |
| [Developer](../../../development/plugins/logging/syslog.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Report catalog](../../../reporting/reports/README.md): the delivery report this plugin contributes
- [Audit log forwarders](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
