# iTop: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| "iTop rejected the credentials, or the user lacks the REST Services User profile" | Wrong token or password, an expired token, or the profile is missing | Check the token in iTop, and give the account the REST Services User profile. Token authentication needs iTop 3.1 or later; use `password` on older versions. |
| "the iTop URL must be an absolute https:// URL…" | `http://`, credentials in the URL, or a query string | Use the plain `https://` base URL of iTop, without `/webservices/…`. |
| "iTop answered HTTP 301" (or 302, 404) | The URL points at a redirect or at the wrong path | Redirects are not followed, so the form body is never replayed to another host. Use the final address of the iTop root. |
| "iTop returned a non-JSON response" | A login page, proxy or maintenance page answered instead of `rest.php` | Open `<instance_url>/webservices/rest.php?version=1.3` from the McHarbor host; it must answer JSON. |
| "iTop found 0 objects for organization …" (or 2 and more) | The name does not match exactly one organization | Copy the name from iTop exactly. |
| "agent N is not a member of team M" | The agent and team pair would be refused by `ev_assign` | Add the agent to the team in iTop, or pick a team they belong to. |
| "iTop error 100: … mandatory …" on create | The data model requires a field McHarbor does not send, often `caller_id` | Set `caller_email`, or relax the field in iTop. |
| Create fails on `origin` | Your data model has no `monitoring` value in the `origin` list | Set `origin` to a value it has, such as `portal` or `mail`. |
| Resolve fails on `resolution_code` | The value is not in the class's resolution-code list | Use a code the class accepts; User Requests and Incidents can differ. |
| "iTop … is in state …, from which McHarbor cannot resolve it" | The ticket sits in a state outside the Incident lifecycle, for example awaiting approval | Resolve it by hand in iTop; McHarbor reads the new state on the next poll. |
| Two tickets for one alert | Someone removed the `[MH-…]` marker from the title | Keep the marker at the start of the title while the alert is open. |
| Status changes arrive late | No webhook; McHarbor polls on `poll_interval_minutes` | Set up the webhook recipe in [Configuration](./configuration.md). |
| The webhook is refused with 401 | Missing or wrong `X-McHarbor-Secret` header, or the inbound URL was rotated | Put the current signing secret in the Remote application connection. |
| The webhook is accepted but nothing changes | The trigger filter or the payload template differs from the documented one, or the ticket was not opened by McHarbor | Use the template exactly; iTop's own tickets are ignored by design. |

**Test connection** creates nothing in iTop. The first real ticket appears when a routed
alert fires.

## Reading a failure

A failed delivery is retried when iTop is unreachable or answers 429 or 5xx, and is
stopped otherwise. iTop reports most failures inside an HTTP 200 answer, and the plugin
passes iTop's own message through, with any credential redacted, to the ticket's error
under **Settings → Alerts → Tickets**. For anything else, the container log has the
detail:

```bash
docker compose logs mcharbor | grep -i comm-itop
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [iTop](./README.md): what it is and how it works
