# Reconciler

`/reconciler`

Declare what should be running (a container, a stack, or everything on an environment)
and McHarbor notices when reality departs from it. What it does next is your choice per
watch: tell you, write a plan for you to approve, or fix it within strict limits.

## Permissions

| Key | What it unlocks |
|---|---|
| `reconciler.view` | See watches, drift, the drift report, plans, Git sources and the settings |
| `reconciler.manage` | Create, edit and delete watches; check drift; reconcile; propose, approve and reject plans; set **Observe** or **Propose a fix**; the debounce setting; Git sources and pull requests |
| `reconciler.enforce` | Set a watch to **Enforce**, change the enforcement limits, and reset enforcement after it stopped |

`reconciler.enforce` is deliberately separate: someone who may declare intent is not
automatically someone who may let McHarbor change containers without asking.

Everywhere except navigation, a control you lack the key for renders **disabled with a
tooltip naming it**, rather than disappearing. Navigation is the exception: a sidebar
entry you cannot use is hidden, because a dead-end page is not informative.

## What it shows

- One row per watch: name (with a **Git** badge when a Git source owns it), its
  **Response**, what it watches, image, status, last check, and the classes of any drift.
- Drift by class: **Configuration**, **State**, **Existence**, **Image**, with *Show what
  drifted* for field, expected, actual and first seen.
- From the header: **Drift report**, **Plans**, **Git sources**, and the settings.

## What to watch

| Kind | Drift when |
|---|---|
| One container | missing or not in its desired status; another image; restart policy, env, labels, ports, mounts, limits, networks, capabilities or healthcheck differ from its config |
| A stack | a service its compose file starts is missing or not running, or it runs a container its file does not declare |
| Everything in this environment | a container runs that no container or stack watch here accounts for |
| Virtual machine, system container, Swarm service or node, Kubernetes Deployment or node | a pinned property departs: power, size, replicas, image, availability; offered only while its plugin is installed, see [Desired state for plugins](../../plugins/desired-state/README.md) |

Everything in the form is picked, not typed: the container and its image come from the
chosen environment, and picking an existing container fills in the image and status it runs
now. **Enter manually…** in each list declares something that does not exist yet. For a
plugin kind, pick the environment (or the Kubernetes cluster), then the subject, then pin only
the properties to hold; a property on **Not managed** is never checked. **Use current
values** fills them from what is running.

## How drift is answered

| Response | What happens |
|---|---|
| **Observe** | Recorded, audited, alerted on. Nothing changes |
| **Propose a fix** | Also writes a plan to restore the desired state, waiting under **Plans** |
| **Enforce** | McHarbor applies the plan itself, within the enforcement limits, and writes a plan instead whenever it cannot or may not act |

See [Plans and enforcement](./plans-and-enforcement.md).

## Reconciler, GitOps and auto-update

Three things that keep something current, answering different questions:

| Feature | Question |
|---|---|
| **Reconciler** | Does this still match what I declared? |
| [GitOps](../gitops/README.md) | Does this environment match the repository? |
| [Container updates](../../settings/updates/containers/README.md) | Is there a newer image for this container? |

They compose badly if pointed at the same container: a watch pinning an image and an
auto-update policy moving it will undo each other, and with **Enforce**, fight. Pick one
owner per container.

## In this folder

| Page | What it covers |
|---|---|
| [Actions](./actions.md) | Everything you can do here, what gates it, and what it really does |
| [Fields](./fields.md) | Every column, filter and form field the screen exposes |
| [Plans and enforcement](./plans-and-enforcement.md) | Plans, approving, enforcement limits and the breaker |
| [Git sources](./git-sources.md) | Desired state from a repository, and drift as a pull request |
| [Troubleshooting](./troubleshooting.md) | Symptom, cause and fix |

## Related

- [Containers](../containers/README.md): what a watch governs
- [GitOps](../gitops/README.md): the repository-driven deploy
- [Git, GitOps and the reconciler API](../../api/endpoints/gitops.md): routes and exact behaviour
