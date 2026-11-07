# Prerelease channels

A prerelease is a test build that is not final, written with a `-` suffix: `2026.11.03-rc.1`. It sorts below the same version without the suffix and above the previous release. A `+` suffix is build metadata and never makes a version a prerelease. The order is defined once, in `src/contracts/versionorder/cases.yaml`.

Everything here is off on a fresh or upgraded install. A stable release is always offered; a prerelease is offered only through a switch below.

## Switches

| Switch | Where | Default | Key to change it |
|---|---|---|---|
| **Include prereleases** | Settings, Updates, McHarbor | Off | `updates.manage` |
| **Also install prereleases automatically** | Settings, Updates, McHarbor | Off | `updates.manage` |
| **Include agent prereleases** | Settings, Updates, Agents | Off | `updates.manage` |
| **Also update agents to prereleases automatically** | Settings, Updates, Agents | Off | `updates.manage` |
| **Offer plugin prereleases** | Plugin Store, page header | Off | `updates.manage` |
| **Offer prereleases for this plugin** | Plugin details dialog | Inherit | `plugins.manage` |

Reading the settings needs `updates.view`. A control you may not change is shown disabled and names the key it needs.

An automatic option does nothing on its own. A McHarbor prerelease installs automatically only when **Include prereleases**, **Install McHarbor updates automatically** and **Also install prereleases automatically** are all on, and inside the install window. Agents follow the same shape with the agent switches. Plugins have no automatic option. Turning an automatic option on asks for confirmation, and **Install McHarbor updates automatically** and **Update remote agents automatically** keep their own existing gates.

The five switches are stored as `mcharborPrereleases`, `agentPrereleases`, `pluginPrereleases`, `mcharborAutoPrereleases` and `agentAutoPrereleases`. They are read with `GET /api/updates/channels` and saved with `PUT /api/updates/channels`, which takes a partial body: a field you leave out keeps its stored value, so a stale client cannot overwrite a switch it never showed. Unknown fields, and bodies over 4 KiB, are refused with 400. A save that changes nothing writes no audit row; otherwise the `update_channels_change` row lists only the fields that changed, old and new.

## Per-plugin override

The details dialog of a plugin has **Offer prereleases for this plugin**. The override is on, off, or cleared (inherit): cleared follows **Offer plugin prereleases**, and on shows that plugin's prereleases even when the Plugin Store switch is off. It is saved with `PUT /api/appstore/plugins/{slug}/channel` and body `{ "prereleases": true | false | null }`, where `null` clears it. The audit entity is the plugin slug, with the old and new override.

In the plugin catalog, stable releases are listed under `versions[]` and prereleases under a separate `prerelease_versions[]` key that hosts without channels never read. Each row carries its own floor (`mcharbor:`). See [Plugin packaging](../../../how-to/plugin/packaging.md).

## What a prerelease host does

On a host running a prerelease:

- **Include prereleases** off does not move the host. The next stable version newer than the running one is offered when it is published, and no older version can be installed. The page says so under the switch.
- A manual update must name an explicit version. An empty, floating or unparsable tag such as `latest` is refused, because `latest` is the previous stable and would be a downgrade.
- A floating image tag reaches a prerelease by being pinned to that version. A digest-pinned image is refused.

On any host, stable or prerelease, a manual update whose image reference carries a digest (`repo:2026.11.03@sha256:...`) is refused with 400 while **Include prereleases** is off, because Docker pulls the digest and ignores the tag, so the tag does not say what would install. Turn the switch on, or give a plain tag. The Settings button sends no image on a container install and is not affected.
- Plugins follow the host and stay on its prerelease builds. They can return to stable once a newer stable McHarbor is installed.

You cannot go back to an older McHarbor version after installing a prerelease.

## Return to stable

### Plugin

**Return to stable** in the plugin details dialog moves an installed prerelease to the newest stable build and keeps its data. Before **Confirm** is enabled the dialog runs a check, `GET /api/appstore/plugins/{slug}/return/check`, which changes nothing:

