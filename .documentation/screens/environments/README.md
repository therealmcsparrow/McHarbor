# Environments

`/environments` · `/environments/:id`

An environment is one Docker endpoint. Everything else in McHarbor (containers, stacks,
images, volumes, networks) is scoped to whichever environment is selected.

## Permissions

| Key | What it unlocks |
|---|---|
| `environments.view` | See environments and their detail |
| `environments.manage` | Add, change, remove, test, and regenerate agent tokens |

Everywhere except navigation, a control you lack the key for renders **disabled with a
tooltip naming it**, rather than disappearing. Navigation is the exception: a sidebar
entry you cannot use is hidden, because a dead-end page is not informative.

## What it shows

- One row per environment: name, platform, connection, endpoint, the host’s own IP address, version, status and type, with one carrying the **default** badge.
- A card view with live figures: CPU, RAM, container counts by state, how many containers need updates, and the host disk. A host disk McHarbor cannot read shows a grey "No disk data, last seen …" line rather than a bar, so an unmeasured disk never looks healthy. The table has a **Disk** column with the same status.
- A detail page with ten tabs in four groups: Overview, Settings, Host and Console.

## Connection types

A new environment is one of two types. The legacy types are still read, displayed and
offered on an environment that already uses one, so editing it does not silently retype it.

| Type | What it is | What it needs |
|---|---|---|
| **Socket** | The daemon on this host | A socket path, usually `/var/run/docker.sock` |
| **Agent** | The McHarbor agent on the remote host | A token; the agent dials out |

TCP, TLS, SSH and Podman environments created by an earlier release keep working. They are
no longer offered for a *new* environment, and plain **TCP** is why: it is an
unauthenticated Docker daemon on the network, and anything that can reach that port has root
on that host.

## Creating one, and getting the agent onto the host

**Nothing is created for you.** A fresh install has no environment at all, not even for the
Docker socket mounted into the container. The first one is made here, by hand, and which
host McHarbor governs stays your decision.

The dialog asks for a deploy method beside the connection type.

| Method | What happens |
|---|---|
| **Manual** | McHarbor mints an install token and hands you the install URLs. You run one on the host yourself |
| **Automatic (SSH)** | McHarbor connects to the host over SSH and installs the agent for you, as a container or as a binary under systemd |

Automatic takes an SSH key or a password, and either may be a `${secret:NAME}` reference
into the [secrets vault](../security/secrets.md) rather than a literal. The escalation is
detected rather than assumed: a host with `su` and no `sudo` is driven with `su`.

Either way the result carries **two install URLs**, one for the container install and one for
the binary install, so a manual environment is no worse off than an automatic one.

### The two host checks

Once the SSH host, user and host key fingerprint are filled in, the dialog offers two
buttons that ask the host what it already has. Both are read-only: every check is a
`command -v` or a file test, nothing is installed, and nothing is written to the host:

| Button | What it answers | What it runs |
|---|---|---|
| **Check Docker** | Is Docker installed on this host, and does it carry the Compose plugin | `docker`, `docker compose version` |
| **Check VM readiness** | Can this host run KVM virtual machines | CPU `vmx`/`svm` flag, `/dev/kvm` (present *and* openable), `qemu-system-x86_64`, `qemu-img`, OVMF, `swtpm` |

Each result lists every item with a pass or fail, the reason it failed, and the install
command for that host's own package manager where one is known. **Check VM readiness** marks
`/dev/kvm`, `qemu-system-*` and `qemu-img` as blocking, because a VM cannot start without
them; OVMF and swtpm are reported but do not block; they are UEFI boot and the vTPM Windows
11 wants.

Neither check gates the **Add** button. A Docker-less host is a legitimate agent host: the
agent still serves host stats, host logs and the host terminal. And VM readiness needs one
more thing no probe can see from here: the agent has to be installed as a **binary** under
systemd, because a containerised agent cannot host guests whatever the host has installed.

The full package list, per distribution, is in
[Host requirements for guests](../../compute-foundation/host-requirements.md).

The separate **Prerequisites / Check host** panel, which also *installs* what is missing, is
currently hidden pending a rework. The probe and its install endpoint still exist and
nothing about them changed; nothing is installed on your behalf while it is hidden.

An environment is **editable** after creation: its name, its connection type and every
connection field, from the detail page, gated on `environments.manage`.

## One host, one environment

