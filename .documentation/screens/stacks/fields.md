# Stacks: Fields

Every column and form field on Stacks: what the list reports about a stack's services and
images, the Compose file itself, and the webhook fields, including the six events one can
fire on.

## Columns

| Column | What it is |
|---|---|
| Name | Stack name |
| Status | Aggregate state across its services |
| Services | Running of total |
| Images | Images the stack uses |
| Description | Yours, on a managed stack |
| Created | When it was first seen or deployed |

## Filters and facets

| Filter | What it narrows |
|---|---|
| Search | Stack name |
| View | Table or card |

## Form fields

| Field | Type | What it is |
|---|---|---|
| Stack name | text | Used as the Compose project name |
| Description | text | Optional, managed stacks only |
| Compose file | code | The `docker-compose.yml` itself, validated on deploy |
| Webhook URL | text | Where the POST goes |
| Webhook secret | secret | Optional HMAC secret |
| Webhook events | multi-select | Up, Down, Stop, Restart, Deploy, Update |

## Shared list behaviour

Every list screen inherits the same controls: free-text search, a filter mode of
**Contains**, **Exact** or **Regex** (an invalid expression says so rather than silently
matching nothing), saved filters and searches, selection with a bulk bar, and pagination
showing "X to Y of Z" with a rows-per-page control.

## Related

- [Stacks](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what you can do with these fields
