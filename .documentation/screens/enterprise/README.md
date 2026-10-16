# Enterprise

`/enterprise`, `/enterprise/ha`, `/enterprise/compliance`, `/enterprise/environment-variables`

The **Enterprise** sidebar section gathers the pages an administrator or an auditor uses to
judge an install as a whole, rather than to operate one resource.

## Permissions

| Page | Route | Sidebar entry needs |
|---|---|---|
| Overview | `/enterprise` | `settings.view` |
| High availability | `/enterprise/ha` | `cluster.view` |
| Compliance | `/enterprise/compliance` | `compliance.view` |
| Environment variables | `/enterprise/environment-variables` | `settings.view` |

Navigation hides an entry you lack the key for; the section disappears entirely when every
entry in it is hidden. Opening a route directly without the key shows a notice naming the
permission and sends no request.

The section and the command-palette entry are governed by the release gate
`ENTERPRISE_NAV_ENABLED` in `src/frontend/resources/layout/sidebar-data.ts`. It is `true`;
setting it to `false` withdraws both without removing the routes.

## Pages

| Page | What it covers |
|---|---|
| Overview | Cards linking to the existing enterprise surfaces: security, identity, SCIM, audit, backups, plugins, HA |
| High availability | See [High availability](../../settings/system/high-availability/README.md) |
| [Compliance](./compliance.md) | The measured posture, section by section, and the downloadable report |
| [Environment variables](./environment-variables.md) | Every variable the server and the agent read, with search and flags |
