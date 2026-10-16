# Incidents and ITSM

20 endpoints: incidents correlated from alerts, their evidence and model narration,
behaviour findings, and the ITSM links that mirror alerts into an external ticketing system.

| Key | Covers |
|---|---|
| `incidents.view` | Incidents, findings, narration settings |
| `incidents.manage` | Refresh evidence, narrate, summarise, resolve; dismiss and explain findings; change narration settings |
| `communications.view` / `communications.manage` | ITSM channels, their inbound token and signing secret |
| `alerts.view` / `alerts.manage` | Linked tickets, resync and retry |
| — | The inbound ITSM callback, authenticated by its token |

## Things worth knowing first

**Narration is opt-in per environment.** Nothing is sent to a model until
`PUT /api/incidents/settings` turns `narrateOptIn` on for that environment; the opt-in is
audited as a warning, because from then on evidence can leave. A refused narration is `409`
with `data.reason` `not_opted_in`, `no_model` or `in_progress`.

**The inbound webhook is public and says nothing.** `POST /api/incident-webhooks/{token}` has
no session; the per-channel token and the plugin's signature check authenticate it, and every
failure is the same `401`, so it reveals nothing about which tokens or channels exist.

**The token and secret are shown once.** `POST /api/itsm/channels/{id}/webhook-token` returns a
new inbound URL and signing secret in its response only, and breaks whatever the ITSM has
configured.

## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `POST` | `/api/incident-webhooks/{token}` | — | The public route an ITSM calls back on. Authenticated by the per-channel token and the plugin's signature check; every failure is the same `401` | `{token}` |
| `GET` | `/api/incidents` | `incidents.view` | Lists incidents, newest first | `?status=` |
| `GET` | `/api/incidents/sources` | `incidents.view` | Lists the loaded plugins that raise incidents, with their subject types and signal labels | — |
| `GET` | `/api/incidents/by-alert/{eventId}` | `incidents.view` | Returns the incident an alert event is part of; `404` when none | `{eventId}` |
| `GET` | `/api/incidents/findings` | `incidents.view` | Lists behavioural findings | `?status=` |
| `POST` | `/api/incidents/findings/{fid}/dismiss` | `incidents.manage` | Hides a finding until it clears and is found again | `{fid}` |
| `POST` | `/api/incidents/findings/{fid}/explain` | `incidents.manage` | Has the model put a finding's numbers into words | `{fid}` |
| `GET` | `/api/incidents/settings` | `incidents.view` | Returns an environment's narration settings and the model its evidence would be sent to | `?env=` |
| `PUT` | `/api/incidents/settings` | `incidents.manage` | Changes an environment's narration consent and limits; opting in is audited as a warning | — |
| `GET` | `/api/incidents/{id}` | `incidents.view` | Returns one incident with its evidence, narrative and prompt | `{id}` |
| `POST` | `/api/incidents/{id}/evidence` | `incidents.manage` | Re-assembles an incident's evidence now | `{id}` |
| `POST` | `/api/incidents/{id}/narrate` | `incidents.manage` | Asks the model to explain an incident on a person's behalf. `409` with `data.reason` `not_opted_in`, `no_model` or `in_progress` | `{id}` |
| `POST` | `/api/incidents/{id}/resolve` | `incidents.manage` | Closes an open incident now, with its timeline; `409` `not_open` when already resolved | `{id}` |
| `POST` | `/api/incidents/{id}/summarise` | `incidents.manage` | Asks the model for a post-incident summary; `409` `not_resolved` while open | `{id}` |
| `GET` | `/api/itsm/channels` | `communications.view` | Lists the channels that can open tickets | — |
| `POST` | `/api/itsm/channels/{id}/webhook-secret` | `communications.manage` | Stores a vendor-issued signing secret for inbound callbacks | `{id}` |
| `POST` | `/api/itsm/channels/{id}/webhook-token` | `communications.manage` | Issues a new inbound URL and signing secret, shown once; breaks whatever the ITSM has configured | `{id}` |
| `GET` | `/api/itsm/links` | `alerts.view` | Lists linked tickets | `?state=`, `?channel=` |
| `POST` | `/api/itsm/links/{id}/resync` | `alerts.manage` | Reads one ticket's state from the ITSM now | `{id}` |
| `POST` | `/api/itsm/links/{id}/retry` | `alerts.manage` | Makes a link's parked deliveries due again | `{id}` |

## Request bodies

**`PUT` `/api/incidents/settings`**: `envId`, `narrateOptIn`, `proactive`, `hourlyCap`

**`POST` `/api/itsm/channels/{id}/webhook-secret`**: `secret`

## Related

- [Incidents](../../incidents/README.md): detection, narration, behaviour findings
- [Tickets](../../settings/monitoring/alerts/tickets/README.md): ITSM channels and links
- [Dashboard, audit, alerts and notifications](./monitoring.md): alert rules and channels
