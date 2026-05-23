# McHarbor

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

The default production compose file pulls `ghcr.io/therealmcsparrow/mcharbor:1.1.7`. To run the optional remote agent from Compose, set `MCHARBOR_URL` and `MCHARBOR_AGENT_TOKEN` and start the `agent` profile:

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
docker run -d \
  --name mcharbor-agent \
  --restart unless-stopped \
  -e MCHARBOR_URL=wss://your-mcharbor-domain:8705 \
  -e MCHARBOR_AGENT_TOKEN=your_agent_token \
  -e DOCKER_HOST=unix:///var/run/docker.sock \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/therealmcsparrow/mcharbor-agent:latest
```

If you see an error like `Cannot connect to the Docker daemon at unix:///var/run/docker.sock`, check that Docker is running on the host and that the socket mount is present.

Note: `ghcr.io/therealmcsparrow/mcharbor:latest` and `ghcr.io/therealmcsparrow/mcharbor-agent:latest` require the GitHub Container Registry publish step to succeed before these commands will pull successfully.
