# Workflows: Fields

Every column, filter and form field on Workflows, plus the four tabs of the node
configuration panel. Nodes hand each other a `msg` object: the Help tab documents what a
node puts into it, and the Debug tab shows what actually arrived.

## Columns

| Column | What it is |
|---|---|
| Status | Run outcome |
| Trigger | What started it |
| Nodes | How many executed |
| Duration | How long it took |
| Started | When |
| Error | The failure, where there was one |

## Filters and facets

| Filter | What it narrows |
|---|---|
| Filter runs | Free text over the run list |
| Search nodes | In the palette, while editing |

## Form fields

| Field | Type | What it is |
|---|---|---|
| Name | text | Required: "Workflow name is required." |
| Description | text | Optional |
| Label | text | Per node, on the Settings tab |
| Port labels | text | Per node output port |
| Skip node | toggle | "Pass input through without executing" |
| Disable node | toggle | "Block the chain at this node" |
| Debug | toggle | Emit this node’s message to the debug stream |

## The node configuration panel

| Tab | What is there |
|---|---|
| **Settings** | The node’s own fields, plus its label and port labels |
| **Help** | Requirements, behaviour and technical details for that node type |
| **Debug** | The execution trace and the payload: input, config, output, data |
| **Errors** | Errors from execution, or "No errors." |

Nodes hand each other a `msg` object, referenced with dot-notation paths in expression
fields. The Help tab documents what a node puts into `msg`; the Debug tab shows what
actually arrived.

## Shared list behaviour

Every list screen inherits the same controls: free-text search, a filter mode of
**Contains**, **Exact** or **Regex** (an invalid expression says so rather than silently
matching nothing), saved filters and searches, selection with a bulk bar, and pagination
showing "X to Y of Z" with a rows-per-page control.

## Related

- [Workflows](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what you can do with these fields
