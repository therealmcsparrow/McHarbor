# Virtual machines: display profiles

A display profile is the set of SPICE server options a machine starts with. The browser console
decodes in JavaScript, so the options decide whether it spends its time decoding or drawing.

The **Display profiles** tab sits between **Virtual machines** and **Conversion** on the add-on's
page. Changing a profile, or the profile a machine uses, applies at the machine's **next start**; a
running machine keeps the options it booted with.

## Built-in profiles

Three read-only presets are seeded with fixed ids. To change one, **Duplicate** it.

| Profile (id) | Images | JPEG on slow links | Video streaming | Audio compression |
|---|---|---|---|---|
| **Fast LAN** (`builtin-fast-lan`), the default | Off | Never | Detected video only | On |
| **Balanced** (`builtin-balanced`) | Automatic (LZ) | Automatic | Detected video only | On |
| **Low bandwidth** (`builtin-low-bandwidth`) | Automatic (LZ) | Always | All | On |

All three keep audio compression on, because the browser client plays only Opus, and none uses GLZ
or zlib-over-GLZ, which it cannot draw. A machine with no chosen profile uses Fast LAN, and that is
stored as no choice at all, so the default can move without rewriting every machine.

## Custom profiles

**New profile** and **Edit** open a dialog with these options. A value outside QEMU's set is refused.

| Option | Values |
|---|---|
| Image compression | `auto_glz`, `auto_lz`, `quic`, `glz`, `lz`, `off` |
| JPEG on slow links | `auto`, `never`, `always` |
| zlib over GLZ on slow links | `auto`, `never`, `always` |
| Video streaming | `off`, `all`, `filter` |
| Audio compression | `on`, `off` |
| Agent mouse | `on`, `off` |

Names are unique. The GLZ choices and QUIC are accepted but the bundled browser client draws
neither well, so the dialog says so beside each option. Video codec is not offered: it needs a
GStreamer-enabled spice-server that nothing probes for, and a wrong value stops QEMU starting.

**Delete** is refused for a built-in, and for a profile machines still use. The refusal is a `409`
with `err.vm.display_profile_in_use` and the count; confirming with the machines listed deletes
the profile and moves them to Fast LAN (`?reassign=true`).

## Choosing one for a machine

In the machine's **Settings**, the **Display profile** select. From the terminal:
`mcharbor kvm vm update <id> --display-profile <profileId>`.

## Routes and permissions

| Route | Key |
|---|---|
| `GET /api/vms/display-profiles`, `GET /api/vms/display-profiles/{profileId}` | `vm.view` |
| `POST /api/vms/display-profiles` | `vm.manage` |
| `PUT /api/vms/display-profiles/{profileId}` | `vm.manage` |
| `POST /api/vms/display-profiles/{profileId}/duplicate` | `vm.manage` |
| `DELETE /api/vms/display-profiles/{profileId}` | `vm.manage` |

Writes are audited as `plugin.kvm.display_profile.created`, `.updated`, `.duplicated` and `.deleted`.

## Command line

`mcharbor kvm display-profile list`, `get`, `create`, `update`, `duplicate` and `delete` (with
`--reassign`) call the same routes with the same keys. `create` leaves an unset option at the Fast
LAN value.

## What goes wrong

| Symptom | Cause | Fix |
|---|---|---|
| A new profile changes nothing | The machine is still running with the options it booted with | Restart it |
| Parts of the screen are never drawn | A GLZ image choice the browser client cannot decode | Use `lz`, `auto_lz` or `off` |
| The console has no sound | Audio compression off sends raw audio the browser cannot play | Turn it on |
| Edit is disabled | The profile is built in | Duplicate it |

## Related

- [SPICE console](../spice-console/README.md)
- [Virtual machines](../README.md)
