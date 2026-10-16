# Incident sources

An optional contract a plugin implements to raise [incidents](../../incidents/README.md) about
what it manages (a virtual machine that crashed, a pod in a crash loop, a Swarm service that
will not converge), with its own evidence and its own judgement of when the subject has
recovered.

The contract lives in `core/plugin/specs/incidentsource`. It is a separate package, so adding
it changed no already-built plugin, and a plugin that does not implement it is unaffected.

## Which plugins implement it

| Plugin | Subject types | Signals |
|---|---|---|
| Virtual Machines (`addon-kvm`) | `virtual_machine` | `vm_crashed`, `vm_guest_panicked`, `vm_restart_loop`, `vm_paused_error`, `vm_unresponsive`, `vm_stopped_unexpectedly`, `vm_shut_down_by_guest`, `vm_guest_unresponsive`, `vm_guest_disk_full`, `vm_guest_clock_skew` |
| System Containers (`addon-lxc`) | `system_container` | `lxc_crashed`, `lxc_stopped_unexpectedly`, `lxc_start_failed` |
| Kubernetes (`addon-kubernetes`) | `k8s_pod`, `k8s_workload`, `k8s_node` | `pod_crashloop`, `pod_oomkilled`, `pod_image_pull`, `pod_unschedulable`, `pod_failed`, `deployment_unavailable`, `node_not_ready` |
| Swarm (`addon-swarm`) | `swarm_service`, `swarm_node` | `service_task_failing`, `service_not_converged`, `service_update_paused`, `service_update_rolled_back`, `node_down` |

`GET /api/incidents/sources` (`incidents.view`) lists what the loaded plugins declare, with
their labels.

## The contract

A plugin's exported type implements `incidentsource.Provider`. The host checks for it after
`Init`, registers it under the plugin's slug, and drops it when the plugin unloads.

| Method | Called | Returns |
|---|---|---|
| `IncidentSubjectTypes()` | For labels, and to rank signals | Each subject type with its signals: `Key`, `Label`, `Strength` 1 to 5 |
| `IncidentSignals(ctx, since)` | About once a minute, 20 s deadline | Every failure that is current, not only new ones |
| `IncidentEvidence(ctx, type, env, id, from, to)` | When an incident opens, and on **Refresh evidence** | `Facts` (short key/value pairs) and `Sections` |
| `IncidentSettled(ctx, type, env, id)` | For an incident quiet for thirty minutes | Whether the subject has recovered |

### Signals

- `SubjectID` must stay the same for the life of the subject: a VM id, a Deployment rather
  than one of its pods. The host keys the open incident on source, subject type, environment
  and `SubjectID`, so a rename or a pod restart does not split one failure into two.
- Report a failure for as long as it lasts. The host adds to an incident only when `At` is
  later than its last signal, so a persisting failure does not inflate the count.
- A signal with a higher `Strength` replaces a weaker one on the open incident.
- A plugin that cannot reach what it watches reports nothing for it. An unreachable host is
  not a crash.
- Queries run concurrently across plugins; one plugin failing or timing out is logged and
  skipped, and the other sources are still scanned.

### Evidence

`Sections` have a `Kind`: `log` (`Lines`), `table` (`Columns`, `Rows`) or `text` (`Lines`
as paragraphs), and an optional `Note` explaining an empty or partial section. Logs are cut
to the last 150 lines and tables to 40 rows. The host adds its own audit trail and alert
history for the subject, then redacts the whole bundle before it is stored or shown to a
model. Never put credentials, secret values or environment variable values in evidence.

### Recovery

`IncidentSettled` decides whether a quiet incident resolves. Return `false` while the subject
is still failing or cannot be checked; a user deleting or deliberately stopping it counts as
recovered. When the plugin that raised an incident is not loaded, the incident stays open, because
plugins load after the incident engine starts, and resolving then would close real incidents.

## What goes wrong

| Symptom | Cause |
|---|---|
| A plugin's failures never open incidents | The plugin build predates the contract. Update it from the Plugin Store |
| An incident never resolves | `IncidentSettled` keeps returning `false`, or the plugin is not loaded |
| Evidence shows "did not answer in time" | `IncidentEvidence` exceeded its 20 s deadline |
