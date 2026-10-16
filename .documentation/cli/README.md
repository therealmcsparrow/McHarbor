# Command-line client

`mcharbor` operates McHarbor from a terminal or a CI job. It is an API client, not a side
channel: it authenticates with an API key, every request passes the same permission checks
as the web interface, and every change it makes is written to the audit log exactly as the
same change made in the interface would be.

| Page | Covers |
|---|---|
| [Scripting and CI](./scripting.md) | Exit codes, output formats, machine-readable errors, confirmation rules, long-running operations |
| [Troubleshooting](./troubleshooting.md) | Certificates, 401 and 403, version skew, `mcharbor doctor` |
| [Command reference](./reference/mcharbor.md) | Every command and flag, generated from the binary |

## Install

| Where | How |
|---|---|
| From your McHarbor | **Security → API keys → Command-line client** lists the archives this server was built with (`GET /api/cli/downloads`), downloads one (`GET /api/cli/download/{os}/{arch}`), and shows the `mcharbor login` command for this server with a copy button. **Server uses a self-signed certificate** adds `--insecure` to that command; `login` saves it with the sign-in, so later commands keep skipping verification. Installing the root CA or passing `--ca-file` is the safer choice. Any signed-in user may download; no permission key is needed |
| Linux, macOS | Unpack `mcharbor_<version>_<os>_<arch>.tar.gz` and put `mcharbor` on your `PATH`. `completions/` and `man/` are in the archive |
| Windows | Unpack `mcharbor_<version>_windows_amd64.zip`; `completions/mcharbor.ps1` is the PowerShell completion |
| Inside the McHarbor container | Already there: `docker exec -it <container> mcharbor …` |
| On an agent host | The agent binary carries the CLI: `mcharbor-agent cli …`, or `mcharbor …` through the link the install script makes (it never replaces a separately installed `mcharbor`). In the agent container, `docker exec -it mcharbor-agent mcharbor …`. The agent's own `MCHARBOR_URL` is used as the server, so only an API key is needed; the agent token is not one |
| CI | The CLI image, built from `docker/Dockerfile.cli`, has `mcharbor`, `jq` and bash completion, and runs as an unprivileged user |

Archives are built by `scripts/development/build-cli.sh` for linux/amd64, linux/arm64,
darwin/amd64, darwin/arm64 and windows/amd64, with a `mcharbor_<version>_checksums.txt` file (not `SHA256SUMS`, which the server packages in the same release already use). The binary carries
the same version and build stamp as the server it was built beside. The McHarbor image builds the same five archives and serves them; a local `cli/` folder, mounted as `/app/cli-local` by `docker-compose.yml` (`MCHARBOR_LOCAL_CLI_PATH`), takes precedence, as `agents/` does for agent binaries.

## First use

Create an API key under **Security → API keys** with only the permissions the CLI needs,
then:

```bash
mcharbor login https://mcharbor.example:8705 --ca-file mcharbor-root.pem
mcharbor whoami --permissions
mcharbor env list
mcharbor env use <environment-id>
mcharbor container list
```

`login` prompts for the key without echoing it, checks it against the server, and saves the
server as a **context**. The key is stored in the OS keyring (Windows Credential Manager,
macOS Keychain, the Secret Service on Linux desktops) or, where there is none (a headless
server, the normal case), in `credentials.yaml` beside the config, readable by the owner
only. Neither is a degraded mode, and the CLI never warns about the second.

### Signing in without pasting a key

```bash
mcharbor login https://mcharbor.example:8705 --device
```

prints a link and a code such as `BCDF-GHJK`, and opens the link in your browser (`--no-browser`
only prints it; on Linux without a display it only prints). In a browser where you are
signed in to McHarbor: the **Approve a command-line sign-in** page (`/device`) shows the
computer and the OS user that asked, and lets you name the key, set its expiry and choose its
roles and permissions, never more than your own. Approving needs `api_keys.manage`, because
it creates an API key. The terminal receives the key once, on its next poll, and stores it
exactly as a pasted key is stored.

| Situation | What `login --device` does | Exit code |
|---|---|---|
| Waiting | Polls every 10 seconds; a `429` or `slow_down` lengthens the wait by 5 seconds | — |
| Denied in the browser | Stops | `4` |
| Nobody approved within 10 minutes | Stops; run it again | `8` |
| The code was already used or never existed | Stops | `3` |
| Ctrl-C | Stops; nothing is created | `1` |

