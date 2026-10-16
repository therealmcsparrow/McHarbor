# Request Tracker: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| "custom field McHarbor Dedup Key is not applied to queue …" on Test connection | The custom field exists but is not applied to the queue, or has another name | Apply it to the queue under Admin → Custom Fields → the field → Applies to, or set `dedup_cf_name` to the real name. |
| A ticket parked with "… ticket #N was created without the dedup value" | The custom field was not applied when the alert fired. RT dropped the value silently | Apply the field to the queue, resolve ticket #N by hand, and retry the ticket from the Tickets list. McHarbor stops instead of retrying, because every retry would create another ticket. |
| "auth token rejected" | The token was revoked, mistyped, or belongs to a disabled user | Create a new token for the McHarbor user and paste it into `auth_token`. |
| "the token's user lacks a right on queue … (likely CreateTicket)" | The McHarbor user is missing that right on the queue | Grant the named right; the full list is on the [Configuration](./configuration.md#before-you-start) page. |
| "queue … was not found, or the token's user lacks SeeQueue on it" | Wrong queue name, or no `SeeQueue` | Check the exact queue name and the right. |
| "RT refused status "resolved" on queue …; set resolve_status …" | The queue uses a custom lifecycle without `resolved` | Set `resolve_status` to an inactive status of that lifecycle, and add it to `status_map` if it is not one of RT's defaults. |
| "RT kept ticket #N in status …" | The comment was added but the status did not change, often a lifecycle transition rule | Allow the transition in the lifecycle, or pick another `resolve_status`. |
| "RT answered with a redirect" | `instance_url` points at the wrong path, or plain HTTP redirects to HTTPS | Use the exact `https://` web root, including a `/rt` prefix if RT is served under one. |
| "response is not REST2 JSON" | A login page or proxy answers instead of RT | Point `instance_url` at RT itself, or exempt `/REST/2.0/` from single sign-on. |
| "instance_url must be an absolute https:// URL" | `http://`, a missing scheme, or a URL with credentials or a query | Use `https://host[/path]`. Plain HTTP is not supported, because the token travels on every call. |
| Status changes in RT never reach McHarbor | The Scrip is not installed, is on another queue, or cannot reach McHarbor | Check the Scrip applies to the queue and the RT server can reach the inbound URL. Until then, polling picks the change up within `poll_interval_minutes`. |
| Scrip callouts are rejected | Wrong secret, the inbound URL was rotated, or the RT server's clock is more than five minutes off | Copy a fresh URL and secret into `RT_SiteConfig.d/McHarbor.pm`, restart RT, and check NTP on the RT server. |
| RT e-mails someone about every alert | `requestor` is set, and the queue autoreplies to requestors | Clear `requestor`. |

**Test connection** creates nothing: it only reads the queue, its custom fields and a one-ticket
search. The first real ticket is created by the first alert routed to the channel.

## Reading a failure

A ticket that failed permanently shows its message in the Tickets list behind the error icon.
Messages this plugin marks as safe (every message in the table above) are shown verbatim; RT's
own error text is shown with the token redacted and cut at 300 characters. Anything else appears
as a generic error, and the container log carries the detail:

```bash
docker compose logs mcharbor | grep -i request_tracker
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Request Tracker](./README.md): what it is and how it works
