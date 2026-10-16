# Alertmanager webhook: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| "receiver `<host>` resolves to a blocked address" | The URL, or the name it resolves to, is loopback, link-local (including `169.254.169.254`), unspecified or multicast | Use the receiver's routable address. A receiver on the McHarbor host itself must be reached through an address other than loopback. |
| "receiver URL for `<host>` uses http://" | Plain `http` without the opt-in | Use `https`, or turn on **Allow plain http** for a receiver on a trusted private network. |
| "Grafana IRM integration URLs end in /" | The trailing slash was lost when copying | Paste the OnCall Integration URL again, ending in `/`. |
| "receiver `<host>` answered HTTP 3xx" | The receiver redirected, and redirects are never followed | Use the final URL the receiver documents, usually its `https` form. |
| "receiver `<host>` answered HTTP 401" or 403 | Wrong token in the URL, or the wrong authentication mode | Copy the URL again; for Keep, use the `keep` profile with the API key as the password. |
| "receiver `<host>` answered HTTP 404" | The integration was deleted, or the URL is incomplete | Recreate the integration and paste its new URL. The link is parked until you retry it. |
| Deliveries retry and then succeed | HTTP 429 or 5xx from the receiver, or a network failure | Nothing; the host retries with backoff and honours `Retry-After`. |
| "link reference was not created by the Alertmanager webhook plugin" | A link's external ID is not `<fingerprint>@<startsAt>` | The link was created by another plugin on this channel type; resolve it manually at the receiver. |
| Every McHarbor alert lands in one incident at Zenduty | The profile is `generic`, and Zenduty groups by `alertname` | Switch the channel to the `zenduty` profile. |
| Acknowledging in the on-call tool changes nothing in McHarbor | Expected: the channel is one-way | Use a dedicated PagerDuty, incident.io, Rootly, Splunk On-Call or xMatters channel for acknowledgement sync. |
| Settings → Alerts → Tickets shows no remote state | Expected for the same reason | The link resolves when McHarbor resolves the alert. |
| Test passes, but no alert appears | The default Test only connects; it sends nothing | Turn on **Test connection sends a test alert** and test again. |

Error messages name the receiver as `scheme://host` only. The URL's path and query, and
the receiver's response body, are deliberately left out because they can carry the
integration token.

## Reading a failure

The host logs the real error server-side, and Settings → Alerts → Tickets shows the
operator-safe message beside a failed link. For anything the table does not explain,
the container log carries the host's side of the delivery:

```bash
docker compose logs mcharbor | grep -i alertmanager
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Alertmanager webhook](./README.md): what it is and how it works
