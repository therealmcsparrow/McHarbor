# OTRS / Znuny / OTOBO: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| `web service "…" not found; import McHarborConnectorREST.yml…` | The web service is not imported, is imported under another name, or the product or script path points at the wrong script | Compare **Web service name** with Admin → Web Services, and check that `<instance_url>/otrs/` (or `/otobo/`) is where the instance lives. Set **Script path override** for a custom path. |
| `… did not return JSON; is the web service imported with this route mapping?` | The URL answers, but not with GenericInterface: a login page, a proxy, or a web service without the `/Session`, `/TicketSearch` or `/TicketGet` routes | Re-import the shipped YAML; the upstream sample connector uses `GET` routes that McHarbor does not call. |
| `OTRS rejected the configured agent login` | Wrong login or password, the agent is invalid, or it has two-factor authentication enabled | Sign in as the agent in the browser; use an agent without 2FA. |
| `OTRS rejected the session twice; check that the agent is valid` | A new session is refused straight after login, usually because the agent was invalidated, or OTRS 8 expects a different session operation | Check the agent under Admin → Agents. On OTRS 8, see the [overview](./README.md#otrs-8-and-later). |
| `dedup dynamic field "…" is not searchable; create it as a Ticket Text field` | The field does not exist, is misspelt, is not a **Ticket** field, or is invalid, so the search ignored the filter | Create the field exactly as named in **Dedup dynamic field**, of type Text, object Ticket, valid. |
| `TicketCreate.InvalidParameter: … Queue …` / `… Priority …` / `… State …` / `… Type …` | A queue, priority, state or type name that does not exist in the instance | Names are matched exactly. Fix **Queue**, **Close state** or **Ticket type**, or override renamed priorities with **Severity map**. |
| `TicketCreate.InvalidParameter: … CustomerUser …` | The customer user does not exist | Create it, or enter an existing login. |
| `… AccessDenied: the McHarbor agent lacks permission on this ticket or queue` | The agent lacks `rw` on the queue's group | Grant `rw` under Admin → Agents ↔ Groups. |
| Every firing opens a new ticket | The agent cannot read the queue (`ro`), so the dedup search finds nothing; or the dynamic field is not stored on create | Grant `ro` (included in `rw`). Check a created ticket shows the key in its dynamic field. |
| Status never changes on the Tickets tab | No invoker delivery and polling off; or the requester's secret header is missing or wrong | Open the web service **Debugger** and look at the requester's outgoing request: a `401` from McHarbor means the token in the controller path or the `X-McHarbor-Webhook-Secret` header does not match what McHarbor issued (after a rotation, both change). Otherwise set a **Poll interval** above `0`. |
| A renamed state shows as Open | A custom state whose name and type McHarbor does not know | Add it to **Extra resolved states** or **Extra cancelled states**. |
| `status 413: the ticket body exceeds the web service MaxLength` | The alert text is larger than the provider's `MaxLength` | Raise `MaxLength` in the web service's provider transport. |

**Test connection** creates nothing. It logs in, runs two searches and stops, so a
passing test does not prove that the agent may **create** in the queue, only that it can
read it. The first real alert proves the rest.

## Reading a failure

A failed delivery shows its message behind the error icon on the Tickets tab when the
plugin marked it safe to show; everything else reads as a generic failure there. The
container log carries the full chain:

```bash
docker compose logs mcharbor | grep -i comm-otrs
```

On the ITSM side, the web service **Debugger** (Admin → Web Services → the web service →
Debugger) shows every provider request McHarbor made and every requester delivery, with
the response each one got. Raise its **Debug threshold** to `debug` while you diagnose,
and lower it again afterwards: the provider log records request bodies, which include
the session.

## Related

- [Configuration](./configuration.md): every field and what it is for
- [OTRS / Znuny / OTOBO](./README.md): what it is and how it works
