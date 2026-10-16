# Workflow Troubleshooting

What goes wrong with a workflow, grouped by how it fails: it never runs, it runs but not
the way you expect, it fails outright, nodes are missing from the palette, or it is slow.
Two catch almost everyone: Inactive arms nothing automatic, and the Schedule Trigger
reads its own `timezone` field rather than the app-wide one.

## It never runs

| Symptom | Usual cause | Fix |
|---|---|---|
| No runs at all on a scheduled workflow | The workflow is **Inactive** | Active arms automatic triggers; manual runs work either way |
| A schedule fires an hour off | The **Schedule Trigger's own `timezone` field** | It does not read the app-wide time zone the way other schedules do |
| A webhook returns 404 | The workflow is inactive, or the path does not match | Both are checked before anything runs |
| A webhook returns 401 | The trigger has a secret and the request did not present a matching one | Send the secret, or clear it, knowing an unsecured webhook is an open entry point |
| A webhook returns 400 | The payload would not parse | The body has to be readable as the trigger expects |
| A container trigger never fires | The filter does not match, or events are not reaching McHarbor | On agent environments, check the event collection mode in Settings → System → Agent |
| The run fails with `no runnable entry nodes` | No trigger, or the named start node is missing | Every workflow needs an entry node |
| The run fails with `invalid workflow canvas` | The stored canvas JSON will not parse | Usually a failed import; re-import the export |

## It runs, but not the way you expect

| Symptom | Usual cause | Fix |
|---|---|---|
| A branch never executes | The node before it is **disabled**, or its group is **blocked**, or the output port is blocked | Disable stops the chain; skip passes through |
| A value is missing downstream | The configured path does not match what the previous node emitted | Read the Debug tab on the receiving node; it shows the real `msg` |
| An array path returns nothing | Paths walk maps only; `payload.items.0` does not index | Use **Loop**, **Range** or **JSON Path** |
| A change in one branch is invisible in another | Every branch gets a deep copy | Put shared values in `flow` scope with **Set Variable** or **Change** |
| A function or a Date vanished from `msg` | Messages are copied by JSON round trip | Only JSON-representable data survives a hop |
| Both branches of a Condition fire | Two edges leave the same port | Check which port each edge actually leaves |

## It fails

| Symptom | Usual cause | Fix |
|---|---|---|
| Status is `completed` but something is wrong | **A failed node does not fail the run** | Read the Error column and the node's Errors tab |
| `workflow exceeded 2000 execution steps` | A cycle, or per-item iteration over a long list | Restructure to handle a batch per run |
| Status is `cancelled` | The five-minute run timeout, or a stop | Split long work, or move the slow part outside the workflow |
| A node fails and nothing catches it | No enclosing **Try Catch** with something wired to `catch` | A guard only catches when its `catch` port has an edge |
| An `error` port is wired and never fires | The node failed outright rather than emitting on `error` | Both paths reach a try-catch guard; only one reaches the `error` edge |
| A custom node fails with "script timed out" | The script exceeded its timeout | 10s by default, 60s maximum. An accidental infinite loop lands here |
| A custom node fails on its return | It returned something other than `{port, msg}` or a modified `msg` | Those are the only two accepted shapes |
| A custom node cannot reach the network | **There is no host bridge**: no `fetch`, no `require` | Use an HTTP Request node and shape the data in the custom node |
| Saving answers **Save refused**, `err.host_self.workflow_required` | A node acts on McHarbor's own containers, stacks or host, and you do not hold `host_self.manage` | See [Self-protection](../self-protection/README.md) |
| A node fails with `err.host_self.workflow_unauthorized` | The workflow was last saved by someone without `host_self.manage`, and the node reached McHarbor's own | Someone holding the key saves the workflow again. See [Self-protection](../self-protection/README.md) |

## Nodes are missing

| Symptom | Usual cause | Fix |
|---|---|---|
| A node shows as **Unavailable** | Its dependency is not configured | The node says which: an email server, a communication channel, or an environment |
| A node is absent from the palette entirely | It is toggled off in the Store's Nodes tab | Overrides live in `$DATA_DIR/workflow-nodes/state.json`; an absent key means enabled |
| A plugin node is on the canvas but will not run | The plugin is uninstalled or disabled | The wiring is kept deliberately; reinstalling restores it |
| A plugin's node never appears | It is registered in code but missing from the manifest | The loader mounts only what `contributes.nodes` declares |
| An old workflow references a renamed node | Legacy keys are rewritten at execution time | Open the workflow and accept the migration prompt to persist the rewrite |

## Performance

| Symptom | Usual cause | Fix |
|---|---|---|
| A workflow is slow and no single node looks slow | The per-node timings are on the message | Read `_performance` on the output, or the duration in each node's Debug tab |
| Memory spikes on a big payload | Every outgoing edge gets a deep copy | A 50 MB payload fanned to four branches copies 200 MB. Narrow the payload before the fan-out |
| The run list fills with near-identical runs | A trigger is firing per event on a busy host | Tighten the trigger's filter, or use a **Rate Limit** node |

## When you cannot tell where it went wrong

1. Run it manually and watch the canvas. Execution stops visibly where it stopped.
2. Open the **Debug** tab on the last node that ran. Input, config and output are all there.
3. Put a **sniffer** on each branch out of the deciding node and run again.
4. Add a **Debug** node at the point you are unsure about, rather than reasoning about it.
5. Check the run's **Error** field even on a completed run.

## Related

- [Execution model](../execution/README.md): limits, error routing, skip and disable
- [Self-protection](../self-protection/README.md): nodes that act on McHarbor's own containers
- [Runs and debugging](../runs/README.md): the four debug surfaces
- [The message contract](../messages/README.md): paths, copies and reserved keys
