# GLPI: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| Test fails with "the GLPI URL must be an absolute https:// URL without credentials or query" | The URL is `http://`, carries `user:password@`, a query string or a fragment | Use the plain `https://` base URL of GLPI. Plain HTTP is not supported. |
| Test fails with "GLPI has no REST API at /apirest.php" | The URL points past GLPI's root, or the REST API is disabled | Use the URL of GLPI's root, and enable the REST API under **Setup → General → API**. |
| Test fails with "There isn't an active API client matching your IP address" | No API client admits McHarbor's address | Add or widen an API client's IPv4 range under **Setup → General → API**. The message names the address GLPI saw. |
| Test fails with "GLPI rejected the user API token" | The token was regenerated, belongs to a disabled user, or login with an external token is off | Regenerate the API token in the user's preferences, and enable login with an external token. |
| Test fails with "parameter app_token seems wrong" | The API client defines an App-Token and it was not pasted, or was mistyped | Copy the App-Token from the API client into **App-Token**. |
| Test fails with "entity N is not active for the API user" | `entities_id` names an entity the user's profile does not cover | Give the user a profile in that entity (recursive where needed), or change **Entity ID**. `0` uses the user's active entity. |
| Test fails with "GLPI has no high-level API at /api.php" | API v2 against GLPI 9.5 or 10 | Choose API v1, or upgrade to GLPI 11. |
| Test fails with "The authenticated client is not authorized to use this authorization grant type" | The OAuth client does not allow the password grant | Enable the **password** grant on the OAuth client. |
| Test fails with "GLPI rejected the credentials (HTTP 401)" | Wrong OAuth client ID or secret, or the token lacks the `api` scope | Re-copy the client ID and secret; give the client the `api` scope. |
| Test fails with "The user credentials were incorrect" | Wrong API user login or password | Check the login and password of the API user. |
| Test fails with "GLPI answered 404 on v2.x; check the HL API version path" | `v2_path` names a version the instance does not serve | Use `v2.0`, or a version GLPI advertises. |
| Test fails with "GLPI answered with a redirect" | The URL is an alias that redirects, often `http` → `https` or a missing path | Set the GLPI URL to the address the redirect lands on. Redirects are never followed. |
| Ticket shows **Stopped retrying**, "GLPI refused the ticket: You don't have permission to perform this action." | GLPI answered 207 Multi-Status: the create was refused for the user's profile or entity | Grant the profile ticket creation in the target entity; then **Try again**. |
| Ticket shows **Stopped retrying**, "GLPI refused the request (HTTP 403)" | The user may not update, follow up or solve that ticket | Widen the profile's ticket rights in the ticket's entity. |
| Ticket stays **Opening** or retries | GLPI answered 429 or 5xx, or could not be reached | Nothing to do: the outbox retries with backoff, honouring `Retry-After`. |
| A second GLPI ticket appears for one alert | The first ticket was solved, closed or trashed before the alert fired again, so it was no longer reusable | Expected: only a live ticket with the exact external ID is reused. Set **Reopen window** to reopen recent ones instead. |
| Status changes in GLPI never show on the Tickets tab (GLPI 11) | No webhook, the CRA option is on, or the secrets differ | Add the webhook (see [Configuration](./configuration.md)), turn CRA off, and make both sides use the same secret. Or set a poll interval. |
| Every webhook delivery is refused with 401 | The inbound URL was rotated, the secret differs, or GLPI's clock is more than five minutes off McHarbor's | Update the URL in GLPI after a rotation, paste the secret again, and synchronise both clocks with NTP. |
| Status never updates on GLPI 9.5 or 10 | **Poll interval** is 0 | Set a poll interval between 1 and 60 minutes. These versions have no webhooks. |
| A ticket stays open in McHarbor after it was deleted in GLPI | The ticket was purged and no webhook reported it; a poll leaves a missing ticket out | Resolve the alert, or add **purge** to the GLPI webhook's events. |

**Test connection never creates a ticket.** On API v1 it opens a session, reads the API
user's active entities and kills the session; on API v2 it obtains a token and reads one
ticket.

## Reading a failure

A ticket row that stopped retrying carries GLPI's own message behind its error icon, with
tokens, secrets, passwords and session tokens redacted. Anything else (a transport error,
an unexpected status) is logged server-side and shown as a generic message. A refused
webhook delivery is logged with the reason: a timestamp outside the five-minute window is
logged as such, a bad signature as "webhook signature rejected". The container log has the
rest:

```bash
docker compose logs mcharbor | grep -i -e comm-glpi -e itsm
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [GLPI](./README.md): what it is and how it works
