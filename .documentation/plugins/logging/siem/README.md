# SIEM

Forwards audit events to an HTTP collector: Splunk HEC, Microsoft Sentinel, Elastic, Datadog, or anything that answers 2xx to a JSON POST. Each event carries an HMAC-SHA256 signature and a SHA-256 body checksum so the collector can verify origin and integrity.

- **Slug** `logging-siem`, **version** 2026.09.01, **forwarder type** `siem`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Logging, as a forwarder type
- **Gated by** `audit.view` **and** `settings.manage` to list forwarders; `settings.manage` alone to create, test, or delete one

## How it works

- The signature is sent as `X-McHarbor-Signature`, computed over the body with `signing_secret`. Verifying it on the collector side is what makes the forwarded trail trustworthy. If you do not verify, anything that can reach the endpoint can write to your audit index.
- The plugin creates its own tables (`db.migrations.run`) to hold forwarder rows and delivery history.
- `max_retries` bounds retries per event; failures stay in delivery history either way, so a dropped event is visible rather than silent.
- It takes part in leader election: in a clustered install only the leader forwards, so events are not duplicated per node.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `EncryptedCredentials` | Yes | The configuration holds a secret, encrypted at rest with AES-256-GCM. |
| `SupportsDeliveryHistory` | Yes | Every attempt is recorded, so a failed forward is visible rather than silent. |
| `RequiresLeaderElection` | Yes | Only the elected leader forwards, so a clustered install does not send each event several times. |

## In this folder

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |
| [Developer](../../../development/plugins/logging/siem.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Report catalog](../../../reporting/reports/README.md): the delivery report this plugin contributes
- [Audit log forwarders](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
