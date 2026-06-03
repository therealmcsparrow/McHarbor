# McHarbor

**!! IMPORTANT !!**

There was a problem with self-updating. This is fixed from version 1.1.13. I am so sorry for any inconvenience it may cause you.
If you have a version below 1.1.13 you need it to update or reinstall it by CLI or with another docker manager.

**!! IMPORTANT !!**

Your containers. Your clusters. Your rules.

McHarbor is a self-hosted control plane for Docker and Kubernetes environments. It brings enterprise-level functionality, container management, remote access, live operational visibility, dashboards, workflow automation, and extensibility into one platform that you run yourself.

Deploy it as a Docker container, connect it to the host Docker socket, and start managing infrastructure from one clean, dark-first interface. Add remote Docker hosts through the outbound McHarbor agent, manage Kubernetes workloads beside Docker environments, stream logs, open browser terminals, build dashboards, automate routine work, and keep a clear record of operational changes without handing your infrastructure to a third party. McHarbor is free for personal and homelab use. Commercial and business use requires a commercial license.

## Preview

<video src="images/McHarbor.mp4" controls muted playsinline preload="metadata"></video>

[Open the product walkthrough video](images/McHarbor.mp4)

## Why McHarbor

McHarbor is built for the work that happens after deployment.

- Inspect, start, stop, restart, and remove containers from a UI built for real operations.
- Manage images, volumes, networks, and Compose stacks without bouncing between point tools.
- Operate pods, deployments, services, and namespaces from the same platform you use for Docker.
- Reach remote Docker hosts behind NAT and firewalls through an outbound agent over WebSocket.
- Follow live logs and infrastructure events, then drop into a browser terminal when you need shell access.
- Build custom dashboards and workflow automations instead of relying on a fixed operational surface.
- Keep activity, alerts, and audit history visible as your environment grows.
- Use the same platform from a Raspberry Pi homelab to enterprise operations without changing tools.

## Named Features

- **Self-Hosted Deployment**: Run McHarbor on your own infrastructure as a Docker container and keep your environments, credentials, and operational data under your control.
- **Embedded SQLite Database**: Use a zero-config, file-based database that is easy to deploy, back up, and maintain.
- **Dark-First Interface**: Work in a modern UI designed for long operational sessions, with a light mode toggle available when you want it.
- **Multi-Environment Management**: Connect local Docker Desktop, remote Linux servers, Raspberry Pi systems, NAS devices, Proxmox-backed labs, and Kubernetes clusters in one interface.
- **Container Lifecycle Management**: Inspect, create, start, stop, restart, and remove containers from a workflow designed around day-to-day infrastructure work.
- **Image, Volume, and Network Management**: Review container artifacts, persistent storage, and service connectivity from one operational surface.
- **Compose Stack Management**: Treat Docker Compose applications as first-class stacks instead of loose groups of containers.
- **Kubernetes Workload Management**: Manage pods, deployments, services, and namespaces alongside your Docker environments.
- **Remote Agent Connectivity**: Use an outbound agent to manage Docker hosts behind NAT and firewalls without exposing the daemon directly.
- **Live Log Streaming**: Follow log output in real time when you need answers quickly during debugging or incident response.
- **Live Event Streaming**: Watch infrastructure events as they happen so restarts, pulls, failures, and runtime changes stay visible.
- **Browser Terminal Access**: Open terminal sessions directly from the UI for fast operational access when logs are not enough.
- **Custom Dashboard Widgets**: Assemble dashboards with widgets for host info, stack status, activity, resource summaries, charts, and workflow visibility.
- **Workflow Automation Engine**: Create visual automations with triggers, conditions, actions, schedules, and utility nodes.
- **Custom Workflow Nodes**: Extend the workflow engine with sandboxed JavaScript nodes for internal tooling and third-party integrations.
- **Webhook and Scheduled Triggers**: Start automations from HTTP events or recurring schedules for repeatable maintenance and response flows.
- **Blueprints and Git Integration**: Standardize repeatable patterns and connect operational workflows to versioned configuration.
- **Registry Integration**: Keep registry-related workflows in the same control plane instead of treating image distribution as a separate system.
- **Notifications and Alerts**: Surface important activity and failures where operators can react quickly.
- **Activity and Audit History**: Keep a clearer record of what changed, who changed it, and when it happened.
- **User Management and Pluggable Auth**: Start with local auth today and keep a path open for broader identity integration over time.
- **Plugin-Friendly Architecture**: Grow the platform with custom nodes, plugins, and extension points instead of being locked into a fixed feature list.

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

```bash
docker compose pull
docker compose up -d
```

The app is served on port `8705` by default, mapped to backend port `5474` inside the container.

The default production compose file pulls `ghcr.io/therealmcsparrow/mcharbor:latest`. To pin a release, set `MCHARBOR_TAG` before starting Compose. To run the optional remote agent from Compose, set `MCHARBOR_URL` and `MCHARBOR_AGENT_TOKEN` and start the `agent` profile:

```bash
docker compose --profile agent up -d
```

## Install From The Command Line

McHarbor needs access to the host Docker socket. Without the `/var/run/docker.sock` bind mount, container management, events, and health checks against the local Docker environment will fail.

Run McHarbor directly with Docker:

```bash
docker run -d \
  --name mcharbor \
  --restart unless-stopped \
  -p 8705:5474 \
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
      - "8705:5474"
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
http://<your-server-ip>:8705
```

To install the optional remote agent on another machine:

```bash
docker pull ghcr.io/therealmcsparrow/mcharbor-agent:latest
docker rm -f mcharbor-agent 2>/dev/null || true
mkdir -p /var/lib/mcharbor-agent/compose
docker run -d \
  --name mcharbor-agent \
  --restart unless-stopped \
  -e MCHARBOR_URL=wss://your-mcharbor-domain:8705 \
  -e MCHARBOR_AGENT_TOKEN=your_agent_token \
  -e DOCKER_HOST=unix:///var/run/docker.sock \
  -e MCHARBOR_COMPOSE_DIR=/var/lib/mcharbor-agent/compose \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /var/lib/mcharbor-agent/compose:/var/lib/mcharbor-agent/compose \
  ghcr.io/therealmcsparrow/mcharbor-agent:latest
```

## AI

McHarbor uses AI-assisted tooling in a few parts of the project. AI helps us move faster, but it does not replace coding, review, testing, or operational judgment.

We currently use AI assistance for:

- **Translations**: Interface translations may be generated or refined with AI so McHarbor can support more users across more languages. These translations are checked for missing keys and broken placeholders, but they still need human review for tone, technical wording, and regional accuracy.
- **Code review and sanity checks**: AI helps inspect changes for obvious bugs, missing edge cases, inconsistent naming, or risky patterns. Implementation remains in the hands of the maintainer.
- **Documentation**: AI helps draft or improve README content, API documentation, setup notes, changelogs, release notes, and user-facing explanations.
- **Developer workflow support**: AI helps summarize changes, generate test ideas, and keep repetitive project maintenance work consistent.
