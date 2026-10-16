# Virtual machines: SPICE console

A SPICE console gives a guest clipboard sharing and resize-to-window through the SPICE agent,
which VNC cannot. It is offered beside VNC and Serial in the browser, and as a `.vv` file for the
native `remote-viewer` application.

## What the host needs

- A **QEMU built with SPICE**. The agent's capability probe reports it as the `spice` feature
  only when its QEMU starts with a SPICE server: listing the `query-spice` command is not
  enough, because Debian and Ubuntu build QEMU in modules and keep that command in the core
  binary even when the SPICE module is missing. On Debian and Ubuntu install
  `qemu-system-modules-spice`; SUSE, Arch and Alpine QEMU packages have it built in; RHEL 9 and
  later `qemu-kvm` does not. A guest on a host without it starts with a VNC console only.
  The probe is cached, so restart the agent after installing the module.
- An agent that reports the feature. An older agent has no `spice` key in its capability
  answer, which reads as unsupported.

Without either, both SPICE controls are disabled with the reason, and
`GET /api/vms/{id}/console.vv` answers with `err.vm.spice_unavailable`: "This machine's host
cannot serve a SPICE console. Its QEMU has no SPICE support, or its agent is too old."

## Which guests have it

SPICE is part of the guest's QEMU command line, so it exists only on a guest **started** after the
host gained support. The agent adds a SPICE server on a unix socket beside the VNC one, with the
guest-side channel the SPICE agent uses, and only for a guest whose display is not `none`. A guest
that was already running needs a restart to get SPICE. The socket is local to the host and is never
published on a TCP port by QEMU.

## In the browser

1. Open the machine's **Console** tab, which needs `vm.console`.
2. Choose **SPICE** under **Console type** (the other choices are **VNC** and **Serial**). The
   choice is disabled, with the reason, when the host does not report `spice`.

The browser client is `@spice-project/spice-html5` 0.2.1, which is loaded only when you first
connect with SPICE. Each SPICE channel opens as its own console stream through McHarbor's console
gateway, so the session is authorised by McHarbor and never reaches QEMU directly. A connection
uses one **ticket per channel**: the ticket request takes a `channels` count from 1 to 8, each
ticket is single use, and all of them share one session. The limit of four viewers per guest counts
sessions, so a SPICE session with several channels still takes one seat.

Known limit: upstream spice-html5's Ctrl+Alt+Del is broken, so the SPICE console has no key menu.
Use VNC when you need to send it.

## Display size and keyboard

The console toolbar's **Display size** picks **Resize guest to window** (the default) or **Scale to
fit**; the choice is remembered per browser. Resizing asks the guest for a new resolution after the
pane stops moving for 200 ms, and the console says why when the guest does not follow:

| Console | What the guest needs |
|---|---|
| SPICE | `spice-vdagent` running in the guest, with a display adapter of virtio-gpu or QXL. `qemu-guest-agent` is a different agent and does not resize the display |
| VNC | A virtio-gpu adapter, its driver in the guest and a desktop that follows display changes, on a host QEMU 6.0 or later (reported as `display_resize`). QXL does not resize over VNC |

A machine started before its display adapter changed needs a restart. When nothing answers within
five seconds the console shows the reason beside the toolbar instead of silently scaling.

While the console has focus it owns every keystroke: McHarbor's own shortcuts stay silent inside
it, so typing in the guest never navigates the app. Click outside the console to use them again.

How the guest is drawn is set by the machine's [display profile](../display-profiles/README.md),
which applies at the next start.

## In remote-viewer

**Remote viewer file** on the console toolbar opens a small form:

| Field | Meaning |
|---|---|
| **Address to connect to** | The host name or address written into the file. Empty uses the first address the agent reports. Set it when you reach the guest's host by a name the agent cannot know. Only letters, digits, `-`, `.` and IP addresses are accepted. |
| **Download .vv file** | Issues a ticket and downloads `<machine name>.vv`. |

The file holds a one-time password that works for two minutes, so open it straight away. The agent
opens one short-lived TCP listener on the host per guest (at most 16 at a time) and checks the
password inside SPICE's own ticket exchange before any byte reaches QEMU. The file asks
`remote-viewer` to delete it once read.

The listener's port is chosen by the kernel for each ticket. A host behind a firewall pins it by
setting `MCHARBOR_SPICE_RELAY_PORT` in the agent's environment and opening that port. The machine
must be running, otherwise the route answers with `err.vm.not_running`.

The route is `GET /api/vms/{id}/console.vv?host=<address>`. It needs `vm.console`, and every issue
is audited as `plugin.kvm.vm.spice_ticket_issued` at **warning** severity, because the file carries
a credential.

## Serial console resize

The serial console keeps the terminal size in step with the pane by running `stty` inside the guest
through the guest agent, debounced, and off the read loop. Without the guest agent the resize is a
no-op: the guest keeps the size it started with, and the serial pane says "Resizing this terminal
needs the guest agent. Without it the guest keeps the size it started with." The host reports the
capability as `serial_resize`.

## Permissions

| Key | Needed for |
|---|---|
| `vm.console` | The SPICE console, the serial console, the VNC console and the `.vv` file |

## What goes wrong

| Symptom | Cause | Fix |
|---|---|---|
| **SPICE** is disabled | The host's QEMU has no SPICE, or the agent is older | Install a QEMU with SPICE (`qemu-system-modules-spice` on Debian and Ubuntu), restart the agent, or update it |
| **SPICE** is offered but the guest shows no SPICE display | The guest was started before the host supported SPICE, or its display is `none` | Restart the guest, and give it a display |
| remote-viewer cannot connect | The ticket expired (two minutes), or the listener port is blocked | Download a new file, or pin `MCHARBOR_SPICE_RELAY_PORT` and open it |
| remote-viewer connects to an address you cannot reach | The agent reported its own address | Set **Address to connect to** |
| A fifth viewer is refused | Four sessions already open on that guest | Close one |

## Licence

The SPICE browser client is LGPL-3.0-or-later code bundled into the add-on. Its notice, source and
replacement instructions are in `THIRD_PARTY_NOTICES.md` in the plugin's folder.

## Related

- [Virtual machines](../README.md)
- [Display profiles](../display-profiles/README.md)
- [QEMU features](../qemu-features.md)
- [Host requirements](../../compute-foundation/host-requirements.md)
