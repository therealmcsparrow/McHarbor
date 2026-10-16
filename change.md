# McHarbor 2026.10.01 — changes since 2.0.8

This compares McHarbor 2026.10.01 with the 2.0.8 release of 2026-07-26. It lists what an
operator coming from 2.0.8 gets, loses or must act on. Fixes to features that did not exist in
2.0.8 are part of those features here, not separate entries. `CHANGELOG.md` keeps the full
record, including the upgrade steps behind each **Breaking** entry.

## Added

### Plugins

- **A plugin platform.** Plugins ship a Go backend, a React frontend, migrations, i18n bundles,
  dashboard widgets, workflow nodes, report types, alert sources and CLI commands, and hook into
  the sidebar, topbar, menus and page tabs. They are signed tarballs installed from the Plugin
  Store, which reads the McHarbor catalog or catalogs you add. 68 first-party plugins ship with
  this release.
- **A Plugin Store** with categories, plugin dependencies installed and removed automatically,
  an explicit consent step for unsigned plugins, **Update all**, and a listing of the nodes and
  widgets a plugin contributes before it is installed.
- **Plugins repair themselves after an upgrade.** The first boot of a new release reinstalls
  each plugin built for the release, keeping its configuration, enabled state and role grants.
  A plugin that cannot be fetched is disabled and marked **Needs repair** instead of failing
  the upgrade.
- **Plugins declare what host pages need from them** under `contributes.capabilities`: runbook
  steps, picker lists, search sources and palette actions, estate counts on environment cards,
  email channels, document export, lifecycle subjects and directory sign-in. Each surface
  disappears cleanly when its plugin is removed.
- **Plugin commands in the CLI.** Six roots (`kvm`, `lxc`, `compute`, `swarm`, `k8s`, `report`)
  reach every plugin object from a terminal, under the same confirmation rules and permission
  keys as the interface.

### Compute: virtual machines and system containers

- **Three compute plugins.** Compute Foundation (`addon-compute`) owns storage pools (directory,
  ZFS, btrfs, LVM), host networks (bridged, NAT, external and macvlan), guest images and a
  per-host capability probe. Virtual Machines (`addon-kvm`) runs KVM guests and System
  Containers (`addon-lxc`) runs full-system LXC guests. The remote agent supervises them
  directly through QEMU/QMP, libcontainer, netlink and cgroup v2, with no libvirt or LXD in the
  path.
- **Virtual machines:**
  - **Consoles:** VNC and SPICE consoles, with SPICE display profiles and a `.vv` file for
    `remote-viewer`. Single-use console tickets, power controls beside the console, and guest
    resize to the window.
  - **Snapshots, clones and templates:** snapshots shown as a tree, filesystem-consistent
    through the guest agent. Full and linked clones, and templates you create machines from.
  - **Hardware:** changes on a running machine (disks, discs, network interfaces, resizable
    memory, processor hotplug), plus hugepages, NUMA placement, per-disk I/O limits, host
    directory shares over virtiofs, a virtual security key, and PCI/USB passthrough.
  - **Migration:** cold and live migration between hosts.
  - **Imports and disk tools:** third-party disk imports that run on the host, and disk
    diagnostics through `qemu-img`.
  - **Backups:** scheduled, encrypted and incremental, with retention that never breaks a
    restore chain.
  - **Visibility:** a Logs tab, guest addresses and OS details from the guest agent, and a
    state of its own for a machine paused by a disk error or a guest panic. Metrics charts open
    with the last hour of stored samples.
- **System containers:**
  - **Day to day:** console and root shell, live statistics, and CPU, memory and process
    limits that apply without a restart.
  - **Copies:** snapshots and clones.
  - **Backups:** scheduled, encrypted and incremental, with recreation of a deleted container
    from its backup.
- **A compute host page** with Overview, Settings, Tests and Logs tabs, configurable storage
  locations per host, a catalog of 49 guest images with distribution logos and an
  operating-system filter.

### Docker, stacks and environments

- **A secrets vault** (**Security → Secrets**). A value is stored once, encrypted, referenced
  as `${secret:NAME}` from stacks, containers and workflows, and substituted server-side. File
  mode delivers it as a read-only file under `/run/secrets` instead of an environment variable.
- **Containers:**
  - **Detail page:** a redesigned page with health and uptime graphs, GPU details and a
    configuration editor.
  - **Actions:** per-container **Recreate**, and a debug shell for distroless and scratch
    containers (`containers.debug_shell`).
  - **Devices and logs:** a host device picker for device mappings, and server-side log search
    with a timeline view.
