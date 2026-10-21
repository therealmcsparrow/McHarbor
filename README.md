# McHarbor

Your containers. Your clusters. Your rules.

McHarbor is a self-hosted control plane for Docker environments, extendable to Kubernetes and Swarm through plugins. It brings enterprise-level functionality, container management, remote access, live operational visibility, dashboards, workflow automation, role-based access control, a tamper-evident audit log, an AI assistant, and a signed plugin platform into one platform that you run yourself.

Deploy it as a Docker container, connect it to the host Docker socket, and start managing infrastructure from one clean, dark-first interface. Add remote Docker hosts through the outbound McHarbor agent, install the Kubernetes or Swarm plugin to manage clusters beside Docker environments, stream logs, open browser terminals, build dashboards, automate routine work, and keep a clear record of operational changes without handing your infrastructure to a third party. McHarbor is free for personal and homelab use. Commercial and business use requires a commercial license.

**Current release: 2026.10.01.** See [What's New in 2026.10.01](#whats-new-in-20261001) and the [CHANGELOG](CHANGELOG.md) for the breaking changes before upgrading from 2.x.

## Why McHarbor

McHarbor is built for the work that happens after deployment.

- Inspect, start, stop, restart, and remove containers from a UI built for real operations.
- Manage images, volumes, networks, and Compose stacks without bouncing between point tools.
- Operate pods, deployments, services, and namespaces from the same platform you use for Docker by installing the Kubernetes plugin.
- Reach remote Docker hosts behind NAT and firewalls through an outbound agent over WebSocket.
- Follow live logs and infrastructure events, then drop into a browser terminal when you need shell access.
- Build custom dashboards and workflow automations instead of relying on a fixed operational surface.
- Keep activity, alerts, and audit history visible as your environment grows.
- Use the same platform from a Raspberry Pi homelab to enterprise operations without changing tools.

## Named Features

- **Self-Hosted Deployment**: Run McHarbor on your own infrastructure as a Docker container and keep your environments, credentials, and operational data under your control.
- **Instance Backup and Restore**: Export the whole control plane — roles, environments, secrets, and the certificate authority your agents trust — into one encrypted file on a schedule, rehearse restoring it, and bring the instance back after losing its host, onto new hardware or another database, or one category at a time. The command line can restore an instance whose interface will not start.
- **Five Database Backends**: Run on the embedded, zero-config SQLite file by default, or point McHarbor at PostgreSQL, MySQL, MariaDB or Microsoft SQL Server when you need several nodes against one database. The whole migration set is translated at apply time and verified against every backend, and Settings → System → Database copies every row to another one without touching the old one.
- **Plugin Commands in the Terminal**: A plugin contributes its own `mcharbor` command group, so virtual machines, system containers, Swarm services, compute pools, Kubernetes objects and reports are reachable from a script and not only from the interface — 228 commands across six roots. A plugin declares a command rather than handling it, so a destructive plugin command refuses `--yes` exactly as a core one does, and can never advertise a weaker permission than the route it calls enforces. The command tree is built from a cached spec, so `--help` works offline.
- **A Command Line That Covers The Instance**: Beyond containers and stacks, `mcharbor` reaches diagnostics and support bundles, capacity forecasts, runbook history, compliance packs, preview environments and a multi-node cluster — with one interactive screen (`mcharbor tui`) for the container list, and a local `plugin dev` toolchain for authors that talks to no server at all.
- **Capacity Forecasting Without A Cost Model**: A sentence with a date — *this host reaches 90% on 6 October at the current rate* — from a linear fit over retained samples, with the confidence and the window beside it. It refuses to answer rather than extrapolating from too short a series, and it starts with idle-resource identification, which reclaims real storage and needs no forecasting at all.
- **Compliance Packs, Stated Honestly**: Map a named control framework to the evidence McHarbor already produces and the checks it can run against a live host — the CIS Docker Benchmark first, with 38 controls of which 31 are checked mechanically. A check that could not run reports *unknown* with a reason rather than passing, and the pack says which controls are **not** covered and why. It is evidence for a framework, never a claim of compliance with one. A reviewer's sign-off is recorded too — who accepted which run, when, with a note — under its own permission, so running a check and accepting its result stay separate authorities.
- **Host Services Without SSH**: On a native-agent host, list every systemd service, start, stop, restart, enable or disable it, edit its configuration as a drop-in override and run daemon-reload — with the units that keep the host reachable protected from being stopped.
- **Agents That Update Only From Your McHarbor**: McHarbor downloads agent releases from the McHarbor catalog, GitHub or your own mirror, verifies each against a checksum and a pinned signing key, and serves the version and build you choose to every install and update — binary and container alike. Agent hosts never contact GitHub or a registry.
- **Runbooks For The Person Holding The Pager**: Attach an ordered set of steps to an alert type, a container, a stack or an environment — instructions, read-only diagnostics that show their output inline, and actions that execute and are audited. Each step runs under the operator's own permissions, so a runbook cannot do what its operator could not; a run records which steps they completed, what they found and where they stopped, and appears in the post-incident timeline. Built for a phone at 03:00, one step at a time, and the read path works offline. A runbook can also run unattended on its alert or a schedule, as a service principal that holds ordinary roles and can never sign in. A runbook exports as a portable file so it can be handed to another install, and as a PDF or a Word document for the binder. A **Runbooks report** says which alert conditions can fire with no runbook behind them.
- **Ephemeral Preview Environments**: A pull request opens an environment with a generated hostname, a TTL and a per-pipeline concurrency cap, and merging, closing or expiry destroys it. Cleanup survives a force-pushed branch, a deleted repository and a restart mid-teardown, and teardown only ever deletes an environment explicitly marked as that preview's.
- **Active/Active, Or An Honest Reason Why Not**: Two nodes can now serve the same fleet: an agent's owning node is leased in the shared registry and a request arriving elsewhere is forwarded to the owner with the caller's identity and timeout intact, with the audit row recording which node executed it. The readiness check names the five stream paths that are pinned to the owning node rather than forwarded, instead of implying the mode is complete.
- **Dark-First Interface**: Work in a modern UI designed for long operational sessions, with a light mode toggle available when you want it.
- **Table Views**: Choose, reorder and resize the columns on any table and save named filters to reapply later — personal, server-stored, and the same wherever you sign in. A fresh install opens every table on a curated default layout, and Reset returns to it. Select every row matching a filter, across pages, before a bulk action.
- **A Palette That Knows Your Plugins**: Ctrl+K lists the virtual machines, system containers, Swarm services, Kubernetes pods and compute hosts that installed plugins declare, and offers their actions — create a machine, add a cluster, deploy a stack — to whoever holds the permission. The host names none of them; a plugin declares `search-source` and `search-action` capabilities.
- **Multi-Environment Management**: Connect local Docker Desktop, remote Linux servers, Raspberry Pi systems, NAS devices, Proxmox-backed labs, and, with the matching plugin, Kubernetes clusters and Swarm nodes in one interface.
- **Container Lifecycle Management**: Inspect, create, start, stop, restart, and remove containers from a workflow designed around day-to-day infrastructure work.
- **Image, Volume, and Network Management**: Review container artifacts, persistent storage, and service connectivity from one operational surface.
- **Compose Stack Management**: Treat Docker Compose applications as first-class stacks instead of loose groups of containers.
- **Kubernetes and Swarm Plugins**: Install the Kubernetes plugin to manage pods, deployments, services, and namespaces, or the Swarm plugin to manage nodes and services, alongside your Docker environments.
- **Virtual Machines and System Containers**: Install the Virtual Machines plugin to run KVM guests, or the System Containers plugin for full-system LXC containers, on the hosts you already manage. Storage pools, host networks, verified guest images, browser consoles (VNC, serial and SPICE, or a `.vv` file for remote-viewer), snapshots, clones and templates, moves between hosts cold or live, and scheduled off-host backups that go incremental after the first copy and refuse to record an incremental the host did not actually take — with container CPU, memory and process limits that apply without a restart. McHarbor drives QEMU and libcontainer directly; there is no libvirt, Proxmox, LXD or Incus in the path. Guests can be given resizable memory, processor hotplug, hugepages, NUMA placement, shared host directories over virtiofs and per-disk I/O limits — each offered only where that host's own QEMU can actually do it, and disabled with the missing piece named where it cannot, rather than failing at use time. Each compute host gets a detail page of its own: what it is, how it is configured, a readiness checklist that says why it cannot carry a guest yet, and eight log sources — including libvirt, the agent's own journal and per-guest QEMU logs — so a failing host is read in the browser instead of over SSH.
- **Remote Agent Connectivity**: Use an outbound agent to manage Docker hosts behind NAT and firewalls without exposing the daemon directly.
- **Stats That Scale With Your Fleet**: Current agents push their container stats once per interval instead of being asked per container, and every open tab shares one sample and one event stream per environment. Each environment shows which path its stats take.
- **Live Log Streaming**: Follow log output in real time when you need answers quickly during debugging or incident response.
- **Live Event Streaming**: Watch infrastructure events as they happen so restarts, pulls, failures, and runtime changes stay visible.
- **Browser Terminal Access**: Open terminal sessions directly from the UI for fast operational access when logs are not enough.
- **Custom Dashboard Widgets**: Assemble dashboards with widgets for host info, stack status, activity, resource summaries, charts, and workflow visibility. Signed widget bundles from a local catalog install with their own permission and load only when their signature verifies.
- **Workflow Automation Engine**: Create visual automations with triggers, multi-branch conditions, actions, schedules, and utility nodes.
- **Custom Workflow Nodes**: Extend the workflow engine with sandboxed JavaScript nodes for internal tooling and third-party integrations.
- **Webhook and Scheduled Triggers**: Start automations from HTTP events or recurring schedules for repeatable maintenance and response flows.
- **Git and GitOps Integration**: Connect operational workflows to versioned configuration with pipelines, promotions, approvals, and pull-request previews.
- **Registry Integration**: Keep registry-related workflows in the same control plane instead of treating image distribution as a separate system.
- **External Storage Plugins**: Add reusable Box, Dropbox, FTP/FTPS, Google Drive, Nextcloud, NFS, OneDrive, OpenCloud, ownCloud, Samba, SFTP, SharePoint Online, and TrueNAS destinations for backups and exports by installing the matching storage plugin.
- **Notifications and Alerts**: Surface important activity and failures where operators can react quickly, through Discord, Exchange, Gmail, Gotify, ntfy, Signal, Slack, SMTP, Teams, Telegram, and WhatsApp channel plugins.
- **ITSM Tickets**: Alerts open, note and resolve tickets in PagerDuty, Zammad, GLPI, iTop, OTRS / Znuny / OTOBO and Request Tracker, or reach any Alertmanager receiver; acknowledgements and closures in the ITSM come back and quiet repeat notifications.
- **On-Call From Your Phone**: Install McHarbor to a phone's home screen for an alert inbox, a fleet glance, container restart, a searchable log tail and elevation approvals, with built-in push notifications that open the right screen. It shows the last data it saw when offline, always with the time it was fetched, and uses the same permissions as the desktop.
- **Incident Narrator**: A failing container — or, with the matching plugin, a virtual machine, system container, Kubernetes workload or Swarm service — becomes an incident with its events, logs, deploys, audit trail, alerts, drift and metrics correlated on one screen, a timeline when it resolves, and — only for environments that opt in — a narrative from your own AI model naming the likely causes and the evidence for each, with the exact prompt kept for review.
- **Drift Control Loop**: Declare a container, a stack or a whole environment — by hand or as documents in a Git repository — or, with the matching plugin, a virtual machine, system container, Swarm service or node, or Kubernetes Deployment or node, all picked from lists rather than typed, and McHarbor reports every departure by class, then observes, proposes a reviewable fix, or enforces it under a per-pass cap, a cooldown and a circuit breaker. Enforcing needs its own permission and is off until switched on; drift in a Git-backed watch can be accepted as a pull request.
- **Board-Ready Reports**: The Reporting plugin generates reports at three altitudes — strategic, tactical, operational — on environments, workloads, security, recovery, access, drift, incidents, high availability, approvals, secrets and delivery, and the platform plugins add reports only they can produce. Docker, virtual machines, system containers, Compute, Kubernetes and Swarm answer the same six questions (inventory, capacity, protection, health, exposure, cost), so the shared reports print one table across all of them. Every report is built from what the person generating it may already see, names each source it read, and exports to PDF and Word or runs on a schedule.
- **Protection For McHarbor's Own Host**: On the environment McHarbor runs on, acting on its own containers, creating a container that mounts the host or joins a namespace, and every module that could reach it needs a separate `host_self.manage` permission, so `containers.manage` is no longer administration of McHarbor itself. Background runs with nobody behind them are refused. A workflow is decided when it is saved: saving one that acts on McHarbor's own containers needs the key, and its runs may do so only while the last save was made by someone holding it.
- **Tamper-Evident Audit Log**: Write-once, hash-chained audit events with a scheduled verifier, signed exports, and syslog or SIEM forwarding keep a record of what changed, who changed it, and when.
- **Role-Based and Attribute-Based Access Control**: Roles, presets, per-environment and per-stack scopes, owner assignment, temporary elevation, and condition-based policies gate every action. A role granted on one environment carries only that environment's keys, so an administrator of one environment cannot use it to manage users or roles, and a grant made on one environment cannot be used against another. Denied controls stay visible and name the missing permission.
- **Approvals, Break-Glass, and a Policy Simulator**: Hold destructive actions until someone else approves them, give a designated operator time-boxed emergency access, behind a password step-up, that pages every account admin, and ask the simulator whether a user may do something and why.
- **Effective Access and Previewed SCIM**: See every permission a user holds in an environment and where it comes from, and run a SCIM sync as a dry run before it links, updates or disables anyone.
- **Support Bundle and Diagnostics**: Build a redacted support bundle, run diagnostics with remediation, list what lives in the data directory, and check an install before upgrading it.
- **Secrets Vault**: Store a credential once, reference it as `${secret:NAME}` from a stack, a container, or a workflow, and let McHarbor substitute it server-side at the moment it calls Docker. Encrypted at rest, gated by its own permissions, revealed only behind a step-up re-authentication, and — in file mode — delivered through `/run/secrets` so the value never reaches the container environment at all.
- **Custom Branding**: Upload your own wordmark and icon, each with a light and a dark variant, and McHarbor uses them on the login page, in the sidebar, and on generated reports.
- **Licensing That Never Switches Anything Off**: An expired, over-used, suspended, or unreachable licence changes what the banner says and nothing about what runs.
- **Identity Plugins, MFA, and SCIM**: Start with local auth, then add Entra ID, generic OIDC, Google, LDAP, or SAML through identity plugins. Multi-factor authentication — which can be required for administrators or for everyone — password policies, idle logout, and SCIM provisioning are built in.
- **AI Assistant**: Chat with your infrastructure, run structured tools with an approval step, and generate ad-hoc reports through Anthropic, OpenAI, Mistral, MiniMax, or Ollama provider plugins.
- **Signed Plugin Platform**: Grow the platform with 61 first-party plugins and your own. Plugins ship Go backends, React frontends, widgets, workflow nodes, migrations, permissions, and translations, and hook into the sidebar, topbar, menus, and page tabs.
- **Command-Line Client**: `mcharbor` operates environments, containers, images, volumes, networks, stacks, backups, GitOps, users, groups, roles, API keys, elevations, policies, the audit log, identity providers, plugins and the AI assistant's actions from a terminal or a CI job — the same permissions and audit trail as the interface, frozen exit codes, JSON/YAML/TSV output, and a refused command that names the permission it needed. Sign in by pasting an API key or with `mcharbor login --device`, approving a code in the browser. Download it for your platform from *Security → API keys*; it also ships as a CI image and inside the McHarbor container itself.
- **Observable Control Plane**: McHarbor exposes its own health at `/api/metrics` for Prometheus — HTTP, the remote-agent transport, background jobs, the database pool and plugin state — behind a scrape-only `metrics.scrape` key, with a Grafana dashboard in the repository and optional OpenTelemetry traces that follow a request through the agent to the Docker daemon — including spans the agent measures itself, relayed so the agent never needs a route to your collector. A background loop keeps desired-state drift current, and its lag is on the dashboard.
- **High Availability**: Run active/passive McHarbor nodes on a shared PostgreSQL, MySQL, MariaDB or SQL Server database — one active node fenced by a database lock, standbys that refuse changes, readiness probes for the load balancer, and Make active / Drain / Resume from Enterprise → High availability. Plugin packages are kept in the database, a shared mount or a storage location so every node can load them.
- **HTTPS by Default**: McHarbor is its own certificate authority. A ten-year root signs a 90-day certificate on first boot and renews it without a restart; install the root once from Settings → Security → HTTPS and the browser warning is gone. Upload your own certificate there instead, or issue one automatically with the bundled ACME companion app. Connections negotiate **TLS 1.3**, the preferred and shipped floor; TLS 1.2 stays available for clients that predate 2019 via `TLS_MIN_VERSION=1.2`, and TLS 1.1 and below are refused.
- **Self-Update Checker**: McHarbor periodically compares the running version against the latest GitHub release, surfaces an in-app banner when a newer version is available, and notifies configured channels. The check interval is configurable from Settings.

## What's New in 2026.10.01

McHarbor 2026.10.01 turns the core into a slim host and moves optional capabilities into signed plugins. It is a major release with breaking changes; the full list is in [CHANGELOG.md](CHANGELOG.md).

### Breaking Changes

- **Plugin-only families**: Kubernetes, Swarm, reporting, external storage providers, communication channels, identity providers, vulnerability scanners, logging forwarders, snapshot providers, and AI providers are no longer built into the core. Install the matching plugin from the Plugin Store to restore each capability.
- **Blueprints removed**: The `blueprints/` module are gone. Use the App Store catalog of more than 1,300 apps and stack templates instead.
- **HTTPS by default**: The main listener serves TLS on host port `8705`.
- **Every mutating route is permission-gated**: Sessions alone no longer authorise changes. API keys carry an explicit permission list.
- **Scoped roles carry only scoped keys**: A role assigned on one environment no longer grants `global` or `account` keys such as `users.manage` or `dashboard.view`; those need a global assignment.
- **McHarbor's own environment asks for `host_self.manage` more often**: Creating a host-level container, a namespace join, a host bind under a sensitive path, network changes, reconciles, restores, runbook steps and workflow saves aimed at McHarbor's own containers now need the key; background runs are refused, and a workflow last saved without the key cannot act on them.
- **Twelve platform report types are replaced**: Their schedules now run the shared cross-platform reports, and `container-inventory` is now `workload-inventory`.
- **Policies on the environment now apply at the route gate**: A policy that reads `env.id` or `env.name` could not see the environment there before.
- **Outbound fetches are stricter**: Kubernetes cluster connections ignore `HTTP_PROXY` and `HTTPS_PROXY`, catalog indexes are no longer read from loopback addresses, an `https` to `http` redirect is refused, and the SQL Query workflow node refuses `json_each` and other table-valued functions.
- **The installer needs a setup token**: A fresh install prints a one-time token in its log (`docker logs mcharbor`, or `journalctl -u mcharbor` on a native install, where the installer also prints it) and the installer asks for it. It changes on every restart.

### Platform

- **Plugin Platform (rewrite)**: Go plugin ABI, UMD frontend loader, declared permissions and i18n namespaces, plugin migrations, signed tarballs, built-in and trusted or untrusted remote catalogs, and a Plugin Store with categories, labels, status master/detail, rescan, and per-family test-connection modals.
- **Role-Based Access Control**: Code-first permission catalog mirrored into SQLite, default role, per-user overrides, environment and stack scopes, owner assignment, role presets, temporary elevation with an approval trail, impact preview, and a permission-aware UI.
- **Attribute-Based Policies**: Condition-based allow and deny rules layered on top of roles, evaluated at every host and plugin route against the resource it touches — containers, stacks, virtual machines, storage pools and 23 more types — with a reference of attributes, variables and functions beside the expression.
- **Elevations For Real Work**: Operational permissions, plugin ones included, can be requested for a bounded window by default, shown by their description; admins decide which permissions are requestable.
- **Governance**: Approval rules for destructive actions, break-glass access, a policy simulator, conflict warnings, six role templates, API key rotation and bulk user actions.
- **Identity and Audit**: SCIM dry runs, an identity provider wizard, Effective access, audit filters by actor and severity, denial and AI-action tabs, and retention per severity that keeps the chain verifiable.
- **Supportability and Compliance**: A redacted support bundle, diagnostics, a state inventory, a pre-upgrade check, a compliance report, a Helm chart and deployment examples.
- **Tamper-Evident Audit Log**: Buffered recorder, write-once storage, SHA-256 hash chain, scheduled verifier, retention presets, access-log capture, failed-login recording, clock-drift detection, signed export bundles, and syslog or SIEM sinks.
- **AI Assistant**: Markdown chat with history and feedback, structured inspection tools, an approval modal before mutating commands, an action log, container audit prompts, and ad-hoc reports through provider plugins.
- **Compute (Virtual Machines and System Containers)**: Three new plugins run KVM guests and LXC system containers on your own hosts. Per-host capability probing with per-distribution remediation, storage pools on directory, ZFS, btrfs and LVM, host bridges, checksum- and signature-verified image downloads that never transit McHarbor, VNC and serial consoles over single-use tickets, snapshots that report their consistency honestly, linked clones and templates, and live cgroup v2 limits for containers. The executor runs in the remote agent, so nothing is queued and an unreachable host reports its guests as unknown rather than guessing.
- **Compute Images**: A 49-entry image catalog covering desktop, server and ARM64 Linux plus Windows and Android, with distribution logos, an operating-system filter and five-column cards on a wide screen. Server entries are cloud images where the project ships one, so a guest boots in seconds and takes its user and SSH key from cloud-init. Every source and checksum URL is verified before it is catalogued; an entry whose publisher offers no usable checksum says so and downloads behind an explicit acknowledgement rather than carrying a made-up one.
- **Secrets Vault**: A credential is stored once, encrypted with AES-256-GCM, and referenced as `${secret:NAME}` from a stack, a container, or a workflow. Reading the list, revealing a value, and browsing the mount are three separate permissions; revealing also needs a five-minute step-up window, and every disclosure is audited. Turn on **file mode** and the value is delivered through a read-only `/run/secrets` volume with `<VAR>_FILE` set instead of `<VAR>`, so it appears in no `docker inspect` and no process environment.
- **Database Backends**: PostgreSQL, MySQL, and MariaDB join SQLite as supported backends, with the complete 82-migration set verified from empty against all four. No migration was rewritten — statements are translated at apply time, and SQLite still receives every file byte for byte. Switch backends from Settings → System → Database; the old database is left untouched as the rollback.
- **Licensing**: Key, tier, seats, expiry, machine identifier, and a daily handshake with the licence server, with a request-a-licence flow in the interface. Nothing is ever disabled by licence state.
- **Security**: Rebuilt MFA with TOTP and recovery codes that can be required per role, a Content-Security-Policy, login throttling and a per-address lockout that work behind a reverse proxy, boot-time warnings for LAN-only defaults, explicit consent for unsigned plugins, password policy enforcement, idle-session logout, password-reset email, case-insensitive usernames, hardened secret encryption with rotation, and SCIM provisioning. Stored credentials (environment keys, tokens, passwords and kubeconfigs) are encrypted at rest. That includes values saved before their column was encrypted, which are sealed on the first boot after upgrading. Snapshot driver secrets are never returned once saved. Fetches of operator or catalog URLs refuse loopback, link-local and cloud metadata addresses, including IPv6 forms that encode them, and the SQL Query workflow node can read only the workflow tables. Report a vulnerability as described in [SECURITY.md](SECURITY.md).
- **High Availability**: active/passive failover in seconds, `/api/readyz` and `/api/livez`, a heartbeat topology, plugin package stores, and failover validated on PostgreSQL, MySQL, MariaDB and SQL Server; `docker-compose.ha.yml` and `ha.env.example` are the template.
- **Plugin Dependencies**: A manifest can declare `requires`. Installing a plugin pulls in whatever it cannot work without — the Virtual Machines plugin brings Compute Foundation with it — and uninstalling removes the dependency again once nothing needs it, never one you installed deliberately.

- **Calendar Versioning**: McHarbor, the agent and every plugin use `yyyy.MM.<sequential>` with a `yyyyMMddHHmm` build stamp. The app and the agent share one version and no longer drift; each plugin keeps its own sequence in the same notation.
- **Native Linux Install**: McHarbor installs directly on Linux as a `.deb`, `.rpm`, or a tarball, running under systemd as root by default or as a sandboxed `mcharbor` account with `--no-run-as-root`, alongside the container which remains the default. Debian 12+, Ubuntu 24.04+, and Fedora 38+ are supported; Ubuntu 22.04, Ubuntu 20.04, Debian 11, and Rocky/RHEL/Alma 9 are not, because plugins are cgo shared objects and the build floors at glibc 2.36. Plugins are not bundled: the Plugin Store downloads all 61 from `https://www.mcharbor.eu/catalog/plugins`, and installed plugins are re-downloaded automatically after an upgrade.
- **You Choose the First Environment**: A fresh install creates nothing. McHarbor no longer detects the mounted Docker socket and inserts a `Local` environment on your behalf, because the socket a container can see is not necessarily the host you meant it to govern. Environments starts empty; the first one is yours to create, socket or agent.
- **Host Checks Before the Agent Lands**: The environment create dialog's SSH install asks the host two questions before anything is installed on it — is Docker here, and can this host run virtual machines. The second covers the CPU virtualization flag, `/dev/kvm` and whether the user can actually open it, `qemu-system-x86_64`, `qemu-img`, OVMF and swtpm, and prints the install command for that host's own package manager.
- **Prerequisites Panel For SSH Deploys**: The same check is a visible panel. It shows how privileged commands will run (root, passwordless sudo, sudo with the SSH password, or su with a root password you type for that run), installs only what you tick, and keeps Create disabled while something required is missing.
- **Cross-Platform Report Facets**: Plugins answer inventory, capacity, protection, health, exposure and cost in one row shape, and sixteen shared reports print an Across platforms section with a Platform column. The plugin contract is `ReportFacetContributor`.
- **SPICE, Clone and Migration For Virtual Machines**: A SPICE console or a `.vv` file, clones of stopped machines, and cold or live moves between hosts with the new `vm.migrate` permission. SPICE needs a QEMU built with it, and live migration has not yet been run between two real hosts.
- **Display Profiles For The Browser Console**: A **Display profiles** tab holds the SPICE options a virtual machine starts with — three read-only presets (Fast LAN, Balanced, Low bandwidth) you can duplicate, and custom profiles you can edit. A machine's profile applies at its next start; `mcharbor kvm display-profile` does the same from a terminal.
- **Hardware Changes On A Running Machine**: Add, grow and remove data disks, swap or eject a disc, and add, remove or unplug the link of a network interface without stopping the machine, on hosts that report `hotplug`. A refusal says what to do, and a step that fails after the guest changed is undone.
- **A Console That Follows The Window**: The VM console asks the guest to resize to the pane (`spice-vdagent`, or virtio-gpu over VNC) and says why when the guest does not answer. A focused console owns every key, so McHarbor's shortcuts stay quiet while you type into a guest.
- **Signed Dashboard Widgets From A Catalog**: Install widget bundles under `dashboard_widgets.install`; each loads only when an ed25519 signature over its key, version, bundle and metadata verifies against a trusted key.
- **Guest Hosting, Fully Supervised**: Guests are systemd transient units on the host, so they survive the agent restarting or updating itself. A guest keeps its own directory inside its pool, with every qcow2 backing reference written relatively, so moving storage no longer takes the guests down with it.
- **Plugins Write the Timeline**: A lifecycle handle joins the audit handle in the plugin contract. Virtual machine state changes reach the Logging timeline with the plugin named as their source, and plugin audit rows carry the acting user.
- **HTTPS You Can Actually Trust**: McHarbor runs a local certificate authority. A ten-year ECDSA root signs a 90-day certificate and renews it thirty days out with no restart, so regenerating rotates the certificate and leaves installed trust intact. Download the root from Settings → Security → HTTPS, verify its SHA-256 fingerprint out of band, and install it once per device. Remote agents pin that root automatically — every install path carries the pin and embeds the PEM — instead of skipping verification the way a self-signed host used to make routine. An uploaded certificate of your own is never reissued over.
- **Database Health Check**: Settings → System → Database sweeps the live database for corruption, rows whose parent is gone, schema drift and reclaimable space, on whichever of the four backends you run. Where a driver cannot answer — PostgreSQL has no in-server integrity check without `amcheck` — it reports the honest subset and says why it skipped the rest, rather than reporting a database it did not verify. Repairs are opt-in, re-verified against the live database before they run, and never offered for governed audit rows.
- **An Audit Trail That Outlives Its Subjects**: Environment and entity names are written onto the row when the event is recorded rather than looked up when you read it, so deleting a user, group, channel or environment no longer degrades its history to a truncated id. Search matches the name as it was at the time, and `audit_logs` is treated as a governed table that no maintenance sweep will offer to prune. A plugin's audit history outlives the plugin too: removing it never removes what it did, and the Coverage tab marks the slug **Uninstalled** rather than quietly dropping the evidence of its removal.
### Operations

- **Incidents and narration**: Crash loops, out-of-memory kills and failing health checks open incidents with their correlated evidence; plugins raise their own — a VM stopped with nobody in McHarbor asking, a frozen guest or a full disk inside it, a pod in a crash loop, a Swarm service that will not converge; opted-in environments get an AI narrative with cited evidence, and behaviour findings flag slow memory climbs, nightly restarts and post-deploy failures.
- **ITSM tickets**: Alert destinations can be ticketing channels — an ordered, retried outbox opens and resolves tickets, the ITSM reports status back, and workflow nodes open, update and resolve tickets too.
- **Drift plans, enforcement and Git sources**: Reconciler watches — now also for VMs, system containers, Swarm and Kubernetes when their plugins are installed — can propose a reviewable fix or enforce it within strict limits (never a disruptive step unattended), read their intent from a Git repository, and turn accepted drift into a pull request.
- **Offsite instance backups, to more than one place**: "Back up now" and every schedule take any number of storage locations rather than one, and attempt each even after another fails — the reason to hold two is that one of them may be unreachable. The copies follow the completed local export and never replace it, so a refused upload is recorded against the backup instead of losing it, and deleting a backup removes every remote copy it made.
- **Multi-branch workflow conditions**: A Condition node routes on its own test, then on each extra condition in turn, and sends anything matching none out of an else port — so one node replaces a chain of them. Greater-than, less-than, starts-with and matches-regex now work alongside equals and contains.
- **Sixteen more reports**: Incident narrative, drift and enforcement, instance disaster recovery readiness, high availability posture, ITSM integration, change approvals and privileged access, secrets hygiene, workflow automation, webhook delivery and compliance posture in the Reporting plugin, plus VM backup assurance, guest stability, snapshot schedules, system container protection, compute image and lease hygiene, and SIEM and syslog delivery from the plugins that own the data.
- **Backups Overview**: Dedicated Backups page with run history, encryption-first key setup, three-way restore selection, per-file restore, restore polling with scope, and freshness insights.
- **Config Editor**: Edit and restore container and network configuration with item selection and tree colouring.
- **Container Detail Redesign**: Tabbed layout with URL-persisted tabs, hero header, quick edit, health and uptime graphs, activity history filter, GPU support, and a rebuilt terminal.
- **Metrics History and Reports**: Metric selector, health history, and host hardware probing.
- **Alerts and Widgets**: Plugin alert sources, failed-login alerts, a fleet-health banner, and fifteen new dashboard widgets including fleet health, needs attention, update queue, vulnerability summary, and snapshot status.
- **Workflow Nodes**: Send-communication routing through any channel plugin, backup plan runs and downloads, host exec and host info, metric record and trigger, file-watch trigger, FTP upload, SQL query, SSH exec, GraphQL request, HTML/XML/YAML/CSV parsers, rate limit, try/catch, and junction.
- **App Catalog**: more than 1,300 app definitions, external trusted and untrusted catalogs, editable catalogs, catalog test button, and image-availability checks.
- **Offline Agent Buffering**: An agent that loses its connection keeps recording Docker events, per-container stats and its own log lines into a local SQLite buffer, then replays them in acknowledged batches on reconnect. Nothing is lost or duplicated, and the agent's own log is readable from the environment page.
- **Push-Based Agent Stats**: An agent at 2026.09.01 build 202609251653 or later streams Docker's stats on its own host and sends one batch per interval (Settings → System → Agent, default 10 s), so a host with hundreds of containers gets complete metrics history. The environment Overview's Container stats card says whether it is pushed, pulled or read directly. Older agents keep working on the pull path until you install the new binary.
- **Virtual Machine Diagnostics**: A Logs tab on the machine detail page reads the guest's own systemd units — QEMU itself, and its software TPM separately, because a vTPM that died takes the guest down with it and says nothing in QEMU's log. Power controls sit beside the console so you can shut down, reset or pause the guest you are watching without leaving it, gated on `vm.manage` rather than `vm.console`. Both need an agent at 3.8.0 or newer.
- **Build a Machine on What You Have**: Create a virtual machine with no image at all — it starts at the firmware prompt, for a PXE install or a disc attached later — or on a disk that already exists in the pool and no machine refers to, which is how a guest deleted without its volumes gets booted again. Nothing is written to an adopted disk.
- **Imports Run in the Background**: The guest import dialog no longer disappears on a stray click, and closing it deliberately leaves the upload and conversion running, with a chip on the machine list to reopen it.
- **Per-Container Recreate**: Rebuild a single container from its current image and configuration from the table row, the card, or the detail hero — not only as a bulk action. The confirm dialog also carries the secret file-mode switch, because Docker fixes environment and mounts at creation.
- **Agent Health and Uptime**: An agent carries a derived health from its keepalive rather than its socket, sampled every 30 seconds with 7-day retention and no extra agent traffic, shown as health and uptime cards on the environment overview.
- **Logs and Terminals**: Server-side log search across the whole stream, a log timeline view, fullscreen on every terminal, reported colour support, and a host terminal that takes a shell, a user, and a working directory.
- **Update Automation**: Install available updates on a policy, set that policy per container, and keep per-agent exceptions.
- **The mcharbor CLI**: `mcharbor login https://your-host:8705`, then `mcharbor container list`, `mcharbor stack up web`, `mcharbor backup plan run <id>` or `mcharbor doctor`. In CI, set `MCHARBOR_URL` and `MCHARBOR_API_KEY` instead. It is already on the `PATH` inside the McHarbor container.
- **Prometheus Metrics and Traces**: Scrape the control plane itself at `/api/metrics` with an API key holding `metrics.scrape`, import the shipped Grafana dashboard, and set `OTEL_EXPORTER_OTLP_ENDPOINT` to export traces. A wedged agent that still reports running shows up as a stale keepalive and a climbing time since its last completed request.
- **Storage Capacity**: Storage destinations report used of total space with a bar that turns amber past 75% and red past 90%. Every bundled provider reports capacity from its own quota call; FTP says so when the protocol cannot.

- **Host OS Updates**: An Updates tab on the environment reports the distribution, kernel and package manager, checks for outstanding OS updates on a schedule you set, and applies them with the output streamed into the page. Previously unreachable on every remote host. How far an upgrade may go is the operator's call: **Install new packages when required** lets apt pull in the new image and header packages a kernel ABI bump needs — without it those are reported as "kept back" and never apply — while still never removing anything. It is per environment and off by default.
- **Templates, Both Ways**: Turn any stopped virtual machine into a template from its list row, card or page header, and pick **From a template** when creating a machine — as an instant linked copy or a full one, with its own identity either way.
- **Snapshots as a Tree**: Guest snapshots branch the way VirtualBox and Hyper-V present them, with expandable branches, free-form names, memory snapshots, and a revert that keeps the branch below it. Reverting a running guest stops it, reverts and starts it again for you, and the snapshot reports its progress on the row instead of holding a modal open.
- **Guest Tooling**: A prebuilt guest-agent disc one click from the machine header, list, card or Hardware tab; the detail page’s full action set on the list and card; a console that survives navigation and closes only on Disconnect; and a Logs tab on the same filter, tail and export toolchain as a container’s runtime log.
- **Imports Run on the Host**: The uploaded disk streams through McHarbor to the agent once and stays there, so inspection and conversion run against a path on the host. The list carries progress, cancel, recovery for a job whose request died, and a Conversion tab of staged files. The convert form gained Secure Boot and vTPM.
- **Environment Installer**: Environments are editable after creation. The installer offers Socket or Agent and Manual or Automatic (SSH), always hands you an install URL for both the container and the binary install, remembers SSH credentials as vault references, resolves `${secret:NAME}` in them, and detects whether the host escalates with `sudo` or `su`. The binary install writes a systemd unit, and its download resumes and verifies its length.
- **Compute Reporting**: Four more widgets — image footprint per host, guest networks, snapshot coverage, imports in flight — and reports that carry charts, findings and narrative instead of a KPI strip, with an All environments scope that is actually honoured.
- **Encrypted, Or Not, At A Glance**: The Environments list carries a shield per agent — green when the control channel is pinned or verified, orange when it is plaintext or verification is off — with the exact mode on hover. The agent reports its own mode, because only the agent knows whether it skipped verification.
- **Pick The Parent Interface, Do Not Type It**: Creating a macvlan or ipvlan network lists the host’s own physical, bond and VLAN links with their kind, address and MTU, with manual entry still there for a name McHarbor cannot see. Each driver mode explains itself in the dropdown. On a socket environment, where there is nothing to discover, the field says so instead of opening empty.
- **A Guest Network Says What It Will Do To The Host**: The create-network form lists the ordered host-side steps for the mode you picked, and what deleting the network undoes. Sharing an adapter states the contract in full: the agent arms a 45-second rollback before it replies, so a long pause and a briefly offline host is the safety net working — and it names the two things the rollback does not cover.
- **Guests Straight On Your LAN**: A macvlan guest network gives each VM or system container its own MAC on a host adapter and an address from your own DHCP server, with the adapter left untouched — even one already carrying Docker macvlan containers. Optional host access lets the host reach its own guests.
### Interface

- **Tidal Restyle**: Modals on the Dialog primitive with an aurora surface, opaque dropdowns, uniform control heights, unified form surfaces, glass widget backgrounds, and a five-wave table-page layout sweep.
- **Navigation**: Command palette, grouped sidebar with environment selector and status pills, collapsible groups, breadcrumbs in the topbar, keyboard tab commands, and tab context menus.
- **Themes and Appearance**: Theme plugins and theme packs, a Themes tab, Google Fonts support, and per-user toast settings with a live preview.
- **Custom Branding**: Four logo slots — wordmark and icon, each light and dark — under Settings → General → Customization, used on the login page, in the sidebar header, and as the default mark on generated reports.
- **Dashboard and Sign-In**: A workload heatmap widget with one square per container, and a redesigned sign-in page.
- **Settings Reorganised**: Security tabs moved to a Security page, timezone setting, and rearranged General, Appearance, Retention, Catalogs, Themes, and Communications tabs.

- **Flat Material**: The shell, header, content, cards, dialogs and menus moved off glass onto flat surfaces with square corners and hairline seams, in both themes.
- **A Real Page Head**: Detail and settings pages render full width, with each page’s icon, title and identity chips in a band under the header rather than compressed into the 56px top bar. The hard-coded quick menu is gone; the tabs you pin yourself sit at the right of that band as badges, on every detail page.
- **Feedback and Focus**: Every button variant acknowledges hover and click, and no dialog closes on a stray click outside it.
- **A Store That Fits The Screen**: The app grid goes to four columns on a wide display, matching the Nodes and Widgets tabs, and the card region scrolls instead of the whole page.
- **On-Call View and Push Notifications**: `/oncall` — chosen automatically on a narrow screen — is an installable phone view with five screens: alerts, fleet, one container with restart, its logs, and elevation approvals. Web Push is built in and turned on per device under *Device*; a push passes through the browser vendor's push service, encrypted, and carries only the alert text and a link. Phone use needs a publicly trusted certificate.
- **Host-Wide Network and Disk I/O**: An environment's Overview charts bytes per second over the host's physical interfaces and disks, so containers, virtual machines, Kubernetes and the host itself all count. The Disk Usage tile is now Disk I/O. Agent hosts need the 2026.10.01 agent; the local environment reads the host through a read-only `/proc` mount in `docker-compose.yml`.
- **Virtual Machine Metrics History**: The Virtual Machines plugin samples every running guest every 30 seconds and keeps a day of history, so a machine's charts open on the last hour. Guests on a macvlan network now report their traffic.
- **Plugins Follow the Release**: On the first boot of a new release, installed plugins move to the catalog version built for it instead of being disabled.
- **Accessible Controls**: Status messages, option groups and progress bars are native elements screen readers announce, every input is tied to its label, and disabled controls stay keyboard-reachable so their tooltip says why.

## What's New Since 1.0.0

McHarbor has grown from the initial public release into a broader operations platform. These are the user-facing features added after `1.0.0`.

### Identity, Users, and Preferences

- **OIDC Authentication**: Connect McHarbor to generic OpenID Connect providers for centralized sign-in.
- **SAML 2.0 Authentication**: Configure SAML identity providers for environments that standardize on enterprise SSO.
- **Profile Management**: Edit account details such as display name and email from a dedicated profile page.
- **Language Preferences**: Persist each user's language preference across sessions, login responses, settings, and profile controls.
- **Expanded Locales**: Added Spanish, French, and Portuguese UI translations across frontend resources, backend API messages, widgets, and workflow nodes.

### Environment and System Operations

- **Environment Overview Cards**: Review environments in card or table mode with CPU/RAM sparklines, container state totals, and image update counts.
- **System Menu**: Inspect host overview metrics, services, processes, runtime dependencies, OS logs, OS terminal access, and OS package updates.
- **Protected Host System Actions**: Use guarded endpoints for host terminal sessions, bounded log snapshots, and package update checks/apply flows.
- **Protected McHarbor Container**: McHarbor now detects and locks unsafe actions against its own running container, including bulk actions, removal, updates, reinstalls, file edits, prune candidates, and destructive lifecycle operations.
- **Container Rename**: Rename Docker containers from the UI with backend validation, audit logging, translated feedback, and protected-container checks.
- **Container-to-Stack Relinking**: Manually link, relink, or unlink standalone containers to managed stacks from container and stack detail views.
- **Per-Workflow Import and Export**: Move workflow definitions between installs with portable workflow JSON files.
- **GitHub Shortcut and Agent Copy Controls**: The About and agent setup surfaces include faster access to the GitHub repository and copyable Docker/binary agent install commands.

### Container Moves and Data Portability

- **Container Moves Between Environments**: Move containers from one Docker environment to another with a preview of required image, volume, stack-label, and network changes.
- **Move Execution Support**: Transfer missing images, create missing named volumes and networks, copy named volume data, preserve Compose stack labels, and optionally stop or remove the source container.
- **Filesystem Snapshot Moves**: Preserve data written inside the source container's writable layer, including containers that do not use Docker volumes.
- **Editable Target Networks**: Adjust target network name, mode, driver, IPAM subnet/gateway/range, aliases, target IP/MAC, internal mode, and attachable settings before moving.
- **Editable Target Volume Mounts**: Change target Docker volume names and target container paths during a move, prefilled from the source volume name and mount path.
- **Remote-Agent Move Support**: Large image and filesystem snapshot transfers now use staged remote-agent uploads so slow target Docker hosts can finish loading archives reliably.
- **Pull-Based Agent Transfers**: Container restores and moves that target remote agents use one-use McHarbor transfer URLs. The target agent pulls the image or archive, stages it locally, validates transfer size when available, then applies it through the host Docker socket instead of pushing large tar streams through the generic Docker-over-WebSocket proxy.
- **Agent Move Archive Transfers**: Named-volume data copied during container moves can be transferred as container archives, including agent-to-agent moves and McHarbor-to-agent moves, with progress events for transferred bytes.

### Workflow Automation

- **Workflow Help and Discovery**: Node documentation, category sections, and palette discovery were expanded so workflow building is easier to navigate.
- **Configured Delivery and Registry Selectors**: Workflow communication, email, webhook, image pull, image push, and registry search nodes can reuse saved channels, webhooks, registries, SMTP settings, encrypted credentials, HMAC-signed webhooks, and custom fallback modes.
- **Docker Event Triggers**: Trigger workflows from Docker event streams.
- **Container Health Triggers**: Start workflows when watched container health changes.
- **Registry Tag Triggers**: Watch registry tag digest changes and trigger automation when images change upstream.
- **Environment Status Checks**: Add environment health/status checks to workflow logic.
- **Approval Gates**: Pause workflows for manual approval before continuing sensitive actions.
- **Image Vulnerability Scan Nodes**: Run vulnerability scan steps inside workflows.
- **Stack Backup Nodes**: Back up stack definitions as part of automation flows.
- **Docker Volume Backup and Restore Nodes**: Automate Docker volume backup and restore tasks from the workflow editor.
- **Automatic Trigger Handling**: Docker events, container health changes, and registry tag digest changes can now launch matching workflows automatically.
- **Localized Node Metadata**: Built-in workflow nodes include metadata translations for English, Dutch, German, Spanish, French, and Portuguese.

### App Store and Catalog

- **File-Based App Catalog**: Bundled app templates now live as individual JSON files under `apps/`, making the catalog easier to extend and maintain.
- **Expanded App Templates**: The catalog includes additional templates for popular web apps and WhatsApp gateway deployments.
- **Homelab and Automation Apps**: Added catalog entries for Piper, Speech-to-Phrase, Frigate, Homebridge, go2rtc, Scrypted, InfluxDB, EMQX, EVCC, rtl_433, Ring-MQTT, AppDaemon, and Technitium DNS Server.
- **Compose Overrides**: Catalog entries can include custom command blocks and richer generated Compose output for database-backed templates and specialized deployments.

### Updates, Self-Management, and Release Safety

- **Reliable Self-Update Helper**: Production self-update and self-reinstall flows recreate the running McHarbor container through a detached helper with durable logs, rollback behavior, and replacement-container recovery.
- **Self-Update Watchdog**: McHarbor starts a watchdog before destructive self-update steps so replacement containers can be started if Docker creates them but leaves them stopped.
- **Improved Self-Detection**: McHarbor recognizes its own container through Docker labels, compose metadata, image references, stored compose data, conventional names, and Docker inspect fallbacks.
- **Release Version Metadata**: Runtime health/about metadata, update checks, startup logging, OpenAPI metadata, frontend package metadata, Docker image labels, and publishing workflows now use the canonical root `VERSION` file.
- **Agent Install Commands**: Generated agent Docker install commands use the latest agent image by default, pull before restart, and mount a stable Compose workspace for agent-side stack deploys.
- **Self-Update Checker**: McHarbor polls GitHub releases on a configurable interval (default 60 minutes, settable from Settings) and surfaces an in-app banner plus a notification whenever a newer version is available. The check is a coordinator singleton on multi-node installs, so only the leader polls; the result is replicated to followers through the shared database.
- **Update Status API**: `GET /api/updates/status` returns `{ current, latest, available, lastCheckedAt, releaseUrl }`. Operators can trigger an immediate check via `POST /api/updates/check` and adjust the interval via `PATCH /api/updates/settings`. The endpoint is auth-protected.

### Backups and External Storage

- **Storage Location Registry**: Add reusable backup/export destinations from Settings, including FTP/FTPS, SFTP, Samba, AWS S3, Google Drive, OneDrive Personal, OneDrive Business, and SharePoint.
- **Encrypted Container Backups**: Generate a one-time backup encryption key from Settings, copy it once, then use **Install and restart** to create `secrets/mcharbor_backup_key` and restart McHarbor with `docker-compose.secrets.yml`. The generated PowerShell setup command remains available as a manual fallback, and encrypted backup runs are written as `mcharbor.tar`. **Running outside Docker Compose?** If you started McHarbor with plain `docker run` (or any runtime other than Docker Compose), the `com.docker.compose.project.working_dir` label is missing and the install returns `err.settings.compose_project_missing`. Open the key dialog, click **Advanced (non-Compose install)**, and enter the absolute host path to your `docker-compose.yml` directory (e.g. `Z:\projects\web\management\McHarbor` on Docker Desktop, `/opt/mcharbor` on Linux). The install proceeds against that path instead.

#### Installing the Backup Encryption Key Outside Docker Compose

By default McHarbor reads the `com.docker.compose.project.working_dir` label to discover the host directory where `secrets/mcharbor_backup_key` should be written. That label is only set by `docker compose up`, so:

- **Plain `docker run`**: Click the **Advanced (non-Compose install)** toggle inside the "Enter backup encryption key" dialog and supply the host path to your `docker-compose.yml` directory.
- **Kubernetes / Podman / other**: same — use the Advanced toggle, or migrate the workload to `docker compose` so the label is present.

The full path on the host is required, including the drive letter on Windows (e.g. `C:\mcharbor` or `\mnt\McHarbor`). The dialog's hint text and `storage.installProjectPathHint` translation cover the supported formats.
- **Selectable Restores**: Restore workflows can preview saved backup contents and choose which image, filesystem, or mounted-data entries should be restored.
- **Restore Progress**: Filesystem and mounted-data restores report stage and byte progress, including remote-agent restores that are pulled and applied by the agent.
- **Protocol-Aware FTP Setup**: Configure FTP and FTPS from one provider choice, then select plain FTP, explicit FTPS, or implicit FTPS with the correct port and firewall guidance.
- **SFTP Key Authentication**: Store SFTP password, private-key, or combined authentication settings with encrypted private keys and passphrases.
- **Encrypted Storage Credentials**: Storage passwords, SSH keys, FTPS certificates, OAuth client secrets, and provider tokens are encrypted at rest and omitted from read responses.

## Built For

- DevOps engineers and sysadmins
- Homelab operators
- Small teams running Docker in production
- Developers who want a cleaner self-hosted alternative to Portainer

## Licensing

- Free for personal, homelab, and other non-commercial use
- Commercial license required for business or revenue-generating environments
- See [LICENSE](LICENSE) for the full terms

## Quick Start

Needs Docker Engine 20.10+ with the Compose v2 plugin — see [Installing Docker Engine](.documentation/getting-started/installation/docker-engine.md) for the procedure on Debian, Ubuntu, Fedora, RHEL/Rocky/Alma, openSUSE and Arch.

```bash
docker compose pull
docker compose up -d
```

The app is served over HTTPS on host port `8705`. Open `https://<your-server-ip>:8705` and accept the self-signed certificate on first use.

The default production compose file pulls `ghcr.io/therealmcsparrow/mcharbor:latest`. To pin a release, set `MCHARBOR_TAG` before starting Compose. To run the optional remote agent from Compose, set `MCHARBOR_URL` and `MCHARBOR_AGENT_TOKEN` and start the `agent` profile:

```bash
docker compose --profile agent up -d
```

## Install From The Command Line

McHarbor needs access to the host Docker socket. Without the `/var/run/docker.sock` bind mount, container management, events, and health checks against the local Docker environment will fail.

> **Note on non-Compose installs:** Both the `docker compose` and the plain `docker run` examples below work for day-to-day operations, but the **encrypted container backup** install flow needs the `com.docker.compose.project.working_dir` Docker label (only set by `docker compose up`). If you start McHarbor with the plain `docker run` example and later generate a backup encryption key, use the **Advanced (non-Compose install)** toggle in the key dialog to supply the absolute host path to your `docker-compose.yml` directory. The full instructions live in [Installing the Backup Encryption Key Outside Docker Compose](#installing-the-backup-encryption-key-outside-docker-compose).

Run McHarbor directly with Docker:

```bash
docker run -d \
  --name mcharbor \
  --restart unless-stopped \
  -p 8705:8705 \
  -e TLS_ENABLED=true \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v mcharbor-data:/app/data \
  ghcr.io/therealmcsparrow/mcharbor:latest
```

Or use Docker Compose:

```yaml
services:
  mcharbor:
    image: ghcr.io/therealmcsparrow/mcharbor:latest
    container_name: mcharbor
    restart: unless-stopped
    ports:
      - "8705:8705"   # HTTPS
    environment:
      - TLS_ENABLED=true
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - mcharbor-data:/app/data

volumes:
  mcharbor-data:
```

Start it with:

```bash
docker compose up -d
```

Then open:

```text
https://<your-server-ip>:8705
```

McHarbor is **HTTPS by default**. On first boot the container generates a self-signed certificate into the `data/tls/` volume so the UI, the API, and the agent WebSocket are all encrypted out of the box. Browse to the URL above and accept the browser warning for self-signed certificates to proceed. For production, upload your own certificate under Settings → HTTPS, or use the bundled [`apps/acme-companion.json`](apps/acme-companion.json) reverse-proxy automation to issue a Let's Encrypt cert automatically.

If you uploaded a certificate that turned out to be broken, the plain-HTTP listener on `:8705` (`HTTP_LISTEN_ADDR`) still serves the full app — this is the escape hatch to reach Settings and replace the certificate. Set `HTTP_LISTEN_ADDR=` to disable it, and optionally set `HTTP_REDIRECT_ADDR=:8080` for a redirect-only listener that 308s every request to HTTPS.

## Native Linux Install

**The container above remains the recommended way to run McHarbor.** For hosts that should not run a container runtime at all — a dedicated Docker control plane, or an environment with a policy against privileged containers or a bind-mounted Docker socket — McHarbor also installs natively as a `.deb`, `.rpm`, or a tarball, running under systemd as root by default, or as a sandboxed `mcharbor` service account with `--no-run-as-root`:

```bash
# Debian / Ubuntu
sudo dpkg -i mcharbor_2026.10.01_amd64.deb

# Fedora
sudo rpm -i mcharbor-2026.10.01-1.x86_64.rpm


# Any supported distribution, no package manager
tar -xzf mcharbor-2026.10.01-linux-amd64.tar.gz && sudo ./install.sh
```

The tarball is about 65 MB: plugins are not bundled, and the Plugin Store downloads them from the catalog.

Supported distributions are Debian 12+, Ubuntu 24.04+, and Fedora 38+ (glibc 2.36 or newer). Ubuntu 22.04, Ubuntu 20.04, Debian 11, and Rocky/RHEL/Alma 9 are **not supported** and fail at service start with `GLIBC_2.36 not found`, because the bundled plugins link dynamically against the libc that built them. Check your host first:

```bash
ldd --version | head -1
```

Whichever account runs it, McHarbor controls the Docker daemon, which is root-equivalent on any host running one — the same trust boundary the container's socket mount already has. Root is the default; `--no-run-as-root` adds a filesystem sandbox that keeps a bug inside McHarbor to `/var/lib/mcharbor`. See [Native Linux install](.documentation/getting-started/installation/native-linux.md) for supported distributions in full, the filesystem layout, upgrading, uninstalling (including an `rpm -e` asymmetry worth knowing about before you rely on it), and how plugins reach a native install.

## API Documentation

McHarbor exposes its machine-readable OpenAPI document at:

```text
GET /api/docs/
```

Most API routes return McHarbor's shared response envelope:

```json
{ "success": true, "data": {} }
```

Authenticated clients can use the browser session cookie or a Bearer API key. Docker, agent, and Kubernetes runtime routes select the target environment with `?env=<environmentId>`.

Version metadata is available from:

```text
GET /api/about
GET /api/versions
```

Self-update checks are exposed through:

```text
GET    /api/updates/status          # current version, latest release, last check time
POST   /api/updates/check           # force an immediate refresh
PATCH  /api/updates/settings        # adjust the check interval
```

Container backup restores use:

```text
POST /api/container-backups/runs/{runId}/restore-options
POST /api/container-backups/runs/{runId}/restore
```

Container moves use:

```text
POST /api/containers/{id}/move/plan?env=<sourceEnvId>
POST /api/containers/{id}/move?env=<sourceEnvId>
POST /api/containers/{id}/move/stream?env=<sourceEnvId>
```

Internal transfer routes under `/api/containers/internal/*` and `/api/container-backups/internal/*` are token-only, one-use transport endpoints for agents. They are not general client APIs.

To install the optional remote agent on another machine:

```bash
docker pull ghcr.io/therealmcsparrow/mcharbor-agent:latest
docker rm -f mcharbor-agent 2>/dev/null || true
mkdir -p /var/lib/mcharbor-agent/compose
docker run -d \
  --name mcharbor-agent \
  --restart unless-stopped \
  -e MCHARBOR_URL=wss://your-mcharbor-domain:8705 \
  -e MCHARBOR_FORCE_TLS=1 \
  -e MCHARBOR_AGENT_TOKEN=your_agent_token \
  -e DOCKER_HOST=unix:///var/run/docker.sock \
  -e MCHARBOR_COMPOSE_DIR=/var/lib/mcharbor-agent/compose \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /var/lib/mcharbor-agent/compose:/var/lib/mcharbor-agent/compose \
  -v /var/lib/mcharbor-agent:/var/lib/mcharbor-agent \
  ghcr.io/therealmcsparrow/mcharbor-agent:latest
```

While the agent cannot reach McHarbor it keeps working on its own: Docker events, per-container stats and its own log lines go into a local SQLite buffer at `/var/lib/mcharbor-agent/offline.db`. On the next successful connection the agent replays the buffer in acknowledged batches into the same activity, lifecycle and metrics tables the online collectors write, plus an **Agent Log** card on the environment page, then clears it. Mount the data directory (as above) so the buffer survives a container recreate. Tunables: `MCHARBOR_OFFLINE_BUFFER` (default `true`), `MCHARBOR_DATA_DIR` (default `/var/lib/mcharbor-agent`), `MCHARBOR_OFFLINE_METRICS_INTERVAL` (seconds, default `30`).

## AI

McHarbor uses AI-assisted tooling in a few parts of the project. AI helps us move faster, but it does not replace coding, review, testing, or operational judgment.

We currently use AI assistance for:

- **Translations**: Interface translations may be generated or refined with AI so McHarbor can support more users across more languages. These translations are checked for missing keys and broken placeholders, but they still need human review for tone, technical wording, and regional accuracy.
- **Code review and sanity checks**: AI helps inspect changes for obvious bugs, missing edge cases, inconsistent naming, or risky patterns. Implementation remains in the hands of the maintainer.
- **Documentation**: AI helps draft or improve README content, API documentation, setup notes, changelogs, release notes, and user-facing explanations.
- **Developer workflow support**: AI helps summarize changes, generate test ideas, and keep repetitive project maintenance work consistent.

## Security

Report a suspected vulnerability to **security@mcsparrow.eu**, not in a public issue. [SECURITY.md](SECURITY.md) lists what to include, the response targets and which versions receive fixes.

## Support

If McHarbor is useful to you, you can support its development:

- **Ko-fi**: [ko-fi.com/mcsparrow](https://ko-fi.com/mcsparrow)
- **PayPal**: `info@mcsparrow.eu`
