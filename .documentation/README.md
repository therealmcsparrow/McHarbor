# McHarbor Documentation

McHarbor is a self-hosted container management platform. It runs as a Docker container,
manages Docker across local, remote and NAT-ed hosts, and ships with stacks, backups,
workflows, RBAC, auditing and a plugin system.

This is the technical documentation, for people who **operate** an install and people who
**change the code**. Every page states the permission keys involved, because both audiences
end up needing them.

## Sections

| Section | Covers |
|---|---|
| [Getting started](./getting-started/README.md) | Prerequisites, installation, first run, every environment variable, upgrading, production deployment, verifying releases |
| [Screens](./screens/README.md) | The application frame, and one folder per screen: what it shows, what you can do, every field, what goes wrong |
| [Settings](./settings/README.md) | All 20 settings sections in their six groups, each with a reference table of every setting |
| [Backup](./backup/README.md) | Backup end to end: plans, runs, consistency, encryption, retention, restore, and where archives go |
| [Workflows](./workflows/README.md) | The execution model, the message contract, triggers, all 86 nodes, and building your own |
| [GitOps](./gitops/README.md) | Promotion pipelines and ephemeral pull-request preview environments |
| [Agents](./agents/README.md) | The remote agent: prerequisites, install, every environment variable, use cases, the protocol, operations, container stats |
| [Plugins](./plugins/README.md) | What plugins are, the catalog, and one folder for every plugin that ships with McHarbor. The six add-ons are listed below with their own sections; the runtime and each plugin's developer notes are under [Development](./development/plugins/README.md) |
| [API](./api/README.md) | Every HTTP endpoint, the conventions above them, and every setting the API exposes |
| [On-call](./oncall/README.md) | The installable phone view (alerts, fleet, container restart, logs, approvals, runbooks), push notifications, and what it shows offline |
| [Incidents](./incidents/README.md) | Failing containers with their correlated evidence, opt-in AI narration, post-incident timelines with the runbook that was followed, and behaviour findings |
| [Observability](./observability/README.md) | McHarbor's own Prometheus metrics and OpenTelemetry traces, the scrape key, the shipped Grafana dashboard |
| [How-to](./how-to/README.md) | Task guides: build a plugin, contract permissions, add a widget, a node, an AI provider, a report, or script the API |
| [Command-line client](./cli/README.md) | `mcharbor`: install, contexts and API keys, every command and the permission it needs, frozen exit codes, output formats, confirmation rules, the generated command reference |
| [Terraform](./terraform/README.md) | The Terraform provider: environments, stacks, roles, role assignments and API keys as code, what lands in state, and the key each operation needs |
| [Compliance](./compliance/README.md) | Compliance packs: evidence for the CIS Docker Benchmark, SOC 2 and ISO/IEC 27001, the checks McHarbor can run mechanically, and the controls it cannot see |
| [Development](./development/README.md) | For people who change the code in `src/`: running and building it with `scripts/development/`, every extension hook point, the plugin contract and the contract tests, and the developer pages for plugins, add-ons and workflow nodes |

## Add-ons

Six add-ons ship with McHarbor. They are installed as plugins, but each one owns a product
area rather than filling an integration slot, so each has a section of its own.

| Section | Covers |
|---|---|
| [Virtual Machines](./virtual-machines/README.md) | KVM virtual machines on your own hosts: create from a cloud image or an ISO, console, snapshots, import |
| [System Containers](./system-containers/README.md) | Full-system LXC containers: a whole distribution with its own init, in a fraction of a VM's memory |
| [Kubernetes](./kubernetes/README.md) | Pods, deployments, services, storage, config, namespaces, nodes and events |
| [Docker Swarm](./docker-swarm/README.md) | Services, tasks, nodes, secrets, configs, swarm stacks and overlay networks, and the cluster lifecycle |
| [Reporting](./reporting/README.md) | Board-presentable reports on environments, workloads, incidents, security, capacity, delivery and recovery |
| [Compute Foundation](./compute-foundation/README.md) | The shared foundation under the two guest add-ons: host requirements, storage pools, guest networks, the image catalog |

## Start here

| If you want to | Read |
|---|---|
| Get it running | [Getting started](./getting-started/README.md) |
| Understand a screen you are looking at | [Screens](./screens/README.md) |
| Change how it behaves | [Settings](./settings/README.md) |
| Protect container data, and get it back | [Backup](./backup/README.md) |
| Run a virtual machine or a system container | [Virtual Machines](./virtual-machines/README.md), [System Containers](./system-containers/README.md) |
| Manage a Kubernetes cluster or a Swarm | [Kubernetes](./kubernetes/README.md), [Docker Swarm](./docker-swarm/README.md) |
| Report on what you run | [Reporting](./reporting/README.md) |
| Automate something with more than one step | [Workflows](./workflows/README.md) |
| Manage a host McHarbor does not own | [Agents](./agents/README.md) |
| Add an integration | [Plugins](./plugins/README.md) |
| Drive McHarbor from a script | [API](./api/README.md) |
| Produce evidence for an audit | [Compliance](./compliance/README.md) |
| Get woken by an alert and act on it from a phone | [On-call](./oncall/README.md) |
| Find out why a container keeps failing | [Incidents](./incidents/README.md) |
| Monitor McHarbor itself from Prometheus and Grafana | [Observability](./observability/README.md) |
| Extend McHarbor yourself | [How-to](./how-to/README.md) |
| Change McHarbor's own code | [Development](./development/README.md) |
| Manage McHarbor from Terraform | [Terraform](./terraform/README.md) |
| Operate McHarbor from a terminal or a CI job | [Command-line client](./cli/README.md) |

## How this set is organised

Each documented thing gets a **folder**, not a page:

```
screens/containers/{README,actions,fields,troubleshooting}.md
settings/integrations/storage/{README,reference}.md
backup/screen/{README,actions,fields,troubleshooting}.md
virtual-machines/{README,configuration,troubleshooting}.md
plugins/communication/slack/{README,configuration,troubleshooting}.md
development/plugins/communication/slack.md
workflows/nodes/logic/README.md
agents/configuration/README.md
api/{README,conventions,endpoints,settings}/…
how-to/plugin/{README,manifest,backend,frontend,packaging,abi-and-libc}.md
getting-started/installation/{README,docker-engine,native-linux,troubleshooting}.md
```

Nothing goes deeper than three folders. A section's `README.md` is its index.

## Conventions

- **Pages carry no date stamps.** What changed, and when it was last verified against the
  code, is in [CHANGELOG.md](./CHANGELOG.md).
- **Permissions are named with their exact keys**: `containers.manage`, not "container
  management rights".
- **Routes are written as they are mounted**, with the permission that gates them.
- **No screenshots.** Labels and strings are quoted from the interface instead, so the pages
  stay true through a restyle.
- **Where a repository guide and the code disagree, the code wins** and the page says so.
