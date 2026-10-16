# Incidents

`/incidents`

When a container, a virtual machine, a pod or a Swarm service starts failing, McHarbor already holds everything that explains it: the
Docker events, the logs, who changed what, which deploy just landed, the alerts, the
metrics, and whether it has drifted from its desired state. An **incident** puts all of it
on one screen for the window around the failure. Optionally, the AI model you configured
can read that evidence and write a short narrative: what happened, what changed shortly
before, and the most likely causes with the evidence for each.

Nothing here changes anything. The incident view and the narrator only read.

## Permissions

| Key | What it unlocks |
|---|---|
| `incidents.view` | The incident list, each incident's evidence and narrative, the behaviour findings, the narration settings, the narrative on an on-call alert |
| `incidents.manage` | **Explain this**, **Summarise**, **Refresh evidence**, **Resolve**, explaining and dismissing findings, and turning narration on or off for an environment |

`incidents.manage` can send evidence off the host, which is why it is a key of its own.

## What opens an incident

| Signal | When |
|---|---|
| **Crash loop** | Three failing exits (non-zero exit code) within ten minutes |
| **Repeated failing exit** | Two failing exits within thirty minutes |
| **Out of memory** | The kernel killed the container for memory |
| **Unhealthy** | The container's health check reported it unhealthy |
| **Alert fired** | A container alert rule fired for it |

### Raised by plugins

Plugins that implement the [incident source contract](../plugins/incident-sources/README.md)
raise incidents about what they manage, with signals, labels and evidence of their own:

| Plugin | What it watches | Examples |
|---|---|---|
| Virtual Machines | Virtual machines | Crashed, guest panicked, stopped without anyone in McHarbor stopping it (killed on the host, or shut down from inside the guest), restart loop, paused on a disk error, QEMU not responding; with the guest agent installed: guest OS frozen, a filesystem inside the guest nearly full, clock drift |
| System Containers | LXC containers | Crashed, stopped unexpectedly, failed to start |
| Kubernetes | Pods, Deployments, nodes | Crash loop, killed for memory, image pull failing, unschedulable, Deployment unavailable, node not ready |
| Swarm | Services, nodes | Tasks failing, not converged, update paused or rolled back, node down |

The Virtual Machines plugin records every start, shutdown, kill, reset, pause and delete
McHarbor performs (from the page, a workflow or a schedule, with who asked) and QEMU's own
reason for each stop, so a stop nobody asked for is told apart from a deliberate one. Both
appear in the incident's evidence as **Lifecycle actions** and **QEMU events**. The guest
checks (frozen guest, full disk, clock drift) need the host's agent from this build or later
and the QEMU guest agent inside the VM; with an older host agent the evidence says so and
those signals stay quiet.

Their evidence is the plugin's facts and sections (the QEMU log of a VM, a pod's
Kubernetes events and previous-container log, a service's recent tasks), plus the audit
trail and alerts for the subject. They resolve when the plugin reports the subject
recovered and it has been quiet for thirty minutes; while the plugin is not loaded, its
incidents stay open.

One incident is open per container at a time. A later signal extends it; a stronger one (a
repeated exit becoming a crash loop) replaces its signal. A clean exit (code 0) is never a
signal. The signals are read from the events McHarbor already records, so an environment
reached through an agent is covered too, within the delay of its event collection.

An incident is **resolved** once the container has been quiet for thirty minutes and its
last event is not a failure. A container that died and stayed down keeps its incident open.
**Resolve** closes one by hand. Either way a timeline is built: the first symptom, the
container's lifecycle, what people did, the alerts, the last change before it recovered,
and how long it lasted.

## The incident view

- **What happened**: the narrative, when there is one, with its overall confidence, the
  changes shortly before, and up to three probable causes, each with the evidence it rests
  on and its own confidence. When the model cannot tell, it says so, and so does the page.
  Under the narrative: which model wrote it, through which connection, for whom, when, and
  **What the model was shown**: the exact text that was sent.
- **Timeline**: for a resolved incident, with an optional model-written **Summarise**.
- **Evidence**: the container's state, its events, its last log lines, image changes, who
  changed what, deploys to the environment, alerts, drift, and metric peaks, for the window
  from thirty minutes before the first signal. **Refresh evidence** takes it again.

## In this folder

| Page | What it covers |
|---|---|
| [Narration](./narration.md) | Opting in, exactly what is sent and what is removed first, limits, attribution, why a narration was refused |
| [Behaviour findings](./behaviour.md) | The four pattern detectors and their thresholds |
| [Runbooks in a timeline](./runbooks/README.md) | How a recorded runbook run replaces the "what finally fixed it" guess, and why there is no append path |
| [Reference](./reference.md) | Every route and the key it needs |

## Related

- [On-call](../oncall/README.md): the narrative on an alert, on a phone
- [Reconciler](../screens/reconciler/README.md): drift, which is part of the evidence
- [AI assistant](../settings/README.md): where the model is configured
