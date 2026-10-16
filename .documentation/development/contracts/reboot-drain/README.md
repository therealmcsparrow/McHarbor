# Reboot Drain Contract

How a plugin lets a host reboot stop its guests cleanly. A reboot of an agent environment used to
read plugin tables directly. It now asks each plugin through two routes the plugin declares.

## Declaring it

```yaml
contributes:
  capabilities:
    - kind: reboot-drain
      subject_type: vm
      routes:
        running: "/api/vms/reboot-drain"
        stop: "/api/vms/reboot-drain"
  routes:
    - method: GET
      path: "/api/vms/reboot-drain"
      requires_permission: "host_updates.view"
    - method: POST
      path: "/api/vms/reboot-drain"
      requires_permission: "host_updates.reboot"
      audit_required: true
      audit_action: "plugin.kvm.vm.reboot_drain_stop"
      audit_entity_type: "vm"
```

| Field | Rule |
|---|---|
| `subject_type` | `^[a-z][a-z0-9_]{0,62}$`. One declaration per subject type. |
| `routes.running` | A `GET` route the plugin declares. |
| `routes.stop` | A `POST` route the plugin declares. |

Both routes must have no `{placeholder}`, must not be `public`, and must carry a
`requires_permission` that the plugin adds under `adds_permissions` or consumes under
`consumes_permissions`. The host calls them as the reboot's requester, so a route with no key would
reach guests the requester may not touch. Validation is in `core/plugincaps` and mirrored by
`mcharbor plugin dev validate`, against the shared cases in `src/contracts/plugincaps/cases.yaml`.

## What the host does

For an agent environment whose agent supports compute, the host calls each loaded plugin's routes
in-process through the plugin router, as the user who asked for the reboot, with
`?env=<envId>`. The inventory read runs as the viewing user.

| Route | Request | Answer |
|---|---|---|
| `running` | `GET`, `?env=<envId>` | `{success, data: [{id, name, graceful}]}` |
| `stop` | `POST`, `?env=<envId>`, body `{"id": "<guest>"}` | `{success, data: {escalated: bool}}` |

The host waits 20 s for each stop request and treats one it stopped waiting for as still in
progress, then polls `running`.

## Shipped declarations

| Plugin | Subject | Routes | Keys | Audit | Behaviour |
|---|---|---|---|---|---|
| `addon-kvm` | `vm` | `GET` and `POST /api/vms/reboot-drain` | `host_updates.view`, `host_updates.reboot` | `plugin.kvm.vm.reboot_drain_stop`, entity `vm` | Records a lifecycle intent with source `host_reboot`, then an ACPI shutdown with a 6-hour grace. |
| `addon-lxc` | `ct` | `GET` and `POST /api/system-containers/reboot-drain` | `host_updates.view`, `host_updates.reboot` | `plugin.lxc.ct.reboot_drain_stop`, entity `container` | Halts with the container's own init system, systemd when it has none. |

Both plugins list `host_updates.view` and `host_updates.reboot` under `consumes_permissions`.

CLI: `mcharbor kvm reboot-drain list|stop` and `mcharbor lxc reboot-drain list|stop`. `list` needs
`host_updates.view` and an environment; `stop` needs `host_updates.reboot` and `--id`.

## The reboot inventory

`GET /api/system/os-reboot/inventory` (and the `/self` form) reports one `guests` count: every guest
any reboot-drain plugin reported. The `vms` and `lxc` counts are gone, which breaks an API client
that read them. The reboot dialog reads "Detected on this host: N guests running (virtual machines
and system containers)."

## What goes wrong

| Symptom | Cause |
|---|---|
| The declaration is dropped | A route is missing, public, carries a placeholder, or has no owned `requires_permission`. Check the loader log. |
| `guests` is `0` while guests run | No loaded plugin declares a drain for them, or the agent does not support compute. |
| A guest never stops | Its stop request exceeded 20 s and `running` keeps listing it. The reboot waits. |

## Related

[Capability declarations](../capabilities.md), [Agent RPC](../agent-rpc/README.md).
