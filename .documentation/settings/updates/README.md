# Updates

Four separate audiences, deliberately split into four sections: container images, remote agents, McHarbor itself, and the operating system of the machine McHarbor is installed on. They used to be one section stacking all three, which made it easy to change the wrong one.

| Section | Covers | Gated by |
|---|---|---|
| [Containers](./containers/README.md) | The container images you run | `updates.view` / `updates.manage` |
| [Agents](./agents/README.md) | Remote agents, against the version this McHarbor expects | `updates.view` / `updates.manage` |
| [McHarbor](./mcharbor/README.md) | McHarbor itself | `updates.view` / `updates.manage` |
| [Host OS](./host-os/README.md) | The operating system of the machine McHarbor is installed on | `host_self.view` / `host_self.manage` / `host_self.reboot` |

## The order they belong in

1. **McHarbor** updates first, since it defines the agent protocol version.
2. **Agents** follow, so nothing is left behind a version floor.
3. **Containers** are independent of both, on whatever schedule suits the workload.

## What each one costs you

| Section | Disruption |
|---|---|
| Containers | A restart per container, when auto-restart is on |
| Agents | A short agent restart; the offline buffer covers the gap |
| McHarbor | The whole interface, for the length of a container recreate |

An old `?tab=updates` URL still resolves; it aliases to the McHarbor section.

## Related

- [Settings](../README.md): the other five groups
