# Native Linux install

McHarbor also installs directly on a Linux host, with no container runtime involved: a
`.deb`, an `.rpm`, or a tarball with its own `install.sh`, running under
systemd as root by default, or as a sandboxed `mcharbor` service account with
`--no-run-as-root` ([section 7](#7-the-security-statement)).

**The container remains the recommended default.** Reach for native when the host's only
job is being a Docker control plane, when policy forbids privileged containers or
bind-mounting the Docker socket, or on a control-plane-only install that only manages
remote agents and never runs a local Docker daemon at all, where a container runtime there is
pure overhead.

## 1. Supported distributions: read this before downloading

**Supported**, glibc 2.36 or newer: Debian 12+, Ubuntu 24.04+, Fedora 38+.

**Not supported, and it fails at service start rather than at install**: Ubuntu 22.04
(glibc 2.35), Ubuntu 20.04 and Debian 11 (glibc 2.31), and Rocky/RHEL/Alma 9 (glibc 2.34)
all fail with:

```
GLIBC_2.36 not found
```

The reason is structural, not a packaging oversight: the 61 bundled plugins are Go shared
objects (`cgo`), so the binary links dynamically against the libc that built it, and glibc
is backward-compatible but never forward-compatible: a binary built against 2.36 will not
load on an older 2.34 or 2.31. **This is not softened for RHEL 9**: an `.rpm` that will not
run on RHEL 9 effectively serves Fedora only, and that is worth knowing before you download
rather than after a failed `systemctl start`.

Check before installing:

```bash
ldd --version | head -1
```

Every McHarbor artifact links glibc, the container included, so one build of each
plugin serves every install type. Alpine is not a supported native target.

## 2. Install, per format

All three artifacts self-report the same version. They carry the `mcharbor` binary, the
frontend bundle, the bundled agent and the theme pack, but no plugins; those are
downloaded from the Plugin Store (see [Plugins on a native install](#plugins-on-a-native-install)),
so the tarball is around 65 MB.

```bash
# Debian / Ubuntu
sudo dpkg -i mcharbor_2026.09.01_amd64.deb

# Fedora
sudo rpm -i mcharbor-2026.09.01-1.x86_64.rpm

# Any supported distribution, no package manager
tar -xzf mcharbor-2026.09.01-linux-amd64.tar.gz
sudo ./install.sh
```

Each path installs the payload, creates the `mcharbor` service account (used with
`--no-run-as-root`, and the owner of `/var/lib/mcharbor`), writes the environment file,
and installs and enables the systemd unit.

On a fresh install the tarball's `install.sh` and the `install-debian.sh` /
`install-fedora.sh` wrappers then read the one-time setup token from the journal and print
it at the end of their output:

```
Setup token: 0123456789abcdef0123456789abcdef
```

Paste it into the **Setup token** field of the first-run installer
([First run](../first-run/README.md#the-setup-token)). It changes every time the service
restarts; read the current one with `journalctl -u mcharbor | grep 'setup token' | tail -n 1`.
No token is printed when the instance already has an account. A bare `dpkg -i` or
`rpm -i` prints nothing beyond the package manager's output, so read it from the journal
there.

## 3. Filesystem layout

| Path | Holds | Survives an upgrade? |
|---|---|---|
| `/opt/mcharbor` | The payload: the binary, the frontend bundle, the 61 bundled plugin `.so` files | No; replaced wholesale by the package |
| `/var/lib/mcharbor` | The database, TLS material, the session secret, installed plugin state | Always; the package never touches it |
| `/etc/mcharbor/mcharbor.env` | Environment variables | Always. It is a conffile; your edits survive |
| `/etc/mcharbor/encryption-key.env` | `ENCRYPTION_KEY`, the data encryption key, root-only (0600) | Always; the installer writes it once and never replaces it |
| `/usr/lib/systemd/system/mcharbor.service` | The unit | Replaced with the package's version on upgrade |

This mirrors the container's `/app/data` split, with one difference worth internalising:
on the container, everything under `/app/data` is one volume and one backup target. Here
it is two directories, and only `/var/lib/mcharbor` needs to be backed up; `/opt/mcharbor`
is reproducible from the package.

## 4. Configuration

`/etc/mcharbor/mcharbor.env` carries the same keys as the container's environment
variables; see [Configuration](../configuration/README.md) for the full table. One key is
native-only and set for you:

`MCHARBOR_INSTALL_KIND` is written automatically by the postinstall script (`deb`, `rpm`
or `tarball`) and should not be hand-edited. It is how the running binary tells a native
install apart from a container one in diagnostics and update logic. The shipped tarball's
sample env file shows `tarball` as this value only because `install.sh` has not run yet to
overwrite it; an installed system always shows the format it was actually installed from.

`ENCRYPTION_KEY`, the key every stored secret is encrypted with, is pinned by the installer in
`/etc/mcharbor/encryption-key.env`. On an existing install it is the key McHarbor already
generated into `/var/lib/mcharbor/.encryption_key`, base64-encoded, so nothing is re-encrypted;
on a fresh install it is a new random key. It is a separate file because `mcharbor.env` is a
conffile, and is skipped when `mcharbor.env` already sets `ENCRYPTION_KEY` or
`ENCRYPTION_KEY_FILE`. Keep a copy of it somewhere other than this host: without the key, the
stored secrets cannot be decrypted. Re-running `install.sh` on an older native install is what
clears the "data encryption key was generated into DATA_DIR/.encryption_key" warning.

## 5. Upgrading

A normal package-manager upgrade: `apt upgrade`, `dnf upgrade`, or a fresh
`install.sh` run over the top. **Settings → Updates → McHarbor** still checks for new
releases, but offers no **Update now** on a native install, and automatic install has no
effect: self-update replaces a container, and there is none. Data, installed plugins, and edits to `mcharbor.env` are all
preserved, because none of them live under `/opt/mcharbor`.

Bundled plugins are **re-synced to the new binary automatically on the next start**. This is the
same ABI-hash problem the container solves in its entrypoint (see
[Upgrading → What happens to plugins](../upgrading/README.md#what-happens-to-plugins))
applies here identically, and the native binary carries the equivalent logic itself rather
than relying on a container entrypoint script.

## 6. Uninstalling, and the rpm asymmetry

| Command | Removes `/opt/mcharbor` | Removes `/var/lib/mcharbor`, `/etc/mcharbor`, the `mcharbor` user |
|---|---|---|
| `apt remove mcharbor` | Yes | No |
| `apt purge mcharbor` | Yes | Yes |
| `./install.sh --uninstall` | Yes | No |
| `./install.sh --uninstall --purge` | Yes | Yes |
| `rpm -e mcharbor` | Yes | **No, always** |

**`rpm -e` has no purge equivalent to reach for.** RPM carries no purge concept at all, so
`/var/lib/mcharbor`, `/etc/mcharbor` and the `mcharbor` user survive every `rpm -e`
unconditionally, and removing them is a manual step:

```bash
sudo rm -rf /var/lib/mcharbor /etc/mcharbor
sudo userdel mcharbor
```

This is deliberate on Fedora as much as everywhere else (losing a database silently to an
uninstall is worse than leaving a directory behind), but it only works as a design choice
if it is written down here, rather than discovered by an operator wondering where their
data went.

## 7. The security statement

**By default the service runs as root.** The unit sets `User=root` with `NoNewPrivileges`
and `PrivateTmp`, and no filesystem sandbox, so any local storage path works as it is.

Whichever account runs it, McHarbor controls the Docker daemon, and **on any host running a
Docker daemon, that control is root-equivalent**: whoever can reach the socket can start a
privileged container that bind-mounts the host filesystem, regardless of what McHarbor's
own RBAC allows. The `mcharbor` service account is added to the **`docker` group** for the
same reason, so it can reach `/var/run/docker.sock` when the service runs as that account.

### Running as the mcharbor account

`--no-run-as-root` runs the service as `mcharbor` inside a filesystem sandbox instead:

```bash
sudo ./install.sh --no-run-as-root                       # tarball
sudo ./install-debian.sh --no-run-as-root                # or install-fedora.sh / install-tarball.sh
sudo MCHARBOR_RUN_AS_ROOT=0 apt install ./mcharbor_*.deb  # a package installed directly
```

It writes `/etc/systemd/system/mcharbor.service.d/run-as-mcharbor.conf` (`User=mcharbor`,
`ProtectSystem=strict`, `ProtectHome=true`, `ReadWritePaths=/var/lib/mcharbor`). A later
install or upgrade keeps that choice and says so; `--run-as-root` (or
`MCHARBOR_RUN_AS_ROOT=1`) removes the drop-in and returns the service to root. Uninstalling
removes it.

| | As root (default) | As `mcharbor` (`--no-run-as-root`) |
|---|---|---|
| What McHarbor can reach through Docker | Everything | Everything |
| A local storage path outside `/var/lib/mcharbor` | Works as is | Needs a `ReadWritePaths=` drop-in and `mcharbor` ownership ([section 8](#8-troubleshooting), item 3) |
| What a bug in McHarbor or a plugin can overwrite directly | Any file on the host | `/var/lib/mcharbor` only |
| Files McHarbor writes into a bind-mounted path | Owned by root | Owned by `mcharbor` |

The sandbox limits what a bug **inside** McHarbor can reach on the rest of the filesystem.
It is not, and is not intended to be, a boundary against a compromise of the McHarbor
process itself: that process already holds a root-equivalent credential by design, the same
way the containerized install's Docker socket mount does. Note also what the unit does **not** do: it does not set
`SupplementaryGroups=`. That directive was tried and removed because it crashed the
service outright on a host with no `docker` group at all, and it is unnecessary besides,
because systemd already reads the account's group memberships from `/etc/group` without
it.

## 8. Troubleshooting

Lead with the two failures most likely to occur, in order of likelihood:

1. **The service will not start, and `journalctl -u mcharbor` shows `GLIBC_2.36 not
   found`.** The distribution is below the glibc floor in [section 1](#1-supported-distributions-read-this-before-downloading):
   Ubuntu 22.04/20.04, Debian 11, or Rocky/RHEL/Alma 9. There is no workaround short of
   a newer distribution or the container image instead.
2. **Plugins fail to load after an upgrade.** Look for `refreshed bundled plugin` lines in
   `journalctl -u mcharbor`, each carrying a `slug=` and a `payload=` attribute, one line
   per bundled plugin the new binary re-synced. Their absence for a plugin that used to
   work means the re-sync did not run for it; reinstalling it from the App Store against
   the new version recovers it, the same fallback the container documents in
   [Upgrading → What happens to plugins](../upgrading/README.md#what-happens-to-plugins).
3. **A local storage location fails its test with `read-only file system`** on an install
   made with `--no-run-as-root`, for example
   `mkdir /mcharbor/backup/mcharbor-conn-test-…: read-only file system`, while `findmnt`
   shows the disk as `rw`. The unit's `ProtectSystem=strict` makes the whole filesystem
   read-only to McHarbor except `/var/lib/mcharbor`, so any other path a local storage
   location writes to has to be opened up. `findmnt` run from a shell shows the host's view,
   not the service's. Two steps, both needed:

   ```bash
   # 1. Let the service write there (a drop-in survives package upgrades)
   sudo mkdir -p /etc/systemd/system/mcharbor.service.d
   printf '[Service]\nReadWritePaths=/mcharbor/backup\n' \
     | sudo tee /etc/systemd/system/mcharbor.service.d/storage.conf
   sudo systemctl daemon-reload && sudo systemctl restart mcharbor

   # 2. Give the mcharbor user write access
   sudo chown -R mcharbor:mcharbor /mcharbor/backup
   ```

   List several paths separated by spaces on the one `ReadWritePaths=` line. If you use
   `sudo systemctl edit mcharbor` instead, put the `[Service]` block **above** the
   `### Lines below this comment will be discarded` line, or the edit is dropped silently.
   To check each step:

   | Check | Command | Expect |
   |---|---|---|
   | The drop-in is active | `systemctl show mcharbor -p ReadWritePaths` | The path is listed |
   | The service sees it writable | `sudo nsenter -t "$(systemctl show -p MainPID --value mcharbor)" -m findmnt -T /mcharbor/backup` | `rw` in the options |
   | The account may write | `sudo -u mcharbor touch /mcharbor/backup/probe` | No error; every parent directory needs execute for `mcharbor` (`namei -l` shows each level) |

   A native agent runs as root without this sandbox and needs neither step, and neither does
   McHarbor in its default, root mode. `--run-as-root` is the other way out
   ([section 7](#running-as-the-mcharbor-account)).

4. **McHarbor, the agent or containerd keep crashing, and containers stop every few
   minutes,** on a CPU with AVX-512 (AMD Zen 4 such as the Ryzen 7 8745HS, recent Intel
   server parts). `journalctl -u mcharbor -u mcharbor-agent -u containerd` shows
   `fatal error: found pointer to free object` or `fatal error: found bad pointer in Go
   heap`, and no other program crashes. Go's garbage collector scans the heap with an
   AVX-512 routine that corrupts it on these CPUs. When containerd dies, Docker restarts
   and stops every container, which looks like the containers being killed.

   McHarbor's own unit and the agent's unit start with
   `GODEBUG=cpu.avx512f=off,cpu.avx512vl=off`, which turns that routine off. On a CPU
   that lists `avx512f` in `/proc/cpuinfo`, the installers also write
   `mcharbor-go-avx512.conf` drop-ins for `containerd` and `docker`. They do not restart
   Docker, because that stops every container; the drop-ins apply at Docker's next
   restart, so restart it yourself when a short container outage suits you:

   ```bash
   sudo systemctl restart containerd docker
   ```

   To check it reached each running process:

   ```bash
   for u in containerd docker mcharbor mcharbor-agent; do
     pid=$(systemctl show -p MainPID --value "$u")
     printf '%s: ' "$u"; sudo tr '\0' '\n' < /proc/$pid/environ | grep GODEBUG || echo missing
   done
   ```

   An install from before this change has neither; add the same `[Service]` drop-in by
   hand to each of the four units, or reinstall. A `GODEBUG` you set yourself in
   `/etc/mcharbor/mcharbor.env` overrides McHarbor's.

Beyond those four:

- **Logs**: `journalctl -u mcharbor -f`.
- **Service state**: `systemctl status mcharbor`.
- **Data**: `/var/lib/mcharbor`, same contents as the container's `/app/data`.
- **Health**: `curl -fk https://localhost:8705/api/health` from the host itself.

## Related

- [Installation](./README.md): the container path, still the recommended default
- [Configuration](../configuration/README.md): every environment variable
- [Prerequisites](../prerequisites/README.md): the `docker`-group statement in context
  with the container's own Docker-socket equivalent
- [Upgrading](../upgrading/README.md): the plugin ABI-hash problem this format's
  auto-resync also solves

## Plugins on a native install

Plugins do not ship inside the package. The payload under `/opt/mcharbor` carries the
binary, the static frontend, the bundled agent and the theme pack, with no `.so` and no
plugin tarballs.

Everything in the Plugin Store is downloaded from
`https://www.mcharbor.eu/catalog/plugins`, so a native install needs outbound HTTPS to
that host the first time it installs or repairs a plugin. An air-gapped host can still
install one by uploading its tarball directly in the Plugin Store.

Because every artifact links glibc, the tarball a native install downloads is the same
one the container uses.

### After an upgrade

A McHarbor upgrade changes the ABI every plugin `.so` was compiled against, so on the
first boot afterwards McHarbor re-downloads each installed plugin at the version you
already had. Plugins it cannot fetch are left disabled and listed in the Plugin Store as
**Needs repair**, naming the release they were built for; **Repair** retries. Requires
`plugins.manage`. The upgrade itself never fails because of this.