A socket environment and an agent environment pointing at the same machine are two
environments for one host: it is polled twice and counted twice in every report. Deleting one
loses its history, so the way out is to **migrate** the one you are
keeping and **merge** the other into it. Both live on the Overview tab; see
[Actions](./actions.md).

McHarbor detects the pair itself, by comparing the Docker daemon id each environment
reports, and says so on both of their Overview tabs.

## The agent token is shown once

> Save this token now; it will not be shown again.

The dialog offers four ways to use it, one per tab: a one-line install script, a
`docker-compose.yml`, a `docker run` command, or the **Binary** tab, which is the native
install. The Compose tab is the one to prefer for a host you will keep as a container host:
it declares the `mcharbor-agent-data` volume that holds the agent's offline buffer and
instance id, which a plain `docker run` without it loses on every recreate.

The **Binary** tab carries three blocks, in the order they are run: a download that takes the
agent this McHarbor was built with, so the two versions match; the foreground command, for a
quick check that it connects; and a systemd unit written with the token already in it,
enabled and started in one paste. Prefer the service; the foreground command does not
survive a reboot. This is also the only install that can host virtual machines or system
containers, so it is the tab to use on a host that will run guests, even when that host has
Docker. See
[Installing an Agent → As a binary with systemd](../../agents/installation/README.md). **The install URL expires after 24 hours and can only be used once.** Lose
the token and you regenerate it from the detail page, which invalidates the old one.

A **self-signed certificate** toggle sits above the tabs and feeds every method at once:
the Compose file, the `docker run` command, the binary environment, and the install-script
URL. It is on by default, because a stock McHarbor signs its own certificate and an agent
that verifies it never completes the handshake. Turn it off when yours is signed by a CA the
remote host trusts.

One more hint that is easy to miss: file transfers are **pushed to**
`https://<agent-host>:8788`, so the agent needs to trust the McHarbor certificate for those
too.

## The detail tabs

| Group | Tabs | What is there |
|---|---|---|
| **Overview** | `overview` | Connection details, live host figures, agent status, and the **Container stats** card: whether this environment's stats are pushed by its agent, pulled, or read directly, and why ([Container stats](../../agents/stats/README.md)) |
| **Settings** | `activity`, `automation`, `retention` | Per-environment events, automation and retention |
| **Host** | `host`, `docker`, `processes`, `updates`, `services` | The machine, its daemon, what runs on it, its operating-system updates, and (on a native agent only) its systemd [services](./services/README.md). The **Host filesystem** panel shows the disk under the host. McHarbor reads it without asking Docker: directly for the local environment, through the agent for an agent host, so an agent host reports its own disk and not McHarbor's |
| **Console** | `terminal`, `logs` | A shell on the **host**, and the host’s own logs: a source (system, kernel, auth, Docker), a line count, **Newest first** / **Oldest first**, and **Live**, which re-reads the log every 5 seconds while it is on (it is off by default, because each read runs a command on the host) |

The **Updates** tab reports the host’s distribution, kernel and package manager, checks for
outstanding operating-system updates on a schedule you set, and applies them with the output
streamed line by line into the page. It works over the agent, which it did not before; both
the check and the apply used to refuse an agent environment outright. Reading the tab needs
`host_updates.view`; changing the schedule or starting an apply needs `host_updates.manage`. The
McHarbor host itself is not an environment and takes `host_self.view` / `host_self.manage`
instead (see [Host OS](../../settings/updates/host-os/README.md)).

### How far an upgrade is allowed to go

An upgrade installs newer versions of packages the host already has, and nothing else. That
is the safe default, and on a Debian or Ubuntu host it has one visible consequence: a kernel
update is reported as **kept back** and never applies. A new kernel ABI (6.8.0-139 to
6.8.0-142, say) needs *new* image, module and header packages, and a plain upgrade refuses
to install a package that was not there before. The tab lists the kernel as pending, the
apply reports success, and it is still pending afterwards.

**Install new packages when required**, on the Automatic check card, is the decision to
allow that. It is stored per environment, defaults to **off**, and is saved with the rest of
the schedule form under `host_updates.manage`. With it on, an apt host upgrades with
`--with-new-pkgs`: dependencies that are missing get installed, and, unlike a
`dist-upgrade`, nothing is ever removed to make room.

It changes apt only. `dnf`, `apk` and `pacman` already pull in new dependencies on an
upgrade, so the setting makes no difference on those hosts. `zypper` is the exception: it is
as conservative as apt, but its only way round that is `dist-upgrade`, which on a
non-rolling SUSE means a distribution upgrade, so the setting deliberately leaves it alone.

