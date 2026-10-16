# PagerDuty: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| Test fails with "routing key must be 32 characters" | A REST key or a service ID was pasted into the routing key field | Copy the **Integration Key** from the service's Events API V2 integration. |
| Test fails with "region must be us or eu" | The channel was created through the API with another value | Set the region to `us` or `eu`. |
| Test fails naming the region, "REST API key was rejected by the EU service region (HTTP 401)" | The REST key belongs to an account on the other region, or was revoked | Switch the region, or create a new read-only key. An EU routing key sent to the US host is forwarded, but a REST key on the wrong host is refused. |
| Test fails on `severity_map` | A key other than `critical`, `warning`, `info`, or a value other than `critical`, `error`, `warning`, `info` | Fix the JSON. PagerDuty accepts only those four severities. |
| Test passes but no incident appears when an alert fires | Test never sends a trigger, so a wrong routing key only shows on the first real event | Use **Send Test Event** on the PagerDuty integration page. The ticket row shows **Stopped retrying** with PagerDuty's own message behind the error icon. |
| Ticket stays **Pending** or retries | PagerDuty answered 429 or 5xx, or could not be reached | Nothing to do: the outbox retries with backoff, honouring `Retry-After`. |
| Ticket shows **Stopped retrying**, "PagerDuty returned a different dedup_key" | A proxy or an event rule rewrote the key, so a later resolve would miss | Remove the rewrite; then **Try again**. |
| Acknowledge in PagerDuty never shows on the Tickets tab | No webhook and no REST key; or the webhook's signing secret does not match | Add the v3 webhook (see [Configuration](./configuration.md)), paste its secret into **Webhook signing secret**, or add a REST key for polling. |
| Every webhook delivery is refused with 401 | The inbound URL was rotated, or the secret on the channel is not the one PagerDuty issued | Rotating the inbound URL invalidates the old one: update the subscription URL in PagerDuty. Paste the subscription's secret again. |
| One ticket never syncs while others do | PagerDuty grouped the alert into an incident another alert opened; that incident carries the other key | Expected with alert grouping. The resolve is still sent when the alert recovers. Turn grouping off on the service if status sync matters more. |
| Ticket shows no incident number or link | No webhook or poll has reported the incident yet | Add a REST key or the v3 webhook; the number appears on the first report. |

**Test connection never pages anyone.** It validates the configuration and, with a REST
key, calls `GET /abilities`. It cannot prove the routing key; PagerDuty's **Send Test
Event** can.

## Reading a failure

A ticket row that stopped retrying carries PagerDuty's own validation text behind its error
icon, with the routing key and REST key redacted. Anything else (a transport error, an
unexpected status) is logged server-side and shown as a generic message. The container
log has the rest:

```bash
docker compose logs mcharbor | grep -i -e comm-pagerduty -e itsm
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [PagerDuty](./README.md): what it is and how it works
- [Tickets](../../../settings/monitoring/alerts/tickets/README.md): link states and retries
