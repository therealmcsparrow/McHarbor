# Incidents: Reference

| Method | Path | Permission | What it does |
|---|---|---|---|
| `GET` | `/api/incidents` | `incidents.view` | Incidents, newest signal first; `?env=`, `?status=open\|resolved`, pagination |
| `GET` | `/api/incidents/{id}` | `incidents.view` | One incident with its evidence, narrative, prompt and timeline |
| `GET` | `/api/incidents/by-alert/{eventId}` | `incidents.view` | The incident an alert event belongs to; 404 when none |
| `POST` | `/api/incidents/{id}/evidence` | `incidents.manage` | Takes the evidence again |
| `POST` | `/api/incidents/{id}/narrate` | `incidents.manage` | Has the model narrate it; refusals answer 409 with `data.reason` `not_opted_in`, `no_model` or `in_progress`, a model failure 502 with `model_failed` |
| `POST` | `/api/incidents/{id}/summarise` | `incidents.manage` | Post-incident summary of a resolved incident's timeline; 409 `not_resolved` when open |
| `POST` | `/api/incidents/{id}/resolve` | `incidents.manage` | Closes it now with its timeline; 409 `not_open` when already resolved |
| `GET` | `/api/incidents/settings` | `incidents.view` | `?env=`: the narration settings and the model evidence would go to |
| `PUT` | `/api/incidents/settings` | `incidents.manage` | `{envId, narrateOptIn, proactive, hourlyCap}` (cap 1 to 100) |
| `GET` | `/api/incidents/findings` | `incidents.view` | Behaviour findings; `?env=`, `?status=open\|dismissed\|cleared` |
| `POST` | `/api/incidents/findings/{fid}/explain` | `incidents.manage` | Has the model explain a finding's numbers |
| `POST` | `/api/incidents/findings/{fid}/dismiss` | `incidents.manage` | Hides a finding until it clears |

Detection, resolution, automatic narration and behaviour detection run on the elected
leader; the scan runs every 30 seconds, the detectors every hour.

## Audit events

`incident.opened`, `incident.resolved`, `incident.narrated`, `incident.summarised`,
`incident.settings.updated` (warning when narration is turned on),
`incident.finding.explained`, `incident.finding.dismissed`.

## Related

- [Incidents](./README.md)
