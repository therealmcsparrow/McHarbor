# Unattended runs

A runbook is followed by a person by default. It can also run **unattended** (when its alert
type fires, on a schedule, or both), and when it does, it acts as a **service principal**: a
real principal with its own role assignments, audited under its own name, that can never sign
in. Nothing about a person's run changes.

## Permissions

| Key | Scope | What it unlocks |
|---|---|---|
| `runbooks.automate` | global | Turning unattended runs on, changing the principal or the triggers, and turning them off (`PUT /api/runbooks/{id}/automation`); listing service principals for the picker (`GET /api/runbooks/principals`); and editing the steps of a runbook that already runs unattended |
| `users.manage` | global | Creating a service principal (Security → Users → **Service principal**) and changing its role assignments |
| `runbooks.view` | environment | Reading unattended runs like any other (`GET /api/runbooks/runs?startedBy=<id>&trigger=schedule`) |

`runbooks.automate` decides **which principal acts**, not what it may do. What an unattended
run may do is exactly what the service principal's roles grant, checked per step.

Editing the steps of an automated runbook needs `runbooks.automate` as well as
`runbooks.manage`; without that, `runbooks.manage` alone could change what a principal
executes overnight. The refusal is `403` with `err.runbook.automate_required` and the key in
the payload.

## The service principal

A service principal is a row in the users list with the kind `service`, labelled
**Service principal** beside its name. It is created with a name and a role and nothing else:

- **No password.** The create form has no password field, and every password path (change,
  admin reset, forced reset, reset link) refuses it.
- **No session.** Signing in answers exactly like a wrong password, and no sign-in path
  (password, identity provider, passkey, device flow) can issue it a session.
- **No API key.** A key it somehow owned would not authenticate.
- **No elevation, no break-glass.** An approved elevation or an open break-glass session
  naming a service principal grants nothing. What its roles say is what it holds.

Its roles are ordinary assignments, shown and edited on its detail page like anyone's. Give it
only what its runbooks need: a principal that restarts containers needs `containers.manage`
in those environments and `runbooks.run`, and nothing else. To stop every unattended run it
makes at once, disable it; the engine records *the service principal is missing or disabled;
nothing ran* on each runbook assigned to it.

## Turning it on

On the runbook's editor page, under **Unattended runs**:

| Field | What it does |
|---|---|
| **Service principal** | Who the run acts as. Only active service principals are offered; the server refuses a person with `err.runbook.principal_invalid` |
| **When its alert fires** | Starts one run for every new firing alert of the runbook's alert type or rule. Only a runbook attached to an alert type can use it (`err.runbook.automation_alert_attach`) |
| **On a schedule** | A readable schedule with an advanced cron field underneath. The server parses the expression and refuses one it cannot (`err.runbook.cron_invalid`). The next run is shown beside it |

At least one trigger is required (`err.runbook.automation_no_trigger`). **Turn off** clears the
principal and both triggers.

The alert trigger answers only alerts that fired after it was turned on, and never one older
than ten minutes, so an engine that was down for an hour does not restart containers for old
news. Each alert is answered once: the run itself is the record, so a restart or a new cluster
leader does not start a second.

A schedule is read in the server's time zone, like backup schedules.

## Where an unattended run stops

An unattended run executes the steps in order, each through the same gate a person's run
uses, and stops at the first of these:

| It meets | The run ends | What is recorded |
|---|---|---|
| An **instruction** step | `escalated` | The note names the step: *stopped at step 3 (Call the owner): an instruction needs a person*. Nothing after it runs |
| A step the principal lacks the key for | `abandoned` | The step is recorded `failed` with *the service principal &lt;name&gt; does not hold &lt;key&gt;*, and **no call is made to the environment** |
| A step that ran and failed | `abandoned` | The step is recorded `failed` with the environment's own message |
| The end | `completed` | — |

A runbook whose **first** step is an instruction cannot be automated at all
(`err.runbook.automation_first_step`): every run of it would stop before doing anything. Put a
diagnostic or an action first; an instruction later in the runbook is where a person takes
over.

When the last unattended run was abandoned, its reason is shown on the editor page behind the
error icon beside **Last run**, in full.

## Auditing

Every row an unattended run writes carries the **service principal's id** as the actor (the
start, each step, each denial and the outcome), with the run as the entity. It is never NULL
and never a label. The run's **Started by** is the principal and its **trigger** is `alert` or
`schedule`; a person's run is `manual`.

| Action | Written when |
|---|---|
| `runbook.step.denied` | An unattended step was refused for a missing key |
| `runbook.automation.save` | Unattended runs were turned on or changed, by the person who did it |
| `runbook.automation.clear` | They were turned off |

## In a cluster

The engine runs on one node at a time, under the singleton role `runbook-automation`, which
appears in the cluster status beside the other singletons. Its state is the schedule's next
run time and the runs table, so a failover loses nothing and starts nothing twice.

## Related

- [Runbooks](./README.md): the engine, step kinds and the per-step check
- [Service principals](../../screens/security/service-principals/README.md): creating and governing one
- [Command-line client](../../cli/README.md): `runbook principals`, `runbook automate`,
  `runbook runs --started-by --trigger`