- **Container backups:** a Backups overview page, a pause window, per-file restore and failure
  details on a failed run. Storage locations show their capacity.
- **An app catalog of 630 applications** with editable and remote trusted or untrusted catalogs
  and image-availability checks.
- **Agents:**
  - **Health and connection:** health and uptime history, and an Agent Log card. The agent
    pins McHarbor's root certificate (`MCHARBOR_PIN`) instead of skipping TLS verification, and
    the Environments list shows whether each agent's channel is encrypted.
  - **Offline buffering:** events, stats and logs are buffered while the agent is offline and
    replayed on reconnect.
  - **Stats:** the agent pushes its container stats in batches instead of being polled per
    container.
  - **Releases and installs:** an agent release store under **Settings → Updates → Agents**,
    and prerequisite checks and installs over SSH when you add an environment.
- **The host behind an environment:**
  - **Updates:** host OS updates with a schedule, streamed output and per-environment options
    (new packages, skip unavailable, remove unused).
  - **Services and logs:** systemd services management for native agents, and host log
    streaming.
  - **Details and editing:** the host's IP address is shown, and environments are editable
    after creation.
  - **Estate counts:** environment cards also show virtual machines, system containers and
    Swarm services.
- **McHarbor's own host** has its own keys (`host_self.view`, `host_self.manage`,
  `host_self.reboot`) and its own Host OS and Services pages under Settings.

### Automation

- **Runbooks.** Ordered steps attached to an alert, a container, a stack, an environment or a
  plugin object. Each step is an instruction to tick off, a read-only diagnostic, or an action
  that runs under the operator's own permissions. Runs record who did what and where they
  stopped. Runbooks can run unattended as service principals, can be exported as JSON, PDF or
  Word, and can take steps that plugins contribute.
- **Drift detection in the reconciler.** It watches containers, stacks, whole environments and
  plugin objects (virtual machines, system containers, Swarm, Kubernetes). It can observe,
  propose a reviewable plan or enforce within limits, and can take its desired state from Git.
  A drift report page, a dashboard widget and a **Desired state drifted** alert condition
  follow it.
- **Incidents** gather the evidence around a failing container, virtual machine, system
  container, pod or service into one timeline. They come with an optional AI narrative that you
  enable per environment, and detectors for memory growth, nightly restarts and failures after a
  deploy.
- **ITSM ticketing.** Alerts open, update and resolve tickets through a durable outbox, and the
  ITSM's status flows back. Plugins cover PagerDuty, Alertmanager-compatible receivers, Zammad,
  GLPI, iTop, OTRS/Znuny/OTOBO and Request Tracker, with matching workflow nodes and a trigger.
- **GitOps:**
  - **Promotions:** a promotion deploys the promoted commit's compose file as a stack.
  - **Previews:** pull-request preview environments with a TTL, a concurrency cap and reliable
    teardown.
  - **Webhook secrets:** managed in the interface and in the CLI.
- **Workflows:**
  - **New nodes:** container backup, host exec, SSH, SQL, FTP, GraphQL, parsers, rate limit,
    try/catch, metric triggers, file watch, and send-communication through any channel plugin.
  - **Plugin pick lists:** a node can offer a pick list of a plugin's own objects.
- **Alerts:** plugin alert sources, failed-login alerts, message templates with a variable
  reference and live preview, and new conditions for capacity, drift, instance exports and API
  key age. Schedules can be edited instead of only created and deleted.

### Observability and reporting

- **Prometheus metrics and OpenTelemetry tracing for McHarbor itself.** `GET /api/metrics`
  needs `metrics.scrape`, and a Grafana dashboard ships with it. Traces run through the agent to
  the Docker daemon, and there are database query and agent call timings.
- **Capacity forecasting and idle resources.** A dated forecast is given only when the data
  supports one, with a `capacity_days` alert condition.
- **Reporting (Reporting plugin).** A Report Studio with search, scheduled documents, and
  cross-platform facets (inventory, capacity, protection, health, exposure, cost) answered by
  Docker and each platform plugin. Built-in report types now cover incidents, drift, disaster
  recovery, HA, ITSM, approvals, secrets hygiene, workflows, webhooks, compliance and runbooks.
- **Dashboard:**
  - **Page head:** a live estate summary across every environment you may read.
  - **Widgets:** fleet health, needs attention, update queue, vulnerability summary, snapshot
    status, port map, restart tracker, storage breakdown, top consumers, container age and
    states, workload heatmap, alert summary, metric chart and more.
  - **Downloaded widgets:** signed widgets you install from a catalog.

