# Agents: Reference

| Setting | Default | What it does |
|---|---|---|
| **Update remote agents automatically** | Off | "After McHarbor updates, push the matching agent binary to every connected agent that is not excluded" |
| **Target agent version** | derived | The version this McHarbor expects, or a statement that it is not known yet |
| **Exceptions** | none | "Agents selected here are skipped by automatic **and** manual agent update sweeps" |
| **Check for new agent releases** | On | Runs the release check on **Check schedule** |
| **Check schedule** | `0 5 * * *` | When McHarbor asks its sources which agent version and build is newest; **Next check** is shown under it. A failed check runs again after 15 minutes, and at start-up when the last one failed |
| **When a new release is found** | Notify only | Notify only · Download it · Download it and make it the target |
| **Releases to keep** | 3 (1 to 20) | Older stored builds are removed after each download, never the target or a build a connected agent runs. Each build of a version counts as one |
| **Sources, tried in order** | Mirror `https://www.mcharbor.eu/catalog/agents/index.json` | Up to eight `https://` sources: GitHub releases or a [mirror](../../../agents/releases/mirror-contract.md) |
| **Container agent images to store** | `linux/amd64` | Platforms whose agent image McHarbor keeps as a tarball for container agents |
| **Image repository** | `ghcr.io/therealmcsparrow/mcharbor-agent` | Where McHarbor pulls the image when a release signs no digest |

The **target** is set per build in the **Agent releases** table, and a download you start sets
it too, keeping the builds stored before it. With no target, installs and updates are served the
locally built agent, else the one bundled with McHarbor.

An agent update pushes the binary and the agent restarts into it. Its data directory carries
the instance id and the offline buffer across the restart, which is why that volume
matters, and why an agent without one loses its buffered events.

## When something does not behave

| Symptom | Usual cause | Fix |
|---|---|---|
| The terminal will not open on one host | That agent is older than v1.1.0 | Update it, or exclude it knowingly and accept the limit |
| An agent reads **Update by hand** | The agent is too old for a pushed update | Update that host by hand |
| An agent on the target version still reads **Outdated** | It runs an older build of that version | **Update agents now**; the sweep compares the build too |
| A newer build is listed but the release does not move to it | The stored build is **Pinned**: an older build was pulled on purpose | Pull the newest build (the **Download** button or the list) to unpin it; see [Pinned builds](../../../agents/releases/README.md#pinned-builds) |
| An agent is never updated | It is in the exceptions list | Exceptions skip automatic and manual sweeps alike |
| An agent lost its buffered events after an update | No persistent data directory | Mount `MCHARBOR_DATA_DIR` as a volume |
| A download moved the target and you want the previous build back | A download you start becomes the target | **Make … the target** on the previous build's row; it is still stored |
| A sweep fails with "McHarbor holds no agent for …" | Nothing McHarbor serves covers that platform | Download a release that does and make it the target, or build one locally |
| A release reads **Refused** | Its signature or a checksum did not verify | The reason is behind the error icon; see [Releases](../../../agents/releases/README.md#when-something-does-not-behave) |

## Related

- [Agents](./README.md): what the section is for
- [Settings](../../README.md): the other groups
