# Networks: Fields

Every column, filter and form field on Networks, across all six drivers: what the list
reports about a network, and what creating one asks for, including the IPAM and
parent-interface fields macvlan and ipvlan need.

## Columns

| Column | What it is |
|---|---|
| Name | Network name |
| ID | Network id |
| Driver | bridge, overlay, macvlan, ipvlan, host, none |
| Scope | local or swarm |
| Subnet | Assigned or configured |
| Gateway | Assigned or configured |
| Internal | Whether outbound access is restricted |
| Containers | How many are attached |
| Created | When |

## Filters and facets

| Filter | What it narrows |
|---|---|
| Search | Network name |
| Usage | All, Used, Unused |

## Form fields

| Field | Type | What it is |
|---|---|---|
| Name | text | Required |
| Driver | select | `bridge` for a single host, `overlay` across a swarm, `macvlan` / `ipvlan` for host-attached addressing |
| Parent interface | select | macvlan and ipvlan: chosen from the host's own physical, bond and VLAN interfaces, each listed with its kind, address and MTU. An **Enter manually** entry at the bottom swaps in a text box for a name McHarbor cannot see. See below |
| Mode | select | macvlan and ipvlan driver mode. Each option carries a one-line description of what it does, in the dropdown as well as under it |
| Internal | toggle | "Restrict external access" |
| Attachable | toggle | Overlay: allow containers to be attached by hand |
| IPAM: subnet, gateway, IP range | repeater | Several subnets can be added. Required in practice for macvlan; assigned by Docker for bridge |
| Custom options | repeater | Driver-specific key-value pairs. Parent and mode come from their own fields |
| Labels | repeater | Key-value labels |

## Where the interface list comes from

McHarbor's own container cannot see the host's network interfaces: `/sys/class/net` is
network-namespaced, so from inside the container it lists its own veth and `lo`. The list
is therefore built from these sources, and the field says which one a suggestion came from:

| Source | What it contributes |
|---|---|
| **The agent** | Every physical, bond and VLAN link the host actually has. `veth`, `tap` and bridge links are excluded (a bridge is accepted by the kernel and defeats the point of macvlan), as are McHarbor's own managed compute links. A link that is down is still offered, because an uplink is often configured before it is brought up |
| **This host** | A **native** McHarbor on an environment reached through its local Docker socket reads its own host's links from `/sys/class/net`, with the same filtering as the agent. Only a native install can: there, McHarbor's interfaces are the host's |
| **Networks already using one** | Any parent already named by an existing macvlan or ipvlan network on this environment |

When nothing can be discovered, the field opens straight into manual entry and says why,
rather than showing an empty dropdown:

- **A containerized McHarbor on a local-socket environment** cannot see the host's
  interfaces from inside its container.
- **An agent that does not answer**: "The agent of this environment did not answer, so its
  interfaces cannot be listed." Check that the agent is connected, and restart it on the host
  if it is stuck. The failure is also logged on the McHarbor side.

Free text is always allowed.

## IPAM values

Each IPAM row is checked before Docker sees it, and a refusal names the field instead of
reading "invalid request body". Surrounding spaces are trimmed.

| Field | Accepted | Refused with |
|---|---|---|
| Subnet | CIDR, such as `192.168.1.0/24` | "Subnet must be in CIDR notation, for example 192.168.1.0/24." |
| Gateway | One IP address inside the subnet, no `/prefix` | "Gateway must be a single IP address, for example 192.168.1.1, without a /prefix." or "The gateway must lie inside the subnet." |
| IP range | A CIDR block inside the subnet, such as `192.168.1.192/27`; not a start-end range | "IP range must be a CIDR block inside the subnet, for example 192.168.1.192/27, not a start-end range." or "The IP range must lie inside the subnet." |
| Auxiliary addresses | Single IP addresses | "Auxiliary addresses must be single IP addresses." |

## Shared list behaviour

Every list screen inherits the same controls: free-text search, a filter mode of
**Contains**, **Exact** or **Regex** (an invalid expression says so rather than silently
matching nothing), saved filters and searches, selection with a bulk bar, and pagination
showing "X to Y of Z" with a rows-per-page control.

## Related

- [Networks](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what you can do with these fields
