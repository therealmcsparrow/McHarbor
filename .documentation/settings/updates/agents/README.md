# Agents

**Settings → Updates → Agents** · `?tab=updates-agents`

Keep remote agents on the version this McHarbor expects.

## Permissions

| Key | What it unlocks |
|---|---|
| `updates.view` | See the section |
| `updates.manage` | Change the setting, and pull, delete or choose agent releases |
| `environments.manage` | Run a sweep (**Update agents now**) and edit the exceptions |

A section you lack the key for renders **disabled with a tooltip naming it**, rather than
opening onto an empty panel.

## What it governs

- Whether agents are updated automatically after McHarbor updates.
- A manual sweep, and per-agent exceptions.
- **Agent releases**: the table of releases McHarbor has downloaded and verified, one row per
  build, which one is the target every install and update is served, and **Check now** /
  **Download &lt;version&gt;**. A download becomes the target once it verifies and keeps every
  build stored before it, so you can make the previous build the target again. When the target
  changes, the agent table shows straight away which agents are now outdated.
- **Agent release downloads**: where new releases come from, when McHarbor checks, and
  whether it only notifies, downloads, or downloads and makes the new release the target.

Pulling, deleting and choosing the target need `updates.manage`; rolling the target out to
agents still needs `environments.manage`. The whole mechanism is described under
[Agents → Releases](../../../agents/releases/README.md).

## Why this exists

An agent speaks a protocol version. Upgrading McHarbor without upgrading its agents leaves
them behind, and some features have hard floors: an interactive terminal through an agent
needs **v1.1.0 or newer**, because the Docker SDK’s non-detached exec bypasses the agent
transport and dedicated WebSocket messages carry it instead.

A stale agent does not fail loudly. It works for almost everything and quietly cannot do one
thing.

## What a sweep reports

**Update agents now** starts the sweep and answers at once, without waiting for any agent:

> 2 agents queued for 2026.10.01; the table shows their progress.

The count is every connected agent not on the exceptions list. Each one then moves through the
agent table on its own: **Queued**, **Updating to &lt;version&gt;…**, **Replacing itself**, then
**Up to date**, or **Failed** with the reason behind the error icon. An agent already on the
target version **and build** is left alone. **Update by hand** marks an agent too old for a
pushed update, and it is the one that needs a person.

## In this folder

| Page | What it covers |
|---|---|
| [Reference](./reference.md) | Every setting, its default, and what changing it costs |

## Related

- [Environments](../../../screens/environments/README.md): agent environments, tokens and versions
- [System → Agent](../../system/agent/README.md): transport settings and the agent-to-agent test
