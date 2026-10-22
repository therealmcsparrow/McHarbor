# Dashboard

`/dashboard`

The landing screen: a grid of widgets you arrange yourself.

## Permissions

| Key | What it unlocks |
|---|---|
| `dashboard.view` | Open the dashboard |
| `widgets.manage` | Add and remove widgets |

Everywhere except navigation, a control you lack the key for renders **disabled with a
tooltip naming it**, rather than disappearing. Navigation is the exception: a sidebar
entry you cannot use is hidden, because a dead-end page is not informative.

## What it shows

- A live line across the page head: how many environments, how many containers and across how
  many stacks, how many virtual machines and across how many environments, how many agents are
  a version behind, and how many scans need review. See **The page head** below.
- A grid of widgets, arranged per user; two people looking at the same McHarbor see their own layout.
- A layout picker with five starting points.
- Widgets that follow the environment switcher, aggregating across environments where they support it.

## The page head

The band under the title is an estate summary, not a static sentence:

> 4 environments · 61 containers across 7 stacks · 7 virtual machines across 2 environments ·
> 1 agent a version behind · 2 scans need review

Every number is live, and it is **fleet-wide** rather than scoped to the environment switcher.
The widgets below it follow the switcher; this line deliberately does not.

Four rules govern what appears:

- **A count you may not read is absent, not zero.** `GET /api/dashboard/summary` computes each
  figure only over the environments you hold `environments.view` on, and omits containers or
  stacks entirely without `containers.view` / `stacks.view`. The line simply has one segment
  fewer. No new permission key was added for it; every figure keeps the gate it already had.
- **Exception counts appear only when they are not zero.** Agents behind, scans needing review
  and unreachable hosts are absent on a healthy estate, and render in the warning colour when
  present. Environments and containers are always shown. Host disks that are critical, above their warning threshold, or without a reading follow the same rule, and each appears as its own segment only when its count is not zero.
- **An unreachable host is said so.** Its containers are not counted, and the line reports how
  many hosts did not answer rather than quietly showing a smaller number.
- **Virtual machines need the plugin.** The segment comes from `GET /api/vms` and appears only
  when the [Virtual Machines](../../virtual-machines/README.md) plugin is
  installed and you hold `vm.view`. Without it there is no segment, not a zero.

"Needs review" has a definition, because a scan carries no review state: an image whose
**latest completed scan** found something at **critical or high**.

The line refreshes every 30 seconds.

## Layouts

| Layout | What it puts in front of you |
|---|---|
| **Starter** | The verdict and your containers; everything else you add as you need it |
| **Homelab** | What is running, on what, and the quick actions you reach for most |
| **DevOps** | Workflow runs, alerts and vulnerabilities front and centre |
| **Production** | Capacity, alerts, vulnerabilities and storage at a glance |
| **Monitoring** | Time-series charts and top consumers, for watching trends |

## The widget catalog

22 shipped widgets, grouped as Resources, Host, Metrics, Monitoring, Operations, and
Plugins for anything an installed plugin contributes:

`fleet-health`, `needs-attention`, `container-list`, `container-states`,
`workload-heatmap`, `container-age`, `restart-tracker`, `stack-status`, `port-map`, `resource-summary`,
`resource-donut`, `top-consumers`, `metric-chart`, `storage-breakdown`, `host-info`,
`activity-feed`, `alert-summary`, `vulnerability-summary`, `update-queue`,
`snapshot-status`, `workflow-runs`, `quick-actions`.

**Workload heatmap** draws one small square per container, coloured by state and grouped
so that anything unhealthy collects at the end of the grid. Click a square for a card
with that container's name, state, status line, stack, published ports, IP address and
short ID, plus a link straight to its detail page. It is the tile to reach for when the
question is *which* container is red, rather than *how many* are.

A widget that cannot load says so in place ("*Resource summary* could not be shown")
with **Try again**, rather than taking the grid down with it.

Individual widgets are limited by what their data needs: one reading alerts shows nothing
to a user without `alerts.view`.

## In this folder

| Page | What it covers |
|---|---|
| [Actions](./actions.md) | Everything you can do here, what gates it, and what it really does |
| [Fields](./fields.md) | Every column, filter and form field the screen exposes |
| [Troubleshooting](./troubleshooting.md) | Symptom, cause and fix |

## Related

- [Containers](../containers/README.md): what most widgets summarise
- [Settings → General → General](../../settings/general/general/README.md): how often data refreshes
- [Store](../store/README.md): the widget catalog, including plugin-contributed widgets
