# Developing for Workflows

Three ways to add a node, and they are not interchangeable.

| Way | Language | Needs a rebuild | Can reach the host | Use it when |
|---|---|---|---|---|
| [Custom node](./custom-nodes/README.md) | JavaScript, in the interface | No | **No** (sandboxed) | Reshaping, validating or branching on data |
| [Core node](./core-nodes/README.md) | Go, in McHarbor | Yes | Yes | The node belongs in the product |
| [Plugin node](./plugin-nodes/README.md) | Go, in a plugin | Plugin rebuild | Yes, with the plugin's handles | The node belongs to an integration |

The rule of thumb: if the work is *data*, write a custom node. If the work needs credentials,
a Docker client, the database or another service, it needs Go: in the product if it is
general, in a plugin if it belongs to one integration.

## What every node is, at minimum

- **A key**: lowercase and hyphenated, or `plugin:<slug>:<key>` for a plugin. The canvas
  stores it and the runtime dispatches on it.
- **A config schema**: the fields the editor renders, with types, defaults, options and
  conditional visibility.
- **Ports**: inputs it accepts, outputs it can emit on. Triggers have no input; most actions
  carry `output` and `error`.
- **An execution**: something that takes `msg` and config and returns a port and a message.

## The contract it has to honour

Whatever you write, it participates in the same [execution model](../../workflows/execution/README.md):

- Every downstream message is a **deep copy**, so mutating the incoming message affects only
  your branch.
- **Return a port and a message**, or an error. Returning an error with no emissions is what
  routes the message into an enclosing try-catch guard.
- **Per-run state belongs on the run**, not in package-level variables, because concurrent runs share
  the process.
- A run is capped at **2000 node executions** and **five minutes** by default. A node that
  blocks for a long time spends the whole run's budget.
- Anything you put on `msg` must survive a JSON round trip.

## Documenting it

A node's **Help** tab is its documentation, and it comes from the node's own README in the
catalog folder. A node with an empty Help tab is a node people configure by trial and error.

Write down what the node puts on `msg`. That is the part nobody can infer from the field list,
and the part every downstream node depends on.

## Related

- [Execution model](../../workflows/execution/README.md): ports, emissions, errors and limits
- [The message contract](../../workflows/messages/README.md): `msg`, paths, scopes and reserved keys
- [Reference](../../workflows/reference/README.md): the API routes and the data model
- [Node catalog](../../workflows/nodes/README.md): the 90 that already exist; check before adding
