# Screens

One folder per screen, each with four pages: what it is, what you can do on it, every
field it exposes, and what goes wrong. A screen that is really a *section* of another one
is a page inside that screen’s folder instead. Secrets lives in
[Security](./security/secrets.md), because that is where the interface puts it.

Read **The application frame** below once: the sidebar, the environment switcher, the
search and the list conventions are shared by nearly every screen, and the individual
folders assume you have.

## The route map

| Route | Screen |
|---|---|
| `/dashboard` | [Dashboard](./dashboard/README.md) |
| `/containers`, `/containers/:id`, `/containers/:id/edit` | [Containers](./containers/README.md) |
| `/stacks`, `/stacks/:name` | [Stacks](./stacks/README.md) |
| `/images`, `/images/:id` | [Images](./images/README.md) |
| `/volumes`, `/volumes/:name` | [Volumes](./volumes/README.md) |
| `/networks`, `/networks/:id` | [Networks](./networks/README.md) |
| `/logs` | [Logs](./logs/README.md) |
| `/terminal` | [Terminal](./terminal/README.md) |
| `/activity` | [Activity](./activity/README.md) |
| `/lifecycle` | [Lifecycle log](./lifecycle/README.md) |
| `/backups` | [Backups](../backup/screen/README.md) is the screen; the rest of the story is the [Backup](../backup/README.md) section |
| `/reconciler` | [Reconciler](./reconciler/README.md) |
| `/workflows`, `/workflows/:id`, `/workflows/:id/runs` | [Workflows](./workflows/README.md) |
| `/environments`, `/environments/:id` | [Environments](./environments/README.md) |
| `/git` | [Git](./git/README.md) |
| `/gitops` | [GitOps](./gitops/README.md) |
| `/store` | [Store](./store/README.md) |
| `/audit` | [Audit](./audit/README.md) |
| `/enterprise`, `/enterprise/ha`, `/enterprise/compliance`, `/enterprise/environment-variables` | [Enterprise](./enterprise/README.md) |
| `/security` and its detail routes | [Security](./security/README.md) |
| `/secrets` | Redirects to `/security?tab=secrets`. See [Secrets](./security/secrets.md) |
| `/notifications` | [Notifications](./notifications/README.md) |
| `/profile` | [Profile](./profile/README.md) |
| `/settings` | [Settings](../settings/README.md) |
| `/plugins/:slug/*` | Pages owned by an installed plugin. See [Plugins](../plugins/README.md) |
| `/docker` | [Docker](./docker/README.md): routed, but **not in the sidebar**; reached from the environment or by typing the path |

Unauthenticated routes (`/login`, `/setup`, `/setup/database`, `/force-password-change`,
`/reset-password`, `/reset-password/:token`) are covered in
[First run](../getting-started/first-run/README.md).

The Enterprise pages have a sidebar section of their own. See [Enterprise](./enterprise/README.md).

## The application frame

### Sidebar

Five host sections, plus whatever installed plugins contribute:

| Section | Contains |
|---|---|
| _(no header)_ | Dashboard |
| **Docker** | Containers, Stacks, Images, Volumes, Networks |
| **Operate** | Logs, Terminal, Activity, Lifecycle Log, Backups, Reconciler, Workflows |
| **Deliver** | Environments, Git, GitOps, Store |
| **Govern** | Audit, Security scans |
| **Enterprise** | Overview, High availability, Compliance, Environment variables |

Plugin sections render after these; a plugin that declares a `parent` gets a section of its
own, and plugins without one collect under **Plugins**, which always sorts last.

Some entries carry a badge: running and stopped container counts, image, volume and network
counts, critical-or-high vulnerabilities, and unread notifications.

### The page head

Under the 56px top strip, every screen renders a full-width **page-head band**: the page's
title block or identity chips on the left, its actions and pinned tabs on the right, and a
hairline under it. It is the page's own header, and it is where a screen puts what used to be
compressed into the strip at 11px.

