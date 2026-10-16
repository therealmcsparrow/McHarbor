# Compute Foundation troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| The capability panel reports "The check did not finish" | Usually a host without systemd | See the compute section's troubleshooting table; the probe refuses outright rather than reporting a negative. |
| Uninstalling it fails | Virtual Machines or System Containers is still installed | It is a dependency; remove the last dependent first. |
| A guest never gets an address from your DHCP server | The request is dropped on the way out (hypervisor MAC filtering, Wi-Fi, port security), or the answer is dropped on the host before the guest sees it | Follow [A guest gets no address](./guest-networking.md#a-guest-gets-no-address). It tells the two apart from one `tcpdump`. |
| A macvlan guest's interface shows RX `errors` = `dropped` = `packets` | The macvtap was created with `txqueuelen` 0 by an agent built before `202609250111` | `sudo ip link set <iface> txqueuelen 1000` fixes it live; update the agent to fix it for good. |
| A macvlan guest reaches the LAN but not its own host | Host access is off, or has no address | Turn on host access with a free address in the LAN's prefix. See [macvlan](./guest-networking.md#macvlan-guests-directly-on-an-adapter). |
| A file or folder made through a pool's file browser inside `containers/<id>/rootfs/` shows as `nobody` inside that system container | An agent from before addon-lxc `2026.09.13` created it as host root, outside the container's id range | Update the agent: new entries there then belong to the container's root. Saving a file again repairs it; a folder needs `chown` on the host. See [Restoring into another id range](../system-containers/backups/README.md#restoring-into-another-id-range) |
| Deleting a file or folder in a pool's file browser fails with *… is open by running guest <id>; stop it before deleting* | A running guest, or another process, has a file under that path open, usually its disk. Deleting it would only unlink the name; the guest keeps the data until it exits, and it is then gone for good | Stop the named guest first. If McHarbor no longer lists it (its record was lost), stop it on the host: `sudo systemctl stop mcharbor-vm-<id>.service`, but only once you are sure its disk is not wanted. Needs an agent built from `202609261240` on |

A feature plugin has no connection to test. What to check after installing is that its
sidebar section appears. It stays hidden until a role holds at least the `view` key it
declares.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i addon-compute
```

This plugin owns tables, so a failure at install time may be a migration rather than a
configuration problem. Its migrations are `0001_init.sql`.

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Compute Foundation](./README.md): what it is and how it works
