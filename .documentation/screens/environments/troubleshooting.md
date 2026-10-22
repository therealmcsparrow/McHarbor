# Environments: Troubleshooting

What goes wrong connecting an environment. Agent environments account for most of it
(reachability, certificates and a single-use install URL), and a local Docker Desktop
environment accounts for the rest, because its "host" is Docker Desktop's own Linux VM.

| Symptom | Usual cause | Fix |
|---|---|---|
| A new agent never connects | The agent cannot reach McHarbor, or will not trust its certificate | Check outbound reachability, then `MCHARBOR_FORCE_TLS` and whether the host trusts the certificate |
| "Waiting for agent…" forever | The install command was never run, or the one-time URL expired | The install URL is single-use and expires after 24 hours; regenerate it |
| The token is lost | It is shown once by design | Regenerate it from the detail page; the old one stops working |
| Metrics are missing on an agent environment | Agent metrics collection is turned off, or the agent cannot push and is too large to pull | Check the Overview's **Container stats** card; it names the reason and, for an old agent, the build that would push |
| File transfers fall back to the relay | The target agent’s transfer listener is unreachable | Run the agent-to-agent transfer test in Settings → System → Agent |
| A TLS environment refuses to connect | Wrong CA, or a certificate issued for another name | Test the same certificates with `docker --tlsverify` from the McHarbor host |
| The Logs tab is empty on an agent environment | Host logs need agent 1.8.0 or newer, and a connected agent | Upgrade from Settings → Updates → Agents, and check the agent is connected |
| The Logs tab shows no lines on a local Docker Desktop environment | On Windows and macOS the "host" is Docker Desktop's own Linux VM, not your machine. That VM has almost nothing in `/var/log` and no `journalctl` | Point McHarbor at a real Linux host through the agent; a Docker Desktop VM has no host logs to read |
| The Host terminal drops you somewhere unfamiliar | Same cause: on a local Docker Desktop environment the shell lands in the Docker Desktop VM | Use an agent environment for a shell on a real machine |
| Kernel packages stay listed after a successful apply | `linux-generic`, `linux-image-generic` and `linux-headers-generic` are metapackages, and a new kernel ABI needs image, module and header packages the host does not have. A plain upgrade never installs a new package, so apt reports them as **kept back** | Turn on **Install new packages when required** on the Updates tab and apply again. Nothing is removed by it |
| The kernel version is unchanged after applying a kernel update | The new kernel is installed but not running | Reboot the host. The tab shows **reboot required** once the host says so; McHarbor never reboots a host on its own |
| The new-packages setting saves but changes nothing on an agent host | The agent binary predates the setting and ignores it, upgrading conservatively as before | Update the agent from Settings → Updates → Agents. A native agent does not self-update across a build, only across a version |
| A host's disk reads "No disk data" or "Not monitored" | No usable reading: the agent is offline, or it cannot report host stats. "Not monitored" means McHarbor cannot read that disk at all | Bring the agent online or update it. The **Host disk data missing** alert names the cause in its `reason` |
| Backup archives take up space in the McHarbor data directory after a crash | An interrupted agent backup left a `.mcharbor-agent-backup-*` file in `backups/containers` or in one of its run folders | McHarbor removes such files older than ten minutes when it starts, and logs the count and bytes. A file younger than ten minutes is left alone, in case a transfer is still writing it |

## Reading the Logs tab's notices

The Logs tab reads whatever the host actually exposes, and tells you what it found.

A **grey** strip is information, not a problem:

- the list of log files discovered under the host's `/var/log`, so you can see what is
  there;
- a note naming the file that was read when none of the expected ones existed.

A **yellow** strip is a warning worth acting on:

| Notice | What it means |
|---|---|
| A host log file exists but McHarbor cannot read it | The file is there but permissions deny it; a fallback source was tried |
| No readable file log or journal source was found | Neither a log file nor `journalctl` is usable on this host |
| The host has no `/var/log` directory | Nothing file-based to read, usually a minimal image or a Docker Desktop VM |
| `journalctl` is not installed | The host is not systemd-based, or the binary is absent |
| `journalctl` returned an error | It ran and failed; anything shown below it may be incomplete |

## When a control is disabled

A disabled control with a tooltip naming a permission is RBAC, not a fault: the tooltip
names the exact key to ask for. A disabled control with no tooltip is usually state: the
action does not apply to the current status of the thing you selected.

## Related

- [Environments](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what each action really does