| Result | Meaning |
|---|---|
| Ready | Nothing blocks the return |
| Blocked | The prerelease ran a database migration the stable build does not include; the migration is named and nothing was changed |
| Unsigned | No trust key verifies the package, so it was not opened and the migration check cannot run until you agree |
| No stable | No stable version of the plugin is available |

The return itself, `POST /api/appstore/plugins/{slug}/return`, checks again, because the state can change between the check and the confirm. It takes `{ "allow_unsigned": bool }`; `true` is sent only after the operator ticks the explicit consent for an unsigned package. The audit row records the version moved from and to. Both routes need `plugins.manage`.

A prerelease already installed stays on it when the plugin's offer is turned off. It moves to the newest stable only after a release change, or when you return it.

### Agent

An agent on a prerelease is returned from the agents table with **Return to stable**. The candidates are the ready store releases that carry that platform's asset (an image for a container agent), plus the binary bundled with McHarbor when the host itself is stable. Anything else is refused, because McHarbor serves only what it holds.

`POST /api/updates/agents/{envId}/return` needs both `updates.manage` and `environments.manage`: moving an agent already needs the second, and the first alone would widen who can do it. The route authorises the environment named in the path.

`POST /api/updates/agents/{envId}/return` answers 200 with `started: true` once the pin is written: the move itself runs after the answer. A move that fails later does not change the answer; it shows on the agent's row as the reason the return failed, which is read from the agent inventory, `GET /api/updates/agents`. The dialog and `mcharbor agent return` therefore report the return as started, never as finished.

The return also **pins** the agent: no update, automatic or manual, reaches it until the pin is released, and that includes stable security updates. The pin is written before the move. If the move fails, the pin stays, the row shows the prerelease still running with the pin and the reason, and the operator retries by returning again; removing the pin would let a sweep move the agent back. A pinned agent stays listed once under **Exceptions** and cannot be edited there.

To release a pin use **Release pin** in the **Pinned agents** list of the agent update settings card (Settings, Updates, Agents), or `DELETE /api/updates/agents/{envId}/pin`. It needs `updates.manage` and `environments.manage` as well. The audit row records the exclusion removed.

## From the command line

`settings channels show`, `settings channels set`, `plugin channel <slug> <on|off|inherit>`, `plugin return <slug> [--check] [--allow-unsigned]`, `agent return <environment-id>`, `agent pins` and `agent unpin <environment-id>`. `agent pins` lists the agents that are pinned or excluded, with the reason a return failed; it needs `updates.view`. The permission keys are in the [CLI](../../../cli/README.md) page. The returns and the unpin ask for `--confirm <id>`.

## Reports

The platform inventory, agent fleet and environment comparison reports carry a **Runs a prerelease** table. It lists McHarbor, every agent and every plugin.

## When something does not behave

| Symptom | Usual cause | Fix |
|---|---|---|
| A switch is disabled | You lack the key it names | Ask for `updates.manage`, or `plugins.manage` for one plugin's own override |
| No prerelease is offered | The path's switch is off, or nothing newer is published | Turn the switch on; a plugin can override it on its own |
| A prerelease did not install by itself | An automatic option needs the path switch, the automatic switch and the install window | Check all three |
| Return is blocked | The prerelease ran a migration the stable build lacks | Stay on the prerelease until a stable build includes it |
| An agent no longer updates | It is pinned | **Release pin** in the Pinned agents list |
| An agent return answered but the agent is still on the prerelease | The move started and then failed; the pin stays | Read the reason on the agent's row or with `mcharbor agent pins`, then return it again |
| An agent return is refused | One of the two keys is missing, or no ready stable release holds that platform's asset | Hold `updates.manage` and `environments.manage`; download a stable release |

## Related

- [Updates](../README.md)
- [McHarbor](../mcharbor/reference.md) and [Agents](../agents/reference.md) reference
- [Plugin packaging](../../../how-to/plugin/packaging.md)