### Access, security and governance

- **Role-based access control.** A permission catalog, roles and role presets, environment,
  stack and owner scopes, temporary elevations with approval, and an impact preview before a
  role change. Denied controls render disabled and name the missing key.
- **Attribute-based policies** layered on roles, for host and plugin resources. The editor has
  a reference panel and a policy simulator.
- **Governance:**
  - **Approvals:** approval rules for destructive actions.
  - **Emergency access:** break-glass access, behind the account password.
  - **Visibility:** an **Effective access** view, plus role templates, API key rotation and bulk
    user actions.
- **A tamper-evident audit log.** Hash-chained, write-once, signed exports, a verify action,
  per-severity retention, a Denials tab and an AI actions tab. Rows keep the names of entities
  that were later deleted and record the API key and node that made each change.
- **Accounts:**
  - **Sign-in security:** rebuilt MFA, an option to require it, a password policy, idle logout,
    and password reset by email.
  - **Directories:** SCIM provisioning with a dry run, a setup wizard for identity providers,
    and LDAP sign-in from the login page.
- **API keys** carry their own permission list and can be created by approving
  `mcharbor login --device` in the browser.
- **Compliance:**
  - **Evidence:** a compliance report (JSON, Word, PDF), and compliance packs that give
    evidence against the CIS Docker Benchmark with SOC 2 and ISO 27001 mappings.
  - **Attestations:** signed-off review records (`compliance.attest`).
- **Support:** a redacted support bundle, diagnostics, a state inventory, a pre-upgrade check
  and a security posture panel under **Settings → System → About**.
- **Licensing:** a licence key with a banner. Nothing is ever switched off.

### Data, backup and availability

- **Database backends beside SQLite:** PostgreSQL, MySQL/MariaDB and Microsoft SQL Server/Azure
  SQL. **Settings → System → Database** copies an install between backends, or to another file
  or server, and runs a database health check with safe remedies.
- **Instance backup and restore.** One encrypted `.mchx` file holds the whole control plane. It
  can be scheduled, copied to several storage locations and checked with restore rehearsals.
  Restore comes in three modes, and `mcharbor restore --local` works for an instance whose
  interface will not start.
- **High availability.** Active/passive with a database-held fence, a nodes page with
  **Make active**, **Drain** and **Resume**, and readiness checks. Active/active relays agent
  sessions between nodes. A shared plugin store, and `docker-compose.ha.yml` with secrets read
  from files.

### CLI, packaging and integrations

- **`mcharbor`, a command-line client** that covers environments, agents, containers (`exec`,
  `logs -f`), images, volumes, networks, stacks, backups, GitOps, users, roles, policies,
  audit, plugins, apps, settings and support. It has fixed exit codes, table, JSON, YAML and TSV
  output, and a `tui`. It downloads from your McHarbor and is built into the agent binary.
- **Native Linux packages** (`.deb`, `.rpm`, tarball) running under systemd, beside the
  container.
- **Terraform** resources for workflows and plugins, data sources for the inventory, a Helm
  chart, Kubernetes manifests, reverse-proxy examples and SBOMs with cosign signatures in the
  release workflow.

### Interface

- **An on-call view for phones** at `/oncall`, installable as an app, with Web Push
  notifications.
- **A command palette** that searches and acts across the host and installed plugins.
- **Table views saved per user:** columns, widths, alignment and named filters, with shipped
  default layouts and **Select all N matching**.
- **Custom branding** with four logo slots, theme plugins and packs, and Google Fonts.
- **Shared controls:** a shared dialog for a failed row's full error, a readable schedule
  builder for every schedule, and notification search and filters.
- **A rewritten documentation set** under `.documentation/`.
- **A What's new button** under **Settings → System → About** that reopens the release
  highlights.

## Changed

- **Breaking — capabilities moved into plugins.** Kubernetes, Docker Swarm, reporting, every
  external storage provider, every communication channel, every external identity provider,
  the vulnerability scanners, log forwarders, snapshot providers and AI providers are no longer
  in the core. Install the matching plugin from the Plugin Store to keep each one.
- **Breaking — removed.** Podman environments, blueprints (use the App Store and stack
  templates), the topbar help button and its help registry, the built-in storage providers and
  email-server settings, and seven routes nothing called.
