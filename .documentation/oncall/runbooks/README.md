# Runbooks

A runbook is an ordered set of steps attached to an alert type, a container, a stack, an
environment, or (while its plugin is loaded) a virtual machine, a system container, a swarm
service or a Kubernetes deployment. It turns what one person knows at 03:00 into something
anyone holding the keys can follow, and it records what they actually did.

A **run** is one person following one runbook once: who, when, which steps they completed,
what they found, and where they stopped. The last of those is the point. A workflow succeeds
or fails; a runbook is deviated from, skipped, paused, abandoned and escalated, and the
deviation is the record.

## Permissions

| Key | Scope | What it unlocks |
|---|---|---|
| `runbooks.view` | environment | Reading a runbook, its steps, and the history of runs against it |
| `runbooks.manage` | global | Authoring and editing runbooks, including which action a step may take |
| `runbooks.run` | environment | Starting a run, completing a step, skipping a step, finishing a run |
| `runbooks.automate` | global | Making a runbook run unattended as a service principal, and choosing its triggers; see [Unattended runs](./unattended.md) |

**`runbooks.run` does not grant what a step does.** A step that restarts a container still
requires `containers.manage`, and that check happens every time the step is pressed, against
the person pressing it. `runbooks.run` is permission to *follow* a runbook, not permission to
do what it says. Granting it to the whole rota is safe; it widens nobody's reach.

## Step kinds

| Kind | What it is | Permission checked |
|---|---|---|
| **Instruction** | A sentence and a checkbox. "Check the dashboard before you touch anything." | `runbooks.run` only |
| **Diagnostic** | A named read-only reading, run on demand, with its output shown inline | the diagnostic's own key, per step |
| **Action** | A named change, behind a confirmation, executed as you and audited | the action's own key, per step |

A step never carries a free-text command. Both the diagnostic and the action list are
allow-lists declared in the backend, each entry naming the permission it needs, and the
authoring editor shows that key beside the entry. A step whose key is not in the list fails
closed rather than running something unexpected.

| Diagnostic | Reads | Needs |
|---|---|---|
| `container.state` | Status, running, restarting, exit code, OOM kill, health, restart count | `containers.view` |
| `container.logs` | The last 150 lines, timestamped | `containers.view` |

| Action | Does | Needs |
|---|---|---|
| `container.restart` | Restarts the run's container | `containers.manage` |
| `container.stop` | Stops it | `containers.manage` |
| `container.start` | Starts it | `containers.manage` |

The Virtual Machines, System Containers, Compute, Swarm and Kubernetes plugins add their own
diagnostics and actions (`vm.start`, `ct.freeze`, `swarm.service.scale`,
`k8s.deployment.restart` and 27 more), each checked against the plugin's own key, first by the
runbook and then by the plugin's route. They are offered only while the plugin is loaded. See
[Plugin steps](./plugin-steps.md) for every entry, its route and its key.
`mcharbor runbook catalog` lists what the server offers right now.

Diagnostic output is redacted for socket paths and endpoints, and capped at 16 KiB with
`[truncated]` on the end, so a noisy container cannot make the run row the largest one in the
database.

## A step runs as you, while you are there

When you follow a runbook, a step is
authorised with `HasPermission` against your user id and the request you are making, which
means an API key driving a step is narrowed to that key's own set, and a temporary elevation
folds in only for an interactive browser session, not for a key. A step that needs an
elevation therefore works from the phone and refuses from a script, which is the correct
answer and is the message shown.

When you do not hold what a step needs, three things happen and all three are deliberate:

1. The control renders **disabled**, not hidden, with a tooltip naming the key.
2. If it is reached anyway, the server answers `403` with `err.runbook.step_forbidden` and
   the missing key in the payload, and the interface shows *Requires the
   &lt;key&gt; permission*.
3. **No call is made to the environment.** The check happens before the runtime is resolved.

A permission revoked mid-incident stops the next step. A run authorised at step 1 is not
still authorised at step 9.

A runbook can also run **unattended**, on its alert or a schedule, as a *service principal*:
a principal with its own roles that never signs in. The same per-step check applies, against
the principal instead of a person, and no elevation ever folds in. See
[Unattended runs](./unattended.md).

## Following one on a phone

The on-call layout is where a runbook is actually used: one step visible at a time, the
primary action at the bottom, full-width targets, no hover-only affordance. Reach it from the
alert in the inbox. An alert with a runbook attached carries its name and a single tap.

If somebody has already started a run against that alert, the banner says who and when and
offers **Rejoin this run** instead of starting a second one.

Each step carries a **What you found** field. It is optional and it is the most valuable thing
in the whole record, because it is what the post-incident summary quotes.

Stopping is a first-class outcome:

- **Completed**: you got to the end.
- **Escalate**: it is beyond this runbook.
- **Abandon**: you stopped. An abandoned run with a reason is worth more to the next person
  than a finished one without.

A finished run cannot be relabelled and no further step can be added to it. Start a new run.

## Offline

`GET /api/runbooks/for` is on the offline read allow-list, so a runbook and all of its steps
come back from the service worker's read cache when the phone has no signal, with the
*Showing saved data from …* marker that every cached on-call screen carries. The cache is
dropped on any session expiry, so a runbook is not long-lived offline storage.

**Writes are never queued.** Completing a step is a `POST`, and a queued completion would
record an action whose permission check happened at an unknown time. Offline, the runbook is
readable and every control that records something is disabled, with *You are offline. You can
read this runbook, but a step cannot be recorded until you are back online.*

