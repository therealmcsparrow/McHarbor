# Profile: Fields

Every field on your own account: the identity other users see, the password rules, the
appearance and language preferences, and the session list.

## Columns

| Column | What it is |
|---|---|
| Sessions | Where and when you signed in, and which session is current |

## Form fields

| Field | Type | What it is |
|---|---|---|
| Display name | text | How other users see you |
| Email | text | Used for password reset |
| Current / New / Confirm password | secret | Minimum eight characters |
| Theme | select | Light, Dark or System |
| Language | select | Six locales; applies immediately |
| Toast preferences | mixed | Where the app-wide policy allows overrides |

## Shared list behaviour

Every list screen inherits the same controls: free-text search, a filter mode of
**Contains**, **Exact** or **Regex** (an invalid expression says so rather than silently
matching nothing), saved filters and searches, selection with a bulk bar, and pagination
showing "X to Y of Z" with a rows-per-page control.

## Related

- [Profile](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what you can do with these fields