- **Breaking — HTTPS by default.** A local certificate authority signs the server certificate
  on first boot, and the main listener serves TLS on `8735`; plain HTTP stays on `8705`. TLS 1.3
  is the default floor, and `TLS_MIN_VERSION=1.2` allows older clients.
- **Breaking — every mutating route needs a permission key,** and API keys carry an explicit
  permission list.
- **Breaking — McHarbor's own containers and host need `host_self.manage`.** Exec, terminals,
  file writes, lifecycle actions, recreate and remove on McHarbor's containers now need it.
  So do creating containers or stacks on McHarbor's environment with host-level access (host
  binds, the Docker socket, privileged options), and saving a workflow that acts on McHarbor's
  containers.
- **Breaking — credentials are masked unless revealed.** Container inspect, image inspect and
  history, stack compose text and env vars, and VM cloud-init return `••••••••` for
  credential-looking values. Revealing needs the `manage` key and is audited. Image export now
  needs `images.manage`.
- **Breaking — everyone is signed out once after upgrading,** because sessions are now stored
  hashed. API keys are unaffected.
- **Breaking — stack routes act on the stack's own environment,** whatever `?env=` says.
- **Breaking — compose runs no longer inherit McHarbor's environment.** They get an allow-list
  plus the stack's own variables.
- **Breaking — the Files tab protects McHarbor's data.** McHarbor's data directory, secrets and
  `/proc` need `system.manage`.
- **Breaking — forwarding headers count only from `MCHARBOR_TRUSTED_PROXY_CIDRS`.** Behind a
  reverse proxy, set it.
- **Breaking — exporting the audit log needs `audit.export`.**
- **Breaking — a fresh install needs a one-time setup token** from the container log before the
  first account can be created.
- **Breaking — agents install and update only from McHarbor,** never from GitHub or a registry
  directly.
- **Breaking — an unrecognised `MCHARBOR_DB_DRIVER` stops the start** instead of falling back
  to SQLite.
- **Breaking — the container image is Debian-based** (`debian:bookworm-slim`), and Alpine is
  not supported as a host.
- **No environment is created for you.** A fresh install starts with an empty Environments
  page instead of a detected `Local` environment.
- **Plugins come from the catalog, not the image.** The image shrinks from 3.4 GB to about
  760 MB.
- **Calendar versioning.** McHarbor, the agent and every plugin use `yyyy.MM.<sequential>`,
  with a separate `BUILD` stamp. The agent shares the app's version.
- **Environment metrics come from the host.** CPU and memory history is the host's own reading,
  and network and disk history is the host's total traffic and disk I/O over its physical
  interfaces and disks, in bytes per second. So the charts include virtual machines, Kubernetes
  and the host itself, and fill for a host with no containers. The Disk Usage tile is now
  **Disk I/O**. The local environment reads the host through a read-only `/proc` mount in
  `docker-compose.yml`, and an agent host needs the 2026.10.01 agent.
- **Live stats and events are shared per environment,** not taken once per open tab. Host and
  activity pages reuse slow figures briefly.
- **Self-update runs on a schedule and waits for work to finish.** **Settings → Updates →
  McHarbor** sets when to check and an install window. An install waits while a backup or
  restore runs. The hourly *Check interval* setting is gone.
- **A flatter interface.** Flat surfaces, full-width detail pages, a page head with icon, title
  and identity chips, and pinned tabs. Modals no longer close on an outside click, every number
  field has a stepper, and create actions are primary buttons. Settings and Security are
  reorganised; Themes is now **Customization**.
- **The environment installer** offers Socket or Agent connection types, and Manual or
  Automatic (SSH) deploys.
- **Workflows:**
  - **Send Email:** sends through an email-capable channel and honours its own "To".
  - **Image scans:** the scan node lists the scanners you actually have.
  - **Per-provider send nodes:** these now run as Send Message on the provider's plugin.
- **Container detail:** the Backup tabs are renamed **Backup** and **Backup list**, and detail
  tabs no longer drag to reorder.
- **Toolchain:** built with Go 1.27.1 and Node 26.

## Fixed

- **A container shell ends when its shell exits,** locally and through an agent.
- **The Terminal tab honours the chosen shell, user and working directory.**
- **A Docker error through a remote agent shows the daemon's own message,** not a JSON parse
  error.
- **Host logs arrive.** They timed out on every environment, and large log files are read from
  the end.
- **A missing volume, image or network answers 404,** not 500.
- **The current session is recognised** on the account routes, so "revoke other sessions" no
  longer revokes your own.