The device code itself never leaves the terminal and the server stores only its hash; the
minted key's plaintext exists only in the one response that delivers it.

McHarbor's default certificate is signed by its own local authority. Download the root
under **Security → HTTPS** and pass it with `--ca-file`; `--insecure` also works and is for
labs only.

## Contexts

A context is one server: its URL, a default environment, and its certificate settings.
The config lives at `<user config dir>/mcharbor/config.yaml` (`MCHARBOR_CONFIG_DIR` moves it)
and never contains a key.

| Command | Does |
|---|---|
| `mcharbor context list` | Every context, the current one marked, and where its key is stored |
| `mcharbor context use <name>` | Switch |
| `mcharbor context set <name> --url … --env … --ca-file …` | Create or change one |
| `mcharbor context rename <old> <new>` | Rename, moving the stored key |
| `mcharbor context delete <name>` | Delete, including its key |
| `mcharbor logout [context]` | Forget the key, keep the context |

Every setting can come from a flag, an environment variable, or the context, in that order:

| Flag | Variable | Meaning |
|---|---|---|
| `--context` | `MCHARBOR_CONTEXT` | Context to use |
| `--url` | `MCHARBOR_URL` | Server URL |
| — | `MCHARBOR_API_KEY` | API key; always wins over a stored one |
| `--env` | `MCHARBOR_ENV` | Environment id for container, image, volume, network, stack and backup commands |
| `--ca-file` | `MCHARBOR_CA_FILE` | PEM root to trust |
| `--insecure` | `MCHARBOR_INSECURE=true` | Skip certificate verification |
| — | `MCHARBOR_NO_KEYRING=1` | Never touch the OS keyring |
| `--timeout` | — | Per-request limit, default `60s`; `0` waits forever |

In CI, set `MCHARBOR_URL` and `MCHARBOR_API_KEY` and skip `login` entirely.

## Commands

