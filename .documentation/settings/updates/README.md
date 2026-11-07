# Updates

Separate audiences, deliberately split into sections: container images, remote agents, McHarbor itself, the operating system of the machine McHarbor is installed on, and the prerelease switches. They used to be one section stacking all three, which made it easy to change the wrong one.

| Section | Covers | Gated by |
|---|---|---|
| [Containers](./containers/README.md) | The container images you run | `updates.view` / `updates.manage` |
| [Agents](./agents/README.md) | Remote agents, against the version this McHarbor expects | `updates.view` / `updates.manage` |
| [McHarbor](./mcharbor/README.md) | McHarbor itself | `updates.view` / `updates.manage` |
| [Host OS](./host-os/README.md) | The operating system of the machine McHarbor is installed on | `host_self.view` / `host_self.manage` / `host_self.reboot` |
| [Prerelease channels](./prerelease-channels/README.md) | Test builds of McHarbor, agents and plugins, and returning to stable | `updates.view` / `updates.manage`, `plugins.manage` for plugins |

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

Test builds that are not final are off everywhere until you turn on a switch, see [Prerelease channels](./prerelease-channels/README.md).

An old `?tab=updates` URL still resolves; it aliases to the McHarbor section.

## Browser requests behind a proxy

A change made from the browser (a save, a return, an update) is refused when it comes from another site. McHarbor judges that by the `Sec-Fetch-Site` and `Origin` headers. A reverse proxy that passes both through needs nothing. A plain-http proxy that rewrites `Host`, or a UI served from a sibling host, makes the browser's origin differ from the one McHarbor sees, so that origin has to be listed in the allowed origins setting (`ALLOWED_ORIGINS`). Requests that carry a bearer token, such as the CLI, are not affected. See [Reverse proxy](../../getting-started/deployment/reverse-proxy.md).

## Related

- [Settings](../README.md): the other five groups