## Authoring

Authoring lives on the desktop, at `/runbooks`. Nobody writes a runbook on a phone, and
pretending otherwise costs the running surface its one-handed shape.

The list itself reads as either a **table** or **cards**, toggled in the page head and remembered
for you. Both are searchable, and the table sorts and offers column and density controls. A
runbook that runs unattended shows its **readable schedule** (*every day at 07:00*) rather than
a bare cron expression, and that readable form is what search and sort read, because nobody types
`0 7 * * *` into a search box.

**New runbook** in the page head, and **Edit** on a row, open the editor as a dialog over the
list. The address follows it (`/runbooks?new=1` and `/runbooks?edit=<id>`), so a link opens
the editor directly, and an old `/runbooks/<id>` link still does. Closing a dialog with unsaved
changes, by **Cancel**, the close button or Escape, asks **Discard your changes?** first.
Creating and editing need `runbooks.manage`; without it the buttons render disabled, naming
the key.

| Field | What it does |
|---|---|
| **Name**, **Description** | What the operator sees on the alert |
| **Environment** | The environment the runbook belongs to, or **Every environment** |
| **Attach to** | `Alert type`, `Container`, `Stack`, `Environment`, and (while the plugin is loaded) `Virtual machine`, `System container`, `Swarm service` or `Kubernetes deployment` |
| **Alert source type** + **Alert condition key** | Chosen from the alert conditions the server knows, built-in and plugin. The durable attachment: it survives an alert rule being deleted and recreated |
| **What it is attached to** | For a runbook written for one object: chosen from the containers, stacks, VMs, system containers, swarm services or deployments of the environment |
| **Enabled** | A disabled runbook is never offered |

Every field whose value comes from a known set is a selector. Where the set can be incomplete
(an object that does not exist yet, a plugin that cannot list right now, an alert source
you lack `alerts.view` for), the selector's last option is **Enter manually…**, which turns it
into a text field, and **Pick from list** turns it back.

Steps are ordered, moved up and down, and each one carries a kind, a title, a body, and,
for a diagnostic or an action, the entry it names, with that entry's permission shown, and
the entry's typed **Parameters**: a selector for the object it acts on, a selector for an
option, a number field for a count, a switch for a flag. The server refuses a parameter the
entry does not declare and a value outside its range. The position is stored as an explicit
number, so reordering a runbook later never changes what a finished run says happened.

A step's target (its container, VM, system container, service or workload) is the run's own
subject unless the author pinned one, so a single runbook attached to an alert condition serves
every object that condition can fire for.

## Exporting one

Each row and card carries an export control: the runbook as portable JSON to hand to another
McHarbor, and, while the Reporting plugin is loaded, as a PDF or a Word document for the binder.
The JSON deliberately carries no ids, environment, author or service principal. See
[Exporting a runbook](./export.md).

## Auditing

Every start, every step and every outcome writes its own audit row, with `runbook_run` as the
entity type, the run's id as the entity, and **the real operator as the actor**. The incident
narrator does not do this (its rows carry a NULL actor), and a runbook step whose actor is
unknown would be worthless.

| Action | Written when |
|---|---|
| `runbook.run.start` | A run is opened |
| `runbook.step.complete` | An instruction is ticked |
| `runbook.step.diagnose` | A diagnostic ran |
| `runbook.step.act` | An action ran |
| `runbook.step.skip` | A step was skipped, with the reason |
| `runbook.step.fail` | A step ran and failed |
| `runbook.run.completed` / `.abandoned` / `.escalated` | The run ended |
| `runbook.step.denied` | An unattended step the service principal does not hold the key for |
| `runbook.save`, `runbook.delete` | Authoring, with the runbook as the entity |
| `runbook.automation.save`, `runbook.automation.clear` | Unattended runs turned on, changed or off, with the runbook as the entity |

Authoring rows name the runbook; execution rows name the run. That keeps "who changed the
procedure" separable from "who did what during the incident".

## What this is not

It is not a workflow, and the reason is structural rather than a preference. A workflow step
has no principal: `FlowContext.UserID` is empty for every scheduled, webhook, event and
link-triggered run, and the Docker nodes never read it; they hold the daemon's own access.
The engine checks permissions once, at the route, and then everything runs as the service. A
runbook's central requirement is the opposite. Making every node per-step-authorised would
change the semantics of all ~40 node types and of every unattended workflow already running.
The two engines want incompatible defaults, and a runbook is the one that must not become
automatable by accident.

A runbook *can* be automated on purpose: by someone holding `runbooks.automate`, as a named
service principal whose roles are visible in the role editor, with every step still checked
per step. That is the difference from a workflow: an unattended runbook has a principal. For
a multi-system automation with branching and retries, use a [workflow](../../workflows/README.md).

## Related

- [Unattended runs](./unattended.md): service principals, alert and schedule triggers, where
  an unattended run stops
- [Exporting a runbook](./export.md): the JSON file format, the PDF and Word documents, and
  what an export leaves behind
- [Plugin steps](./plugin-steps.md): VM, system container, compute, swarm and Kubernetes steps,
  their routes and keys, and attaching a runbook to a plugin object
- [Incidents and runbooks](../../incidents/runbooks/README.md): how a run reaches the
  post-incident timeline
- [Offline and stale data](../offline.md): the read cache these pages use
- [Command-line client](../../cli/README.md): `runbook list`, `runbook get`, `runbook runs`,
  `runbook principals`, `runbook automate`, `runbook catalog`