### When a package cannot be downloaded

**Update host** opens a confirmation that carries the three options for this run,
**Install new packages when required**, **Skip unavailable packages** and **Remove unused
packages**, preset from the
environment's saved defaults on the Automatic check card. Changing them in the dialog
affects that run only; the card is where the default is saved.

The package lists are always refreshed before an upgrade, so there is no separate refresh
switch. When the upgrade still fails on a download (a mirror that dropped a file between
the refresh and the fetch, which is what apt's *"Unable to fetch some archives, maybe run
apt-get update or try with --fix-missing?"* means), McHarbor refreshes the metadata once
more and retries the upgrade **once**, printing `McHarbor: package lists refreshed, retrying
once` into the output. It never retries twice, and never on a failure that is not a
download: a held dpkg lock, a dependency conflict or a full disk fails on the first run with
the package manager's own message.

| Manager | Retried on | Refresh before the retry | **Skip unavailable packages** |
|---|---|---|---|
| apt | `Unable to fetch some archives`, `E: Failed to fetch` | `apt-get update` | `--fix-missing` |
| dnf | `Failed to download`, `Cannot download`, `Error downloading packages`, `Curl error`, `Status code: 404` | `dnf clean metadata`, retry with `--refresh` | `--skip-broken --setopt=skip_if_unavailable=True` |
| yum | as dnf, plus `No more mirrors to try` | `yum clean metadata` | `--skip-broken` |
| zypper | `Download (curl) error`, `File … not found on medium` | `zypper refresh --force` | ignored |
| apk | `fetch … failed`, `ERROR: <pkg>: temporary error` / `network error` / `unable to …` | `apk update` | ignored |
| pacman | `failed retrieving file`, `failed to retrieve some files` | `pacman -Syy` | ignored |

zypper, apk and pacman have no switch that skips a missing package without risking a
half-applied transaction, so the option is deliberately ignored there. When a run still
fails on a download with the option off on apt, dnf or yum, the progress panel says so and
names **Skip unavailable packages** as the way through.

**Skip unavailable packages** is stored per environment (default **off**) with the rest of
the schedule form under `host_updates.manage`. Over an agent, both the option and the retry
need an agent built from `2026.09.01` build `202609271352` or later; an older agent still
upgrades but ignores both, and the tab says **update the agent**.

### Removing unused packages

**Remove unused packages** is off by default. When it is on, and the upgrade (including its
one retry) exited 0, McHarbor prints `McHarbor: removing unused packages` and then runs the
manager's own cleanup. A failed upgrade never triggers it, and a failing cleanup fails the
run with the cleanup's exit code, so a half-finished cleanup is never reported as success.

| Manager | Cleanup |
|---|---|
| apt | `apt-get -y autoremove`, which also drops old kernels that were installed as dependencies |
| dnf | `dnf -y autoremove` |
| yum | `yum -y autoremove` |
| pacman | `pacman -Rns` on the orphans `pacman -Qdtq` lists, and nothing when there are none |
| zypper, apk | ignored; the output says so |

It is stored per environment with the rest of the schedule form under `host_updates.manage`.
Over an agent it needs an agent built after the option shipped; an older agent upgrades but
ignores it.

A kernel that has just been installed is not the kernel that is running. The tab reports
**reboot required** once the host says so, and rebooting stays a thing you do yourself.

The Overview tab’s **agent log** card carries the runtime log’s own actions: refresh, live,
newest or oldest first, and export.

The host terminal is a different thing from a container’s: it is a shell on the machine,
through the agent. It is the most privileged surface in the product, and it exists because
diagnosing why Docker will not start cannot be done from inside a container.

## In this folder

| Page | What it covers |
|---|---|
| [Actions](./actions.md) | Everything you can do here, what gates it, and what it really does |
| [Fields](./fields.md) | Every column, filter and form field the screen exposes |
| [Troubleshooting](./troubleshooting.md) | Symptom, cause and fix |
| [Services](./services/README.md) | The **Services** tab: a native agent host's systemd units, the drop-in override editor, the protected units |

## Related

- [Docker](../docker/README.md): daemon info for the selected environment
- [Settings → System → Agent](../../settings/system/agent/README.md): transport defaults and diagnostics
- [Settings → Updates → Agents](../../settings/updates/agents/README.md): keeping agent versions current
