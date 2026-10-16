# System Containers troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| Container starts, services inside do not | The image expects a privileged container or extra capabilities | Check what the distribution's init needs before granting `ct.privileged`. |
| Limit changes appear not to apply | Looking at the wrong cgroup version on the host | Limits are live; confirm with the guest's own `systemd-cgtop`. |
| "This container's host runs an agent too old to build or remove a container's rootfs" | The host's agent predates rootfs unpacking and guarded removal | Update the agent on that host to `2026.09.01` build `202609271429` or newer. Nothing was changed on the host |
| "Image … is not in this host's image cache" | The image was never downloaded to this host | Download it to the host from the image library, then create again |
| "Refusing an unsafe rootfs archive" | The image or template holds an absolute or `..` path, a symlink escaping the rootfs, or a file owned outside the container's id range | Use a different image. Nothing was left on disk |
| "Lxc: rootfs for … is missing" on create | The container was created by addon-lxc `2026.09.06` or older, which never unpacked its image | Delete the crashed container and create it again |
| "Refusing to remove this rootfs" on delete | The container's recorded rootfs is not strictly inside its pool (or the pool has no directory path, such as LVM or ZFS) | Delete with **Keep the filesystem**, then remove the tree by hand |
| "ct_delete asked to remove the rootfs but carried no container spec" | An addon-lxc older than `2026.09.07` against a current agent | Update the plugin from the Plugin Store |
| The console repeats "can't open /dev/tty1" … "tty6" and never shows a shell | A busybox-init container (the alpine minirootfs) created by an agent at build `202609271856` or older: its inittab runs getty on virtual terminals a container does not have | In an **exec** session, comment out the `tty1` to `tty6` lines in `/etc/inittab`, add `console::respawn:-/bin/sh`, then restart the container. New containers get this at create; see [The console and the init system](./README.md#the-console-and-the-init-system) |
| The console shell says "can't access tty; job control turned off" | PID 1 owns `/dev/console`, so the shell cannot make it its controlling terminal | Expected. Use **exec** for a full terminal |
| Stats show swap equal to memory, memory high 0, or rootfs used 0 | An agent at build `202609271856` or older: it reported memory+swap as swap, never read `memory.high`, and did not measure the rootfs | Update the agent. Rootfs used is measured in the background and refreshed every five minutes, so the first sample of a large rootfs can still read 0 |
| The capability check's **subuid** item said "room for 1 containers" | An agent that sized capacity from `/etc/subuid`, which McHarbor does not use | Nothing to fix. McHarbor gives each unprivileged container its own 65536-id block from host id `1000000` upward and the agent writes the id map as root; the item now says so, and names any `/etc/subuid` or `/etc/subgid` entry that reaches into that block |
| A container's memory throttle never slows it down | An agent at build `202609271856` or older wrote the throttle as `memory.low`, a protection floor | Update the agent and restart the container; the throttle is then written as `memory.high` |

A feature plugin has no connection to test. What to check after installing is that its
sidebar section appears. It stays hidden until a role holds at least the `view` key it
declares.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i addon-lxc
```

This plugin owns tables, so a failure at install time may be a migration rather than a
configuration problem. Its migrations are `0001_init.sql` through `0004_backup_idmaps.sql`.

## Related

- [Configuration](./configuration.md): every field and what it is for
- [System Containers](./README.md): what it is and how it works
