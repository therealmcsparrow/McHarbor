# Put guests on your network

Compute guests (virtual machines and system containers) reach the network through one of
four modes. This page explains which to pick, why a Docker macvlan network is not one of
them, how to convert a host that already runs Docker macvlan so guests and containers
can share the LAN, and how to find out why a guest has no address.

## The four modes

| Mode | What McHarbor does | Guests get addresses from | Closest Docker equivalent |
|---|---|---|---|
| **bridged** | Creates only the guest's own interface and attaches it to a bridge you already have. Never touches the bridge or the physical link. | your own DHCP server | `macvlan` / `ipvlan` |
| **external** | Attaches one of the host's adapters to a bridge and moves the host's address onto it. Same idea as an external switch in Hyper-V or bridged networking in VMware. | your own DHCP server | `macvlan`, when no bridge exists yet |
| **managed** | Creates its own bridge beside your adapters, serves DHCP on it, and masquerades guest traffic out through an interface you choose. | McHarbor | the default `bridge` network |
| **macvlan** | Stacks each guest directly on an adapter with its own MAC address, created when the guest starts and removed when it stops. The adapter is not reconfigured and keeps the host's address. | your own DHCP server | `macvlan` |

Pick **managed** when the host has one adapter and no bridge, and the guests do not need to
be reachable as peers on the LAN. Pick **bridged** when a bridge already exists. Pick
**external** to have McHarbor build the bridge for you. Pick **macvlan** to put guests on
the LAN without touching the host's adapter at all, including one that already carries
Docker macvlan containers.

### Managed networks serve DHCP from the agent

A managed network's DHCP server runs inside the agent process, bound to that network's
bridge. The bridge and the guest taps are kernel state and outlive the agent; the DHCP
server does not, so the agent persists each managed network's addressing and restores the
servers when it starts. A guest that boots while the agent is down keeps its address until
the lease expires, and can renew once the agent is back.

If a managed network's range is left empty, the agent derives one from the subnet, skipping
the network, broadcast and gateway addresses: a `10.100.0.0/24` network with gateway
`10.100.0.1` serves `10.100.0.2` to `10.100.0.254`.

## Creating the network

**Compute → Networks → Add network.** Creating one needs `compute.manage`; deleting one
needs `compute.delete`, which is a separate key because removing a network takes the guests
on it off the network with it.

Three fields are asked for whatever mode you pick, then the form changes shape.

| Field | What it is |
|---|---|
| **Host** | The agent environment the network is built on. A network belongs to one host; there is no cluster-wide network. It defaults to whatever the topbar has selected. |
| **Network name** | Yours, and only McHarbor's label for it. It is not the bridge name and the guests never see it. It must be unique on that host. |
| **Mode** | `bridged`, `managed`, `external` or `macvlan`, the four above. Bridged is preselected because it is the one that cannot break the host. |

If every picker on the form is empty and it says the agent cannot see the host's network,
the agent is running in its own network namespace: a container without host networking.
Nothing on this page works from there; rerun the agent with `--network host` and
`NET_ADMIN`.

### Bridged: join a bridge you already have

| Field | Notes |
|---|---|
| **Bridge** | Every bridge the agent can see on the host, with its address. Only bridges; a plain NIC does not appear, because a tap cannot be enslaved to one. |
| **VLAN tag** | Optional, 1 to 4094. Leave it empty unless guests belong on a specific VLAN; the guest itself stays unaware of the tag either way. |

Nothing about the host changes. McHarbor creates the guest's tap and attaches it to the
bridge, and your own DHCP server addresses the guest.

### Managed: McHarbor's own network, with its own IP plan

This is the one where you write the addressing down. McHarbor creates a bridge beside your
adapters (it picks the bridge name itself), serves DHCP on it, and masquerades guest
traffic out of an interface you choose. Your adapters are not touched and not attached to
anything.

