# GitOps: Fields

Every column, filter and form field on GitOps: the pipeline itself, the stages it promotes
through, and the nine states a deployment can be in.

## Columns

| Column | What it is |
|---|---|
| Name | Pipeline name |
| Repository | What it watches |
| Stages | How many environments it promotes through |
| Trigger | Auto on push, Manual approval, or PR preview |
| Enabled | Whether it runs |
| Updated | Last change |

## Filters and facets

| Filter | What it narrows |
|---|---|
| Filter by state | On deployments: pending, succeeded, failed, skipped, rolled back |

## Form fields

| Field | Type | What it is |
|---|---|---|
| Pipeline name | text | Your label |
| Source repository | select | A repository from the Git screen |
| Trigger | select | Auto on push, Manual approval, PR preview |
| Stage name, Order | text / number | Per stage |
| Target environment | select | Where the stage deploys |
| Watch branch | text | The branch this stage follows |
| Compose path | text | Path to the Compose file inside the repository |
| Deploy mode | select | Auto, Manual or PR preview |
| Requires approval | toggle | Whether a promotion into this stage waits |
| Webhook URL, Webhook secret | text / secret | For PR previews; configure these on your forge |

Deployment statuses are **pending**, **succeeded**, **failed**, **skipped**,
**rolled back**, **approved**, **rejected**, **active** and **demolished**. The last two
belong to preview environments.

## Shared list behaviour

Every list screen inherits the same controls: free-text search, a filter mode of
**Contains**, **Exact** or **Regex** (an invalid expression says so rather than silently
matching nothing), saved filters and searches, selection with a bulk bar, and pagination
showing "X to Y of Z" with a rows-per-page control.

## Related

- [GitOps](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what you can do with these fields
