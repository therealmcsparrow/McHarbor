# Networks: Troubleshooting

What goes wrong with networks. Two are Docker's rules rather than McHarbor's (the
built-in networks are protected, and macvlan needs a parent interface and a subnet), and
two are about which network a container is actually on.

| Symptom | Usual cause | Fix |
|---|---|---|
| Removing a built-in network is refused | `bridge`, `host` and `none` are protected | Expected; they belong to Docker |
| Bulk remove partly fails | Some networks still have attached containers | Disconnect those containers; the rest of the selection already went |
| Containers on the same network cannot reach each other | Internal-only network, or the wrong network attached | Check the Internal flag and which networks each container is on |
| macvlan creation fails | No parent interface, or no subnet and gateway | macvlan needs both, unlike bridge |
| Creation is refused naming the subnet, gateway or IP range | A value is not in the form Docker accepts: a gateway with a `/prefix`, a start-end IP range, or an address outside the subnet | Correct the field the message names; see [Fields → IPAM values](./fields.md#ipam-values) |
| The parent interface opens as a text box, saying the agent did not answer | The environment's agent is disconnected or stuck | Restart the agent on the host, or type the interface name |

## When a control is disabled

A disabled control with a tooltip naming a permission is RBAC, not a fault: the tooltip
names the exact key to ask for. A disabled control with no tooltip is usually state: the
action does not apply to the current status of the thing you selected.

## Related

- [Networks](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what each action really does