The band supplies the **top** spacing for the content below it. The page area adds the
horizontal and bottom gutter and nothing on top, so the two do not stack. A screen that
appeared to have an extra band of empty space under its title was this doubling, and it is
the shell's job rather than each page's to get it right.

### Denied controls stay visible

Everywhere except navigation, a control you lack permission for renders **disabled with a
tooltip naming the missing key** ("Requires the `containers.manage` permission") rather
than disappearing. You can see that the capability exists and learn what to ask for,
instead of discovering it from a 403.

A whole section you cannot read shows "Permission required — you do not have permission to
view this section."

Navigation is the one deliberate exception: a sidebar entry you cannot use is hidden,
because a dead-end page is not informative.

### The environment switcher

Most screens are scoped to one environment, chosen in the sidebar. **All Environments** is
available where an aggregate makes sense. The choice persists, so a screen opened later is
still pointed where you left it.

### Search and the command palette

Two overlays:

- **Command palette**: commands and page links, including the tab list of whatever screen
  you are on.
- **Global search**: the same, plus resources across every environment: containers,
  images, volumes, networks and stacks, each group counted in its own heading. It also
  offers "search in this page" to hand the query to the current screen's own filter.
  Installed plugins add their own groups and actions: virtual machines, system containers,
  Swarm services, Kubernetes pods and deployments, and compute hosts, pools and images, plus
  entries such as creating a machine or adding a cluster. A group or action appears only when
  you hold the permission its plugin names, and a plugin that is not installed adds nothing.
  How a plugin declares them: [Search palette contract](../development/contracts/search/README.md).

Both are keyboard-first; `?` opens the shortcut cheat sheet.

### Tabs

Screens with many tabs (containers with 21, settings with 20, security with 10) use a
two-level bar: a group row, then the sections in that group. The **tab id stays the unit of
navigation**, so `?tab=<id>` in a URL keeps resolving no matter how groups are rearranged.
Right-clicking a tab offers switch, reload, copy tab link, and open in a new browser tab.

### Lists

Nearly every list screen shares the same behaviour:

| Feature | What it does |
|---|---|
| Search | Free text over the visible rows |
| Filter modes | Contains, Exact, or **Regex**; an invalid expression says so rather than silently matching nothing |
| Saved filters and searches | Name a filter and re-apply it later |
| Facets | Filter by status, environment, and the fields that matter for that resource |
| Date range | Presets from "Last 15 minutes" to "Last year", plus a custom range, on time-series screens |
| Selection and bulk actions | A bar shows "N of M selected", with **Select all N matching** to extend a page's selection to every matching row. Bulk runs report "done / total (succeeded, failed, running)", so partial failure is visible, not swallowed |
| Pagination | "Showing X to Y of Z", rows per page, go-to-page |
| Views | Table or timeline, where a timeline makes sense |

Destructive actions (stop, remove, kill, prune) always confirm first, and the
confirmation says how many items it will affect.

Columns and saved filters are shared infrastructure across nearly every list screen. See
[Table views](./table-views/README.md) for the Edit table dialog, column resizing, and the
saved-filters menu.

### Live data

Container logs, the terminal and the events stream are WebSocket-backed. Metrics, lists and
status badges are polled on the interval set in
[Settings → General](../settings/general/general/README.md). A proxy in front of McHarbor
has to allow WebSocket upgrades, or the first three stop working while everything else
looks fine.

### Notifications and toasts

Toasts report the outcome of what you just did. Where they appear, how long they stay, and
whether users may override the default is
[Settings → General → Notifications](../settings/general/notifications/README.md). The bell
in the top bar opens [Notifications](./notifications/README.md), which is the durable list;
toasts are transient, that page is not.

## Related

- [Getting started](../getting-started/README.md): installing and first run
- [Settings](../settings/README.md): every setting, section by section
- [Plugins](../plugins/README.md): the screens plugins add
