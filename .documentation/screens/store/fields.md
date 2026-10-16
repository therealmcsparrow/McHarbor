# Store: Fields

Every column, filter and form field on the Store: what an app declares in the catalog, the
badges that mark where a node or widget came from, and the six steps of the install
wizard.

## Columns

| Column | What it is |
|---|---|
| Name | App, node, widget or plugin |
| Category | Catalog grouping |
| Ports, Volumes, Environment variables | What an app declares |
| Badge | Built-in, Downloaded or Plugin, on nodes and widgets |

## Filters and facets

| Filter | What it narrows |
|---|---|
| Search | Name and description |
| Installed only | Narrows apps to what is already installed |
| Search nodes / Search widgets | On those tabs |

## Form fields

| Field | Type | What it is |
|---|---|---|
| Stack name | text | What the installed app is deployed as |
| Environment | select | Where to install it |
| Host port, Container port, Protocol | repeater | The Ports step |
| Host path, Container path | repeater | The Volumes step |
| Network mode | select | The Network step |
| Environment variables | repeater | The Environment step |

### Placeholders in environment values

A value on the Environment step may contain four placeholders, and McHarbor replaces them when the
app is installed, in a catalog default and in a value you type alike:

| Placeholder | Becomes |
|---|---|
| `{{serviceName}}` | The stack name the app is deployed as |
| `{{containerName}}` | The same stack name |
| `{{projectName}}` | The same stack name |
| `{{appDataPath}}` | The app's data directory on the host |

So a default such as `{{serviceName}}-redis` reaches the app as `myapp-redis`, which is how an app
with a companion container names its host. The replacement is a single pass: a placeholder inside
the substituted text is not expanded again. Anything else in braces is left as written. Before this
expansion, a value that held one of these placeholders reached the app as literal text. For
catalog authors, the generator (`scripts/apps/generate-entries.mjs`) still refuses a spec whose
environment default contains `{{`, asking for the placeholder to go in the stack's `appEnv`
instead, so shipped entries do not rely on an environment default being expanded.

## Shared list behaviour

Every list screen inherits the same controls: free-text search, a filter mode of
**Contains**, **Exact** or **Regex** (an invalid expression says so rather than silently
matching nothing), saved filters and searches, selection with a bulk bar, and pagination
showing "X to Y of Z" with a rows-per-page control.

## Related

- [Store](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what you can do with these fields