| Group | Commands | Permission keys |
|---|---|---|
| Server | `version`, `status`, `doctor`, `whoami [--permissions]`, `cluster status` | none beyond a valid key |
| High availability | `cluster readiness [--target]`, `cluster agent-routing` | `cluster.view` |
| High availability | `cluster failover <node>`, `cluster drain <node>`, `cluster resume <node>`, `cluster routing-check` | `cluster.manage` |
| Environments | `env list/get/use/test/delete`, `agent list` | `environments.view`, `environments.manage` |
| Agent host checks | `agent probe <host>`: connects over SSH and lists the prerequisites the host has and lacks, changing nothing; `agent install-prereqs <host> --prereq <key>...`: installs exactly the named prerequisites and re-probes, deploying no agent and minting no token (disruptive, asks unless `--yes`). Both take `--user`, `--fingerprint` (required), `--port`, `--method docker|binary`, `--scope docker|vm` and exactly one of `--key-file`, `--secret-id` (a vault secret) or `--password-stdin`; `--root-password-stdin` supplies the `su` password. Secrets are never flag values | `environments.manage` |
| Dashboard widgets | `widgets catalog`: the downloadable widget catalog with signature and install state; `widgets install <key>` downloads and installs a signed widget; `widgets remove <key>` uninstalls a downloaded widget (irreversible, needs `--confirm <key>`, built-ins are refused) | `widgets.view` or `store_widgets.view` (`catalog`), `dashboard_widgets.install` (`install`, `remove`) |
| Agent releases | `agent releases list`, `agent releases check`, `agent releases pull <version> [--build <yyyyMMddHHmm>]`, `agent releases target <version> [--build <yyyyMMddHHmm>]` / `--clear`, `agent releases delete <version> [--build <yyyyMMddHHmm>]`: McHarbor's own store of verified agent releases, which every install and update is served from ([Agents → Releases](../agents/releases/README.md)). `list` shows the refusal reason of a release that failed verification in its `ERROR` column. `pull` starts the download and returns; the release reads `pulling` until stored or refused, and a verified download becomes the target while every build stored before it stays. `--build` fetches that build instead of the newest (`check` lists the last five per version); a build older than the newest is pinned, so checks leave it in place until another build is pulled. `target --build` picks a stored build (without it, the newest stored build of the version); `delete --build` is required when the version holds more than one build. `target` is disruptive (it changes what installs and sweeps hand out, but restarts nothing) and asks unless `--yes`; `delete` is irreversible, needs `--confirm <version>`, and is refused for the target and for a version a connected agent runs | `updates.view` (`list`), `updates.manage` (the rest) |
| Host services | `host service list`, `host service cat <unit>`: the systemd `.service` units of a native agent host, and a unit's effective configuration plus its McHarbor override ([Services](../screens/environments/services/README.md)). `--env` is required; `list` fails with exit 6 and the reason when the environment cannot be managed | `host_services.view` |
| Host services | `host service start/enable <unit>`, `host service stop/restart/disable <unit>` (disruptive, ask unless `--yes`), `host service edit <unit> --file <path>\|- [--no-reload] [--verify]` (disruptive; writes `/etc/systemd/system/<unit>.d/override.conf` atomically and reloads), `host service revert <unit>` (irreversible, needs `--confirm <unit>`), `host service daemon-reload` (disruptive). Stop and disable of `mcharbor-agent*`, ssh/sshd, `systemd-*`, `dbus*`, docker and containerd are refused with exit 6 | `host_services.manage` |
| Host devices | `host devices`: the character and block device nodes under the host's `/dev` that a container can map, with kind, major/minor, udev label, the stable `by-id`/`by-path` name to use with `--device`, and block devices marked dangerous ([Devices](../screens/containers/devices/README.md)). `--env` is required; a truncated listing is flagged on stderr | `host.view` |
| Host updates | `host updates check`: runs the package check now and lists what can be upgraded; `host updates status`: the last check and the stored schedule, running nothing; `host updates apply [--with-new-packages] [--skip-unavailable] [--remove-unused]`: starts the upgrade and prints the job id (disruptive, asks unless `--yes`; an option flag you leave out keeps the stored default, `--remove-unused=false` overrides it off); `host updates schedule [--enable\|--disable] [--time HH:MM] [--days mon,fri,…] [option flags]`: reads the stored schedule and writes it back with only the flags you pass changed, and refuses with no flag. Name the host with `--env`, or pass `--self` for the machine McHarbor itself runs on (not both) | `host_updates.view` for `check` and `status`, `host_updates.manage` for `apply` and `schedule`; with `--self`, the global `host_self.view` and `host_self.manage` instead |
| Containers | `container list/get/start/stop/restart/pause/unpause/kill/remove/logs/exec` | `containers.view`, `containers.manage`, `containers.delete`, `logs.view` for `logs`; `exec -- <command>` needs `containers.manage`, an interactive `exec` needs `terminal.access` or `containers.manage`. `get` shows credential-looking environment values as `••••••••`; `get --reveal` shows them, needs `containers.manage` and is audit-logged, and never reveals a McHarbor container's own keys |
| Containers (distroless) | `container exec <id> --debug`, `container exec <id> --debug -- <command>` | `containers.debug_shell`; the `-- <command>` form also needs `containers.manage`. Uploads a temporary static busybox to `/.mcharbor-debug`, prints its digest, and removes it when the session or command ends; see [Debug shell](../screens/containers/debug-shell/README.md) |
| Images | `image list/get/pull/remove`; `image get --reveal` shows masked env values | `images.view`, `images.manage` (and for `--reveal`), `images.delete` |
| Volumes | `volume list/get/create/remove` | `volumes.view`, `volumes.manage`, `volumes.delete` |
| Networks | `network list/get/create/remove`; `network create` takes `--ipv6`, `--opt key=value` (such as `parent=eth0`), `--subnet` (repeatable), `--gateway` and `--ip-range` (both for the first subnet), and for a macvlan network `--host-link-name`, `--host-link-ipv4` and `--host-link-ipv6` (a CIDR, or `auto` for SLAAC), which also create a host interface on the parent so the host can reach the containers; `network host-link get <id|name>` shows whether that interface exists on the host and `network host-link apply <id|name>` re-creates it from the network's recorded settings. `network remove` removes the host interface first | `networks.view` (`host-link get`), `networks.manage` (`create`, `host-link apply`), `networks.delete` (`remove`) |
| Stacks | `stack list/get/compose/up/stop/restart/down/remove` | `stacks.view`, `stacks.manage`, `stacks.delete`. `compose` prints credential-looking `environment:` values as `••••••••`; `compose --reveal` shows them, needs `stacks.manage` on the stack and is audit-logged |
| Stack environment | `stack env <name> [--reveal]`, `stack env set <name> KEY=VALUE...`, `stack env unset <name> KEY...`: the variables stored for a stack, as a `KEY`/`VALUE` table (`-o json` and `-o yaml` give one object). Credential-looking values print as `••••••••`; `--reveal` sends `reveal=true` and shows them only when the server allows it (it needs `stacks.manage` on the stack, is audit-logged as `reveal_env_vars`, and values carrying McHarbor's own keys stay masked). `set` is Reversible and runs without asking; `unset` is Disruptive (the value is deleted and cannot be read back), so it prompts on a terminal and `--yes` skips the prompt, and unsetting a variable the stack does not have exits 5. Both read the masked view and write the whole set back, where the server keeps the stored value for every key still masked, so credentials you never saw survive; two people changing one stack at once can overwrite each other. Values are stored, not deployed: run `stack up` to apply them. `set` refuses a name Compose would not accept and the `••••••••` placeholder as a value. `--env <id>` scopes all three | `stacks.view` for `stack env`; `stacks.manage` for `--reveal`, `set` and `unset` |
| Backups | `backup plan list/run`, `backup run list/get/cancel`, `backup restore` | `backups.view`, `backups.manage`, `restore.run` |
| GitOps | `gitops pipeline list/get/promote/history`, `gitops pipeline promote --wait` (follows the background deploy and exits 7 with the recorded reason when it fails), `gitops approval list/approve/reject` | `gitops.view`, `gitops.manage`, `gitops.approve` |
| GitOps previews | `gitops preview list`, `gitops preview destroy <id>` | `gitops.view`, `gitops.delete` |
| GitOps webhook secret | `gitops webhook-secret show <repo-id>`, `gitops webhook-secret rotate <repo-id>` (prints the secret once on stdout), `gitops webhook-secret remove <repo-id>` (irreversible, needs `--confirm`) | `gitops.view`, `gitops.manage` |
| Users | `user list/get/create/disable/enable/unlock/delete/roles/assign-role/unassign-role`: `create` reads the password without echo or from `--password-stdin` | `users.view`, `users.manage`, `account.manage` |
| Groups | `group list/get/create/add-member/remove-member/delete` | `groups.view`, `groups.manage` |
| Roles | `role list/get/create/delete`, `role permissions` (the whole catalog) | `roles.view`, `roles.manage`, `permissions.view` |
| API keys | `api-key list/get/create/revoke`: `create` prints the key once on stdout, never again | `api_keys.view`, `api_keys.manage` |
| Elevation | `elevation list [--show mine/pending/all]/get/request/approve/deny/withdraw` | your own requests: none; deciding: `users.manage` |
| Policies | `policy list/get/test/enable/disable/delete`: `test` is read-only and meant for CI | `rbac.policies.view`, `rbac.policies.manage` |
| Audit | `audit list/search/export/verify`: filter with `--action` (repeatable), `--entity-type`, `--entity-id`, `--search [--exact]`, `--since 7d` or `--from/--to`; `export` takes the same filters, so it exports exactly what the search showed | `audit.view`; `audit.export` for `export` |
| Identity | `identity provider list/get/test/delete`, `scim list/get/sync/delete` | `identity_providers.view`, `identity_providers.manage`, `settings.manage`, `scim.view`, `scim.manage` |
| Plugins and apps | `plugin list/catalog/get/status/install/update/uninstall/reconcile`, `app list/get/install/uninstall`, `catalog list/get`: `install` takes the catalog entry (`--version` to pin) and the server verifies its signature; a release no trusted key verifies is refused with `err.plugin.unsigned_confirmation_required` unless `--allow-unsigned` is passed (also on `update`); install and update show progress on stderr; `uninstall --purge-data` also deletes the plugin's settings and needs `--confirm <slug>`; `plugin reconcile <slug> --version V [--disabled] [--allow-unsigned]` pins the plugin to that exact catalog version (installs it when absent, reinstalls it when another version is installed, keeps it when it matches) and sets whether it is enabled, asking unless `--yes`; `app install <slug> [--name N] [--env-var K=V ...] [--secret-file-mode]` installs the catalog app as a stack in the `--env` environment over the streaming install route, showing progress on stderr and asking unless `--yes` (`--env-var` repeats, `--secret-file-mode` delivers `${secret:NAME}` values as files under `/run/secrets`); `app uninstall <installation-id>` removes the installation and its stack, is irreversible, and needs `--confirm <installation-id>` | `plugins.view`, `plugins.manage`, `plugins.delete`, `store_apps.view`, `store_apps.manage` for `app install/uninstall`, `settings.view` |
| Plugin authoring | `plugin dev new/validate/build/package/sign`: the author's local toolchain, described in [Plugin authoring from the CLI](../development/plugins/authoring-cli/README.md). `validate` reports route drift, a CRLF `go.mod`, a dependency version that disagrees with the host's, a permission key the installer will reject, a `Version` symbol that has drifted from the manifest and a stale UMD bundle; it exits `1` on an error and `0` when the worst is a warning. `build` and `package` drive `scripts/development/catalog.sh` and `build-plugin.sh`; `--dry-run` prints the command instead of running it | **none; local only.** These commands read and write your checkout and drive the build scripts and the signer. They reach no server, so there is no API key, no context and no permission key involved, and nothing they do is audited |
| Interactive screen | `tui`: the container list for the current environment, refreshed while it is open: `j`/`k` move, `/` filters, `s` starts, `t` stops, `r` restarts, `d` removes, `R` refreshes, `q` quits. The confirmation rules are the documented ones, not softer ones: a stop or a restart asks `y/N` and a remove makes you type the container's name back, so `--yes` still does not cover a remove. Needs a terminal | `containers.view`, plus `containers.manage` and `containers.delete` for the actions |
| Capacity | `capacity list`, `capacity idle [--days N]`: `list` gives the projected exhaustion date per filesystem and storage destination, or the refusal reason (`short_series`, `noisy`, `not_rising`, `already_over`, `beyond_horizon`) when there is no date. `idle` lists containers stopped, or running with no CPU and no traffic, for longer than `--days` (default 14, max 365), with what each one holds; `SIZE KNOWN false` means the size was never sampled (agent environments do not collect writable-layer sizes) and is not a zero. Both are read-only, so neither asks for confirmation; reclaim with `mcharbor docker prune` | `host.view` |
| Compliance packs | `compliance packs`, `compliance runs <framework>`, `compliance results <run-id>`, `compliance run <framework> [--env <id>]`: evidence for a named control framework, never a claim of compliance with one. `packs` lists each pack with its coverage counts and its latest run; `results` prints one row per control **including the ones McHarbor cannot check**, which carry the reason code `notCoveredByMcHarbor` rather than being left out. A check whose evidence could not be gathered reports `unknown` with a reason, and the `NOT ASSESSED` column is shown beside the passes for that reason. `run` records the run and returns immediately; the sweep continues server-side, so read the verdicts with `compliance results <run-id>`; it changes nothing on the host, so it is Reversible and asks for no confirmation, which makes it safe in a cron entry. | `compliance.view`; `compliance.manage` for `run` |
| Compliance attestations | `compliance attest <framework> [--run <id>] [--scope <label>] --note "…"`, `compliance attestations [--framework <key>]`: a sign-off on a pack run (`cis-docker-benchmark --run <id>`), on the access review (`access-review`, no run) or on a mapping as a whole (`soc2`, `iso-27001`). `--note` is required and refused locally when empty. There is no edit and no delete: a later attestation supersedes an earlier one and `attestations` lists both, newest first. `attest` is Disruptive, so it prompts on a terminal and `--yes` skips the prompt | `compliance.attest` for `attest`; `compliance.view` for `attestations` |
| Runbooks | `runbook list`, `runbook get <id>`, `runbook runs [--incident <id>] [--started-by <id>] [--trigger manual\|alert\|schedule]`: read the runbooks attached to an alert type, a container, a stack or an environment, and the runs recorded against them: who responded (a person or a service principal), what started the run (`TRIGGER`), how it ended (`completed`, `abandoned`, `escalated`, `in_progress`) and which step it stopped at. `--started-by <principal-id> --trigger schedule` is "what did this service principal do overnight". All three are read-only, so none asks for confirmation. There is deliberately **no `runbook run`**: a person's step is authorised against the operator pressing it, and unattended runs are started by the server, not by the CLI | `runbooks.view` |
| Runbooks (unattended) | `runbook principals` lists the service principals a runbook may run as. `runbook automate <id> --principal <id> [--on-alert] [--cron '<five fields>']` makes a runbook run unattended as that service principal, when its alert type fires, on a schedule, or both; `runbook automate <id> --off` stops it. Turning it on is Disruptive (it asks, or needs `--yes` off a terminal) because it lets a principal act with nobody watching; turning it off does not ask. Every step is still checked against the principal's own roles | `runbooks.automate` |
| Runbooks (catalog) | `runbook catalog` lists every action and diagnostic a runbook step may name (the host's container entries and, while their plugins are loaded, the Virtual Machines, System Containers, Compute, Swarm and Kubernetes entries) with `KIND` (action or diagnostic), `PLUGIN` (empty for a host entry), `TARGET` (what it acts on), `NEEDS` (the permission checked per step against whoever runs it, a person or a service principal) and `DESTRUCTIVE`. It is the answer to "which roles does this service principal need?". Read-only, so it never asks for confirmation | `runbooks.view` |
| AI assistant | `ai providers`, `ai actions`, `ai approve/reject <action>`: the consent step stays; nothing the assistant proposed runs without it | `settings.view`, `ai_assistant.view`, `ai_assistant.act` |
| Support | `support diagnostics [--check a,b]`, `support inventory`, `support encryption-key`, `support pre-upgrade [--target <version>]`, `support bundle preview/create/list/get/download/delete`: `diagnostics` exits `0` whatever the verdict, because a failing check is data; `pre-upgrade` exits `7` when a check blocks the upgrade, so CI can gate a rollout on it; `download` writes a file and refuses to overwrite one without `--force`; `delete` is irreversible and needs `--confirm <bundle-id>` | `support.view`, `support.manage` for create, download and delete; `updates.view` for `pre-upgrade` |
| Settings | `settings get <key>`, `settings set <key> <value> [--category <c>]`, `settings posture`: one row of the settings table through the interface's own routes. The server validates the value (`security.mfa_required` takes `off`, `admins` or `all`; anything else is refused with `err.settings.mfa_requirement_invalid`), so the CLI adds no validation. `set` keeps the row's existing category unless `--category` is given. Changing a `security.*` key is Disruptive: it prompts on a terminal and `--yes` skips the prompt; other keys are Reversible. `posture` lists the boot security-posture findings, each with the environment variable that addresses it, and prints `no findings` when every check passed | `settings.view` for `get` and `posture`; `settings.manage` for `set` |
| Shell | `completion bash/zsh/fish/powershell` | — |

A refused command exits `4` and names the key that would have allowed it:

```text
Error: volumesCreate: You do not have permission to perform this action (HTTP 403) [err.rbac.permission_denied]; missing permission: volumes.manage
```

## Audit

The server writes the audit row, not the CLI, so a change made with `mcharbor volume
remove` produces the same `delete` / `volume` row the interface produces. The row names the
key's owner as the user **and** the API key it was made with. The audit table shows
"via API key <name>" under the user, and the key's id is part of the row's tamper-evident
hash, so rewriting which key made a change breaks the chain. Give automation its own key
and its changes read as its own.

## Plugin commands

A plugin can contribute its own command group, so a VM, a system container, a Swarm service,
a compute pool or a report is reachable from a terminal and not only from the interface. The
six that ship contribute these roots:

| Root | Plugin | Covers | Permission keys |
|---|---|---|---|
| `kvm` (alias `vms`) | Virtual Machines | guests, disks, NICs, snapshots and their schedules, clones (`vm clone --linked`), moves between hosts (`vm migrate`, `vm migration`, `vm migration-cancel`, `vm.migrate`), templates, imports, backups, the serial console, and `vm console <id> --console vnc [--listen 127.0.0.1:5900]`, which serves the VNC console on a local port for a native VNC client (one console ticket per connection, nothing opened on the host); `kvm reboot-drain list|stop` lists and stops the running guests a host reboot would stop (`host_updates.view`, `host_updates.reboot`); `vm cloud-init` shows masked user-data (`vm.manage`) | `vm.view`, `vm.manage`, `vm.delete`, `vm.import`, `vm.privileged`, `vm.backup.*`, `vm.restore.run`, `vm.console`, `vm.migrate` |
| `lxc` (alias `ct`) | System Containers | containers, processes, NICs, mounts, snapshots, clones (`container clone --linked`), backups and plans, the console; `lxc reboot-drain list|stop` lists and stops the running containers a host reboot would stop (`host_updates.view`, `host_updates.reboot`) | `ct.view`, `ct.manage`, `ct.delete`, `ct.console`, `ct.backup`, `ct.restore`, `ct.privileged` |
| `compute` | Compute Foundation | pools, networks, images, host locations, the capability probe | `compute.view`, `compute.manage`, `compute.delete`, `compute.images.upload`, `compute.pools.browse` |
| `swarm` (alias `swarms`) | Docker Swarm | the cluster, services, tasks, nodes, secrets, configs, stacks, networks | `swarm.view`, `swarm.manage`, and the per-object `swarm_services.*`, `swarm_nodes.*`, `swarm_secrets.*`, `swarm_configs.*`, `swarm_stacks.*`, `swarm_networks.view`, `swarm_tasks.view` |
| `k8s` (alias `kube`) | Kubernetes | clusters, pods, deployments, services, namespaces, pod logs | `kubernetes.view`, `kubernetes.manage`, `kubernetes.delete`, `kubernetes.logs`, `kubernetes.clusters.manage` |
| `report` (alias `reporting`) | Reporting | report types, jobs, schedules, delivery targets, the ad-hoc studio, exports | `reporting.read`, `reporting.generate`, `reporting.manage`, `reporting.schedule` |

Run `mcharbor plugin cli list` for the tree your instance actually offers, and
`mcharbor plugin cli explain <root> <path>` to see which route a command calls and what it
needs. A refused plugin command names its key exactly as a core one does.

**`ct` and `ctr` are not the same thing.** `ctr` is an alias of the core `container` command
and means a Docker container; `ct` is an alias of the `lxc` root and means a system container.
The short forms are one letter apart on purpose, to match the permission prefixes
(`containers.*` against `ct.*`) and the plugin's own vocabulary, but if you use both kinds,
prefer the full `container` and `lxc` names in scripts.

Two scoping notes, because they differ from the rest of the CLI:

- **`k8s` is cluster-scoped, not environment-scoped.** Every command takes `--cluster` rather
  than reading `--env` or the context's environment. `k8s summary` is the only enumeration the
  plugin exposes, so it is how you get a name for everything else.
- The `k8s` surface is thirteen commands because the plugin registers thirteen routes. Anyone
  expecting `kubectl` will be disappointed, and closing the gap means new plugin routes rather
  than a longer command table.

**These commands come from the server, not from the binary.** The CLI fetches the command
spec at `login`, at `context use`, and on demand with `mcharbor plugin cli refresh`, then
caches it per context under `<config dir>/cli-spec/<context>.json`. The tree is built from
that cache alone, so `--help` works offline and costs no request.

Three consequences worth knowing:

- **A root disappears when its plugin is uninstalled.** Until you refresh, your cached tree
  still offers commands that now answer 404. `mcharbor doctor` reports the cache's age and
  the server build it came from.
- **A command this CLI is too old to run is skipped, not fatal.** `doctor` says how many and
  from which plugin. Upgrading the CLI is the fix.
- **`MCHARBOR_NO_PLUGIN_CLI=1` turns the surface off**, which is what a CI job wants when it
  needs the tree frozen to exactly what the binary ships.

Confirmation works the same way as for a core command: a plugin's destructive command is
`irreversible` and refuses `--yes`, needing `--confirm <target>`. A plugin cannot loosen that.

Writing a plugin that contributes commands: see
[Plugins → CLI commands](../plugins/cli-commands/README.md).

## Not in this release

`tui` is one screen, containers. A log pane is deliberately not there yet: it earns its
place once the container screen has been lived with, not before.

## Restoring an instance that will not start

`mcharbor restore --local <export.mchx>` restores an instance export with McHarbor stopped, by
running the server binary's own restore against the data directory. See
[Instance backup → Restore](../settings/system/instance-backup/restore.md#restoring-an-instance-whose-interface-will-not-start).
`mcharbor audit verify` reports a chain that went through a restore as *verified across N restore
anchors* and lists them; a broken chain exits 1.
