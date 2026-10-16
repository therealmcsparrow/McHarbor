# Zammad: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| Test fails with "the Zammad URL must be an absolute https:// URL" | The URL is `http://`, has no host, or has no scheme | Use the `https://` address of the instance. Plain HTTP is refused because the token travels in a header. |
| Test fails with "Zammad answered with a redirect" | The URL points at an address that forwards elsewhere, such as `http` → `https` or a bare domain → `www` | Set the Zammad URL to the final address. Redirects are never followed. |
| Test fails with "access token rejected (revoked or the token feature is disabled)" | The token was deleted, or **Admin → Settings → System → API** has token access off | Create a new token, or switch token access on. |
| Test or a ticket fails with "the token owner cannot create tickets in group …" | Zammad answered 403, or 422 `Not authorized`: the token owner lacks `ticket.agent` on that group | Give the McHarbor agent `ticket.agent` on the group, or pick a group it already has. |
| Test fails with "group … not found or inactive" | The name does not match exactly, or the group is disabled | Copy the group name from **Admin → Groups**. For a nested group, use the full name Zammad shows. |
| Test fails with "customer … not found" | No user has that login or email | Create the customer user, or use `guess:<email>`. |
| Test fails on `severity_map` or `status_map` | Not a JSON object of strings, an unknown severity key, or a status value other than `open`, `acknowledged`, `resolved`, `closed`, `cancelled` | Fix the JSON. |
| Ticket shows **Stopped retrying** with a message about a required field on close | A Core Workflow makes a field (a resolution, for example) mandatory on close | Relax the workflow for the McHarbor user, or set **State on recovery** to a state the workflow allows, such as `pending close`. |
| Ticket stays **Pending** or retries | Zammad answered 429 or 5xx, or could not be reached | Nothing to do: the outbox retries with backoff, honouring `Retry-After`. |
| Two tickets for one alert | The second is from a new firing after the alert recovered; each firing has its own marker | Expected. A retried open of the same firing never duplicates. |
| State changes in Zammad never show on the Tickets tab | No trigger or webhook, the trigger does not match, or polling is off | Check the trigger conditions (**Tags contains one** `mcharbor`, **State has changed**), or set a poll interval above `0`. |
| Every webhook delivery is refused with 401 | The signing secret in Zammad is not the channel's, or the inbound URL was rotated | Paste the current signing secret into **HMAC SHA1 Signature Token**, and after a rotation update the webhook endpoint too. |
| Webhook deliveries succeed but nothing changes | The custom payload lacks `ticket.id`, `title` or `state`, or the ticket title no longer starts with `[MH-` | Use the default payload, or add the fields. Do not edit the marker out of the title. |
| A ticket merged into another reads as **Closed** | Zammad reports the merged ticket's state as `merged` | Expected. McHarbor stops updating it; the ticket it was merged into is not tracked. |
| Search finds nothing and a retried open creates a second ticket | The instance has no Elasticsearch and its database search did not match the marker | Enable Elasticsearch on the instance, and close the duplicate by hand. The title filter never accepts a wrong ticket, but it cannot find one the search does not return. |

**Test connection never creates a ticket.** It only reads the current user, the groups and
the customer.

## Reading a failure

A ticket row that stopped retrying carries Zammad's own `error_human` text behind its error
icon, cut to 300 characters, with the access token redacted. Anything else (a transport
error, an unexpected status) is logged server-side and shown as a generic message. The
container log has the rest:

```bash
docker compose logs mcharbor | grep -i -e comm-zammad -e itsm
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Zammad](./README.md): what it is and how it works
- [Tickets](../../../settings/monitoring/alerts/tickets/README.md): link states and retries