- **Changing an environment's stack owner changes one stack,** not every stack of that name.
- **Saving the self-update settings no longer stops the update checker.**
- **Metrics are no longer written for an environment that does not exist.**
- **Agent stability:** unexplained disconnects, a reconnect backoff that never reset, a
  host-terminal waiter that could hang, and noisy stats polling.
- **The binary agent install wrote a systemd unit without the binary,** because the download's
  `HEAD` probe was refused.
- **Failed Git syncs appear in the deployment history,** and a Git credential that cannot be
  decrypted is an error instead of being sent as the password.
- **A drift check no longer reports an unreachable host's containers as missing,** and a
  desired state with no environment can be edited.
- **Workflow conditions:** extra branches of a Condition node are evaluated, and the
  greater-than, less-than, greater-or-equal, less-or-equal and starts-with operators work.
- **App catalog placeholders expand inside environment values.**
- **TrueNAS storage authenticates.**
- **An encrypted backup upload no longer hangs** when the destination stops reading.
- **Audit entries are no longer lost** to `SQLITE_BUSY`.
- **The interface:**
  - **Tables:** a saved rows-per-page choice survives a reload.
  - **Menus and headers:** the user menu no longer paints behind sticky table headers, and a
    page's header actions no longer leak onto the next page.
  - **Tooltips:** icon-only buttons show their name, and native tooltips are replaced.
  - **Scrolling:** the Workflows, Custom Nodes, GitOps and Runbooks pages scroll.
- **Smaller fixes:**
  - **Sign-in:** first-admin setup, a login race and logout bounce-back on a fresh install.
  - **Stacks and images:** a stack list failure for a stack with no containers, and image ID
    unescaping in detail routes.
  - **Noise:** notifications about McHarbor's own container, and Docker event noise from
    health-check execs.
  - **Settings:** several settings and email-configuration save errors.

## Security

- **Sessions:**
  - **Storage:** sessions are stored as a SHA-256 hash, and session ids are 32 random bytes.
  - **Throttling and lockout:** login throttling works behind a reverse proxy, with a
    per-address lockout across usernames.
  - **Step-up:** step-up re-authentication is throttled and counts toward lockout.
- **Two-factor authentication can be required** for administrators or everyone.
- **Every mutating route is permission-gated,** permission denials are audited, and a grant on
  one environment cannot be used against another.
- **Credentials stay out of responses.**
  - **Masking:** credential-looking values are masked in container and image inspect, image
    history, stack compose and env vars, Swarm service env and VM cloud-init, with audited
    reveals.
  - **Connection details:** these go only to `environments.manage` holders.
  - **Channels:** fields a plugin marks secret are withheld.
- **McHarbor's own keys are protected.** `MCHARBOR_SECRET`, `ENCRYPTION_KEY`, `MCHARBOR_DB_DSN`
  and the backup key:
  - **Never shown:** they are never revealed through inspect, compose or the Files tab.
  - **Out of reach:** they are removed from McHarbor's process environment after start and
    kept out of compose runs and self-update helpers.
  - **From files:** each can be read from a file (`_FILE` variables), so it appears in no
    `docker inspect` output.
- **McHarbor's own containers and host are protected** from `containers.manage` holders. A
  container with access to the host on McHarbor's environment needs `host_self.manage`, and
  refusals are audited.
- **Encryption at rest everywhere:**
  - **Existing values:** values stored in plaintext before are encrypted on first boot, and
    writes fail rather than store plaintext.
  - **Credentials and secrets:** snapshot provider credentials are encrypted and no longer
    returned, and the secrets vault uses AES-256-GCM.
  - **Reconciler state:** desired states are encrypted, and proposals keep no live environment
    values.
- **Protected settings:** TLS keys and push keys can only be written through their own routes.
- **The GitOps pull-request webhook verifies an HMAC signature** over the raw body against a
  per-repository secret, and an approval is always recorded as the user who made it.
- **Reading workflows needs `workflows.view`.**
- **API keys need `account.self.manage`** on self-service account routes.
- **HTTPS by default with a local CA,** and agents pin it rather than skip verification.
- **A Content-Security-Policy** covers everything McHarbor serves.
- **A tamper-evident audit log:** hash-chained, write-once and signed on export.
- **Plugins:**
  - **Signatures:** plugins are signed and verified before install, and an unsigned plugin
    needs explicit consent.
  - **Routes and slugs:** every plugin route takes the permission its manifest declares, and
    the slug `host` is reserved.
- **McHarbor reports unsafe defaults at boot,** naming each variable that fixes them.
