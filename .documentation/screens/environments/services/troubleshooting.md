# Services: Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| There is no **Services** tab | The environment's agent is not native (install kind `container`, or empty on an agent older than 3.5) | Install the agent as a binary under systemd; the Services tab needs the host's systemd, which a container cannot reach |
| **Update the agent to use Services** | The agent predates build `202609271429` of 2026.09.01. It would drop the request rather than refuse it | Update the agent (Settings → Updates → Agents). An agent reporting build `dev` was compiled without the build stamp and is treated as too old |
| **This host does not run systemd** | The agent reported no `host_services` capability: PID 1 is not systemd, or `systemctl` is not on `PATH` | Nothing to manage on this host |
| **The agent is not connected** | The agent is offline | Wait for it to reconnect, or check it on the host (`systemctl status mcharbor-agent`) |
| Stop or Disable is greyed out with a shield | The unit is [protected](./README.md#protected-units) | Use the host terminal if you really mean it; McHarbor will not do it |
| `409 err.host_services.protected` from the API or CLI | Stop or disable of a protected unit | As above |
| **systemd refused the service action: Job for … failed** | systemd ran the action and the unit failed: a bad `ExecStart`, a port already in use, a missing file | Read the reason in the dialog; the unit's own error icon and **Logs → journal** show more |
| After a save nothing changed | The override is loaded but the running process predates it, or **Reload systemd after saving** was off | Reload systemd if it was off, then **Restart** the unit |
| A verify warning after saving | `systemd-analyze verify` found something, often an unknown key (a typo) or a harmless complaint about the vendor unit | Read it; the override is saved either way. Fix a typo and save again |
| Restarting the agent's unit shows the host as disconnected | Expected: the agent restarts and reconnects a few seconds later | Wait; the list refreshes once it is back |
| Revert left `<unit>.d` behind | Another drop-in lives in the same directory (a package or another tool put it there) | Intended: only `override.conf` belongs to McHarbor |
| The list says it was cut short | The host reported more units than fit the 4 MiB output cap or the 5000-unit list limit | Search narrows the list; the CLI returns the same cut list |
