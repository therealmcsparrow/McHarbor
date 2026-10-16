# Workflows: Self-protection

A workflow runs for the system, not for a user. When a schedule, a webhook or a trigger starts a
run, nobody is there to hold a permission. So the `host_self.manage` question is asked when a
workflow is **saved**, and the answer is stored on the workflow. The run then acts on McHarbor's
own containers, stacks, volumes or host only when that stored answer is yes.

This is the workflow half of [Containers → Self-protection](../../screens/containers/self-protection/README.md),
which defines what counts as McHarbor's own and lists every other path the key guards.

## Which nodes it covers

| Node | Operations checked |
|---|---|
| `container-action` | `start`, `stop`, `restart`, `remove`, `rename` |
| `container-exec` | Any command. This also covers a container that reaches McHarbor's host |
| `container-create` | The spec the node would create, judged by the create-time rules |
| `host-exec` | Always. Its helper runs in the host's PID namespace |
| `network-manage` | `connect` and `disconnect` |
| `stack-manage` | `deploy` (the compose content), `up`, `down`, `remove` |
| `volume-manage` | `remove` and `restore` of a volume a McHarbor container mounts |
| `container-backup-restore` | Restoring into a McHarbor container, or one that reaches the host |

Other nodes, and other operations on these nodes, are not affected.

## When a workflow is saved

Saving the canvas (`PUT /api/workflows/{id}`) and importing an export (`POST /api/workflows/import`)
both run the check. Creating a workflow does not, because a new workflow starts with an empty
canvas. Deleting one never needs the key.

1. **The saver holds `host_self.manage`.** The save goes through and the workflow is marked
   authorized.
2. **The saver does not hold it, and a node's target is McHarbor's own.** The save is refused with
   `403` `err.host_self.workflow_required`, and `data.reasons` names the node id, then
   `<node id>:<reason>` for each reason. The refusal is audited as `host_self.denied` on the
   workflow, with `op=update` or `op=import`.
3. **The saver does not hold it, and no target is McHarbor's own.** The save goes through and the
   workflow is marked not authorized.

A target counts as known at save time only when it is written into the node. A container, stack
or volume name that contains a `{{ }}` template or a `${secret:…}` reference, a message-supplied
environment, or compose content read from a file or the message can only be resolved by a run.
Those nodes do not block the save. The run checks them instead.

When the save-time check cannot reach the environment, it does not refuse the save. It logs the
problem and marks the workflow not authorized, so a run refuses the node if it turns out to be
McHarbor's.

Every save evaluates the rule again. When someone without the key edits a workflow that was
authorized, it loses the mark, even if they changed nothing near a guarded node.

## The mark

The stored value is `selfHostAuthorized` on the workflow (`workflows.self_host_authorized`,
migration 129). Only the server sets it, from the last saver's permissions. The request body
cannot set it.

The editor toolbar shows a **Self-host authorized** badge while it is set. Its tooltip reads
"Last saved by someone holding host_self.manage, so its nodes may act on McHarbor's own
containers, stacks and host." With the mark cleared, the badge is not shown.

A refused save stays on the canvas as a **Save refused** alert that carries the server's message,
so the key to ask for is still visible after the toast has gone.

## When a workflow runs

In an **authorized** workflow, the guarded nodes run as they always did, whoever or whatever
started the run.

In a workflow that is **not** authorized, each guarded node classifies its resolved target just
before it acts:

- Not McHarbor's own: the node proceeds.
- McHarbor's own: the node fails with `err.host_self.workflow_unauthorized` and nothing is changed.
  The run log shows the translated message, followed by `[err.host_self.workflow_unauthorized
  target=<kind> reasons=…]`.
- Cannot be classified, for example because the daemon cannot be reached or a compose file cannot
  be read: the node is refused as `target=unknown`. An unknown target could be McHarbor's.

The failure follows the normal [error routing](../execution/README.md#errors). A **Try Catch**
guard catches it, and without one the rest of the queue carries on.

A container node acts on the container ID it inspected, so a name that is pointed at another
container between the check and the call is not followed.

## What goes wrong

| Symptom | Cause | Fix |
|---|---|---|
| **Save refused**, `err.host_self.workflow_required` | A node's target is written into it and is McHarbor's own | Have someone with `host_self.manage` save it, or point the node elsewhere |
| A node fails with `err.host_self.workflow_unauthorized` | The workflow was last saved without the key, and the resolved target is McHarbor's own | Someone with `host_self.manage` opens the workflow and saves it again |
| A workflow that used to work on McHarbor's containers now fails there | Someone without the key edited it and cleared the mark | Same |
| The badge disappeared after a small edit | Every save evaluates the rule again | Same |
| A node fails as `target=unknown` | The environment or the compose file could not be read at run time | Fix the connection or the file. The node is refused until the target can be classified |

## Related

- [Containers → Self-protection](../../screens/containers/self-protection/README.md)
- [Execution model](../execution/README.md)
- [Troubleshooting](../troubleshooting/README.md)
- [Permission conventions](../../api/conventions/permissions.md)