| Field | Default | Notes |
|---|---|---|
| **Subnet** | `10.100.0.0/24` | The guest network in CIDR. Pick a range nothing else on your LAN uses, or the guests will reach the wrong side of a collision. |
| **Gateway address** | `10.100.0.1` | The address the bridge itself takes, and what guests get as their default route. It has to sit inside the subnet. |
| **Hand out addresses on this network** | on | McHarbor's DHCP server. Off means you address the guests statically. |
| **Route guest traffic out through the host** | on | NAT. Off makes the network isolated: guests talk to each other and to nothing else. |
| **Outbound interface** | — | Only asked when NAT is on, and then required. The interface carrying the host's default route is marked and is the normal choice. It is not modified, not bridged, and keeps its own address. |

A worked plan for a lab host whose LAN is `10.35.0.0/16`:

| | |
|---|---|
| Subnet | `10.100.0.0/24`, outside the LAN's range, so nothing collides |
| Gateway | `10.100.0.1` |
| DHCP | on, pool derived as `10.100.0.2` to `10.100.0.254` |
| NAT | on, out through `enp1s0` |

Guests get `10.100.0.x`, reach the internet through the host, and are **not** reachable as
peers from the LAN. If they need to be, that is bridged or external, not managed.

The form does not ask for a DHCP range; the agent derives one from the subnet, as
[above](#managed-networks-serve-dhcp-from-the-agent).

Addresses handed out are visible per network under **leases**: IP, MAC, hostname and the
guest holding it. A bridged or external network has no leases page, because McHarbor is not
the DHCP server there.

### External: take over one of the host's adapters

| Field | Notes |
|---|---|
| **Adapter to share** | Physical, bonded and VLAN interfaces. One already carrying other interfaces on top of it (a macvlan or VLAN child) is shown as *in use* and refused; see the section below for why. |
| **I understand this reconfigures the host adapter** | A confirmation switch. The Add button stays disabled until it is on. |

The adapter joins a bridge McHarbor creates, and the host's address and routes move onto
that bridge. Picking the adapter carrying the default route reconfigures the connection you
are working over, and the form says so, but **the agent rolls the change back after 45
seconds** unless it can still reach McHarbor, so a host that drops off comes back by itself
without anyone reaching the console.

### macvlan: guests directly on an adapter

| Field | Notes |
|---|---|
| **Parent interface** | The adapter guests are stacked on. It is not reconfigured: its address, its routes and the host's traffic on it stay exactly as they are. |
| **VLAN tag** | Optional, 1 to 4094. With a tag, McHarbor creates a VLAN sub-interface on the parent and stacks guests on that instead. |
| **Mode** | How guests on this host reach each other (see below). Empty means `bridge`. |
| **Let this host reach its guests** | Host access. Creates a macvlan interface for the host itself on the parent, because the kernel never forwards between a parent and the children stacked on it. |
| **Address for this host** | Only with host access. An address and prefix on the LAN, like `10.35.36.250/16`. Empty brings the interface up with no address. |

| Mode | Guests on this host reach each other | Host access |
|---|---|---|
| `bridge` | Directly | Allowed |
| `vepa` | Only if the switch reflects traffic back (hairpin) | Allowed |
| `private` | Never, not even through the switch | Allowed |
| `passthru` | There is only one guest; it owns the adapter, MAC included | Refused |

Every mode reaches the rest of the LAN and the router, so every mode can take a DHCP
address. The mode only decides what happens *between* guests on the same host. `bridge`
is the usual choice.

The guest's interface is created when the guest starts, with the guest's own MAC address,
and removed when it stops. McHarbor runs no DHCP and no NAT on a macvlan network; your own
DHCP server addresses the guest.

**Host access, in practice:**

- **The prefix must match the LAN.** Give the host's address the LAN's own prefix
  (`/16` on a `10.35.0.0/16` LAN). With a narrower one, like `10.35.179.1/24`, the host can
  only reach guests that happen to land in `10.35.179.x`.
- **Pick an address nothing else uses.** It has to be free, and outside your DHCP server's
  pool if it cannot be reserved there. Never the router's.
- **Host access with no address does nothing on its own.** The interface comes up with no
  address, and unless something on the host configures it, the host still cannot reach the
  guests. Turn host access off, or give it an address.
- **Without host access, the host and its guests cannot see each other at all.** That is how
  macvlan works, not a fault: a guest that cannot reach a service on its own host, while
  reaching everything else on the LAN, is exactly this.

### What the form tells you before you submit

Below the mode-specific fields the form lists, in order, exactly what will be done **on the
host** if you press Add, and what will be undone if you later delete the network. The three
modes change the host by very different amounts, and the difference is not obvious from the
names.

For **external** mode it also states the contract and the caveats, because this is the one
that touches the connection you are working over:

- The chosen adapter is enslaved to a bridge McHarbor creates, and the adapter's IPv4
  addresses and routes are moved onto that bridge.
- The agent arms a **45-second rollback before it replies**. If the host loses its
  connection to McHarbor, the reply never arrives, the request fails with a generic error
  and the agent reverts on its own. A failure of that exact shape (a long pause, an
  unhelpful error, the agent briefly offline) is this rollback working, not the host
  being broken.
- **IPv6 addressing is not carried onto the bridge.** Only IPv4 addresses and routes move.
- **Nothing tells the host's own network manager that the address moved.** NetworkManager,
  systemd-networkd or a DHCP client may pull it back; pin the adapter in the host's own
  configuration if it does.

### What the server refuses

Validation is server-side, so the same rules apply to the API:

| Refused | Reason |
|---|---|
| A bridged network with no bridge | Nothing to attach a tap to |
| A managed network with no subnet or no gateway | There is no addressing to serve |
| A managed network with NAT on and no outbound interface | Nowhere to masquerade to |
| An external network with no adapter | Nothing to share |
| A macvlan network with no parent interface | Nothing to stack guests on |
| A macvlan network with DHCP or NAT on | The LAN it joins addresses it; McHarbor would be a second DHCP server on a segment that already has one |
| Host access on a `passthru` network | The single guest owns the adapter; the host cannot have an address on it too |
| A host address that is not an address and prefix | `10.35.36.250` is refused; `10.35.36.250/16` is accepted |
| A VLAN tag outside 1 to 4094 | Not a VLAN |
| A name already used on that host | Names are unique per host |

### Attaching a guest

A network is not used until a guest points at it. **Compute → Virtual Machines → Create**
(or **Add NIC** on an existing guest) offers every network on the chosen host, preselecting
the one marked *default*. System containers work the same way.

Changing a running guest's network needs the guest's NIC changed, not the network. Editing
a network does not move guests already on it.

## Why a VM cannot join a Docker macvlan network

A Docker `macvlan` network has no bridge. Docker creates a macvlan sub-interface directly on
the parent NIC for each container, so there is no device for a guest's tap to be enslaved
to. Compare the other Docker drivers: a `bridge` network has a `br-<id>` interface, and a
guest can be pointed at that. A macvlan network has nothing of the kind, which is why it
never appears in the network picker.

`external` mode would normally solve this by turning the NIC into a bridge, and McHarbor
refuses it here on purpose: a link carrying VLAN or macvlan interfaces is rejected with
`uplink_busy` rather than adopted, because adopting it would tear the floor out from under
every macvlan container on that host.

The kernel enforces the same rule. Enslaving such a NIC fails:

```
enp1s0: Failed to set master interface: Device or resource busy
enp1s0: Failed
```

The **macvlan** mode is the way round this that leaves the host alone: it stacks each guest
on the same adapter the Docker macvlan network uses, beside the containers, with nothing
converted. Guests and containers then share the LAN. They can reach each other when both
sides use `bridge` mode on the same parent. The conversion below is still the route when you
want a real bridge.

## Converting a macvlan host to a bridge

The end state is a bridge that everything hangs off: the host's address, the Docker macvlan
network's parent, and the guests. Containers and guests then share the LAN and can reach
each other and the host, with no shim interface needed.

**The order matters.** The NIC must be free of macvlan children *before* it can join the
bridge. Doing it the other way round leaves the address on a bridge with no port, and takes
the host off the network.

### 1. Prepare a way back in

Reconfiguring the adapter you are connected over can lock you out. Two precautions, both
worth taking:

- **Console access.** A second interface on the same LAN is worth having, but it is not a
  dependable rescue path: `netplan apply` re-applies *every* interface, so a WiFi fallback
  can lose carrier in the same moment as the wire. On the host this guide was written
  against, both paths went away together for five minutes.
- A scheduled revert armed *before* the change, so an unreachable host fixes itself:

```bash
sudo cp -a /etc/netplan/50-cloud-init.yaml /root/netplan-pre-bridge.yaml
sudo systemd-run --on-active=300 --unit=netplan-revert \
  /bin/bash -c 'cp -a /root/netplan-pre-bridge.yaml /etc/netplan/50-cloud-init.yaml; netplan apply'
```

Cancel it with `sudo systemctl stop netplan-revert.timer` once the change is verified.

### 2. Free the NIC

Stop the containers on the macvlan network, remove the network, and delete any shim
interface. Record what you are about to recreate first, the container addresses especially:

```bash
docker network inspect homeLAB-network > /root/homeLAB-network.json
docker network inspect homeLAB-network -f '{{json .Containers}}'   # names, IPs, MACs
sudo systemctl stop macvlan-shim          # if a shim unit exists
sudo systemctl stop docker                # or disconnect each container
```

A macvlan network built from a **config-only** network keeps `parent=` on the config-only
network, not on the macvlan itself. Check both:

```bash
docker network inspect homeLABconfig -f 'configOnly={{.ConfigOnly}} parent={{index .Options "parent"}}'
```

**Before deleting anything, find out which members can simply be reattached afterwards.**
A container whose *primary* network mode is this macvlan cannot be reconnected to a
recreated network. It has to be recreated from its own definition:

```bash
for c in $(docker network inspect homeLAB-network -f '{{range .Containers}}{{.Name}} {{end}}'); do
  printf '%-16s %s\n' "$c" "$(docker inspect -f '{{.HostConfig.NetworkMode}}' "$c")"
done
```

Any container whose network mode is the network's **ID** rather than `default` or `bridge`
is in that group. Confirm you hold its compose file or `docker run` line *now*, while the
network still exists. After the network is gone, `docker inspect` still shows the
container's settings, but nothing will recreate it for you.

Then confirm the NIC is genuinely free before going near netplan; this is the condition the
whole procedure turns on:

```bash
ip -d link show | grep -c macvlan     # must be 0
```

### 3. Create the bridge

```yaml
network:
  version: 2
  ethernets:
    enp1s0:
      dhcp4: no
  bridges:
    br0:
      dhcp4: no
      interfaces: [enp1s0]
      addresses: [10.35.36.254/16]
      routes:
        - to: default
          via: 10.35.254.101
          metric: 100
      parameters:
        stp: false
        forward-delay: 0
```

Two things netplan will refuse, both easy to hit:

- **Two unmetered default routes.** If a second interface (WiFi, say) also declares a
  default route on the same subnet, give each an explicit `metric:` or netplan rejects the
  file with *"Conflicting default route declarations for IPv4"*.
- **A config that never validates.** Run `sudo netplan generate` and read its output before
  applying. It can print `Error:` while still exiting 0, so check the text, not just the
  exit status.

Apply it detached from your session, so losing the connection does not kill the apply:

```bash
sudo systemd-run --unit=netplan-apply-now /usr/sbin/netplan apply
```

Then verify `br0` holds the address and `enp1s0` is a port of it:

```bash
ip -br addr show br0
bridge link show br0
```

### 4. Recreate the Docker network on the bridge

```bash
docker network rm homeLAB-network
docker network rm homeLABconfig
docker network create --config-only --subnet 10.35.0.0/16 --gateway 10.35.254.101 \
  -o parent=br0 homeLABconfig
docker network create -d macvlan --config-from homeLABconfig homeLAB-network
```

Reattach the containers that can be reattached, those that had this network as an
*additional* network:

```bash
docker network connect --ip 10.35.35.1 homeLAB-network Homeassistant
```

The rest (the ones whose network mode was the old network's ID) refuse:

```
Error response from daemon: could not find a network matching network mode 7dcfb68d2962…
```

**Restarting one is not a fix.** It comes back up and reports healthy while attached to no
network at all, which is worse than failing outright:

```bash
docker restart phpmyadmin          # "succeeds"
docker inspect -f '{{range $k,$v := .NetworkSettings.Networks}}{{$k}}{{end}}' phpmyadmin
                                   # prints nothing
```

Recreate those from their definition instead: `docker compose up -d --force-recreate` in
the project directory, or redeploy the stack from McHarbor. The network name is unchanged,
so it resolves to the new macvlan on `br0`.

Compose stacks that reference the network as `external` need no edit. The name is the
same, only the parent moved.

### 5. Point the shim at the bridge, or drop it

A macvlan shim exists so the host can reach its own macvlan containers. If you keep one,
re-point it at `br0`, because its old parent is now a bridge port:

```ini
ExecStart=/sbin/ip link add macvlan-shim link br0 type macvlan mode bridge
```

### 6. Add the McHarbor network

**Compute → Networks → Add**, mode **bridged**, bridge `br0`. Guests pointed at it take
addresses from your own DHCP server, exactly like the containers.

## Seeing a guest's address

| Where | Source | Needs |
|---|---|---|
| VM list and Overview tab | the QEMU guest agent inside the guest | `qemu-guest-agent` installed and running; on Windows it ships in the virtio-win ISO |
| Compute → Networks → leases | McHarbor's own DHCP server | a **managed** network |
| The guest's console | the guest itself | nothing |

On a **bridged** or **external** network McHarbor is not the DHCP server, so there are no
leases to show and the guest agent is the only in-product source.

A guest with no address at all is usually one of three things: the guest has no operating
system installed yet, its disk is an image the host could not boot, or no DHCP server is
answering on its network. The next section is how to tell which.

## A guest gets no address

On a **bridged**, **external** or **macvlan** network your own DHCP server hands out the
address, so the question is always the same: where between the guest and that server does
the exchange stop? Answer it from the host, while the guest is running and retrying. The
commands below find the guest's interface by its MAC address, which is shown on the guest's
NIC in McHarbor.

### 1. Does the request leave the host, and does an answer come back?

Capture DHCP on the adapter the network uses (the parent interface for macvlan, the bridge
for bridged or external):

```bash
sudo tcpdump -eni enp1s0 'port 67 or port 68'
```

| What you see | What it means | Go to |
|---|---|---|
| Nothing from the guest's MAC | The guest is not asking | [4](#4-the-guest-is-not-asking) |
| `Request from <guest MAC>`, nothing back | The request leaves, the DHCP server never answers it | [2](#2-requests-leave-and-nothing-answers) |
| `Request from <guest MAC>`, a `Reply` back, and the guest keeps asking | The answer reaches the host but not the guest | [3](#3-the-answer-arrives-and-the-guest-never-sees-it) |

A tcpdump without `-v` prints DISCOVER and REQUEST both as `Request`, and OFFER and ACK both
as `Reply`. A guest asking again every few seconds, for minutes, is the guest never having
received an answer.

### 2. Requests leave and nothing answers

The guest's frames carry its own MAC address, and something between the host and the DHCP
server is not accepting that MAC.

- **The host is itself a virtual machine.** A hypervisor drops frames from MAC addresses it
  did not assign. Hyper-V: enable **MAC address spoofing** on the host VM's adapter. ESXi:
  set **Promiscuous mode**, **MAC address changes** and **Forged transmits** to *Accept* on
  the port group. Proxmox: check the firewall and any MAC filter on the host VM's NIC.
- **The parent is a Wi-Fi adapter.** A Wi-Fi client may only send from its own MAC, so
  macvlan does not work over Wi-Fi. Use a wired adapter, or a **managed** network.
- **Switch port security or a MAC limit.** A port that allows one or two MAC addresses drops
  every guest beyond them. The switch's log usually says so.
- **The wrong VLAN.** A tagged network puts the guest on that VLAN; the DHCP server has to
  serve it, and the switch port has to carry the tag.

### 3. The answer arrives and the guest never sees it

The DHCP server is fine; the break is on the host, between the adapter and the guest.
Find the guest's interface and read its receive counters:

```bash
MAC=02:4d:43:02:22:48
IF=$(ip -o link | awk -F': ' -v mac="$MAC" '$0 ~ mac {print $2}' | cut -d@ -f1); echo "$IF"
ip -s link show "$IF"
```

On a macvlan network `$IF` is the guest's macvtap, `<name>@<parent>` in `ip link`. Look at
the `RX` line:

- **`errors` and `dropped` equal to `packets`, rising together**: the kernel receives every
  frame for the guest and discards it before QEMU can read it. Check the queue length:

  ```bash
  cat /sys/class/net/"$IF"/tx_queue_len        # 0 is the fault
  sudo ip link set "$IF" txqueuelen 1000
  ```

  A macvtap's receive ring is sized from its `txqueuelen`, so a length of 0 drops every
  frame while sending keeps working. Agents built before build `202609250111` created every
  macvtap that way. Setting the length takes effect immediately, without restarting the
  guest; updating the agent makes it permanent.

- **`dropped` rising, with some frames received**: the receive ring is full because nothing
  is emptying it. Rule out vhost-net, the kernel helper that moves frames into the guest, by
  turning it off for one test:

  ```bash
  sudo systemctl edit mcharbor-agent          # [Service]  Environment=MCHARBOR_VM_VHOST=off
  sudo systemctl restart mcharbor-agent
  ```

  Stop and start the VM from McHarbor, then read the counters again. If the drops stop,
  vhost-net is the cause on this host. Either way, undo the override afterwards;
  see [Undoing an override](../how-to/agent/README.md#undoing-an-override).

- **No drops, and the capture on `$IF` shows the `Reply`**: the frame reaches the guest's
  interface and the guest is not accepting it. Go to [4](#4-the-guest-is-not-asking).

To see exactly what reaches the guest's own interface:

```bash
sudo timeout 20 tcpdump -eni "$IF" 'port 67 or port 68'
```

And to confirm QEMU holds that interface (exactly one process, `qemu-system`, should hold
its device):

```bash
IDX=$(cat /sys/class/net/"$IF"/ifindex)
sudo fuser -v /dev/tap"$IDX"
Q=$(pgrep -f "qemu-system.*$MAC")
sudo ls -l /proc/"$Q"/fd | grep -E 'tap|vhost'
tr '\0' ' ' < /proc/"$Q"/cmdline | grep -o -- '-netdev [^ ]*'
```

A second holder splits the incoming frames across two queues, and the one nobody reads
drops them.

### 4. The guest is not asking

- Open the guest's console. A guest still in its firmware, or at an installer, has not
  started a DHCP client.
- Check the guest has a driver for its NIC model. A guest without virtio drivers needs the
  NIC set to `e1000e`.
- Check the guest's own network configuration: a static address, or DHCP turned off on that
  interface, never sends a request.

### Other things that look like "no network"

| Symptom | Cause | Fix |
|---|---|---|
| The guest reaches the LAN and the internet, but not the host it runs on | macvlan without host access; the kernel never forwards between a parent and its children | Turn on host access with a free address in the LAN's prefix |
| The host reaches only some of its macvlan guests | The host-access address has a narrower prefix than the LAN | Edit the network and give the address the LAN's prefix |
| Two guests on one host cannot reach each other | `vepa` without a hairpin switch, or `private` | Use `bridge` mode |
| Old `mch…@<parent>` interfaces remain after guests were deleted | Children from earlier starts that were never removed | `ip -d link show \| grep -B1 macvtap` lists them; delete one whose MAC belongs to no guest with `sudo ip link del <name>` |
