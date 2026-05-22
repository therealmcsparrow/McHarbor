# Application Overview

McHarbor is a self-hosted control plane for Docker and Kubernetes environments.
It combines runtime management, remote access, observability, workflow automation,
dashboarding, notifications, and governance into a single application.

## What Runs Where

### Backend

- Language: Go
- Default internal port: `5474`
- Responsibilities:
  - HTTP API
  - auth and session management
  - RBAC and audit logging
  - Docker and Kubernetes client pools
  - workflow execution
  - agent transport
  - notifications, alerts, scans, widgets, and other modules

### Frontend

- Framework: React 19 + Vite
- Dev port: `8173`
- Responsibilities:
  - authenticated SPA
  - dashboards
  - Docker and Kubernetes management UI
  - workflow editor
  - settings, notifications, and security screens

### Remote Agent

- Language: Go
- Runs separately on remote hosts
- Opens an outbound WebSocket connection to McHarbor
- Enables Docker management behind NAT or firewalls without exposing the Docker daemon directly

### Database

- SQLite
- Default path: `./data/mcharbor.db`
- Embedded and file-based

## Main Runtime Model

1. The backend starts, loads env-based config, opens SQLite, runs migrations, and initializes shared services.
2. Module routes are mounted under `/api`.
3. The frontend SPA is served by the backend from `./static` in production.
4. Users authenticate with a session cookie or API key.
5. Runtime operations are directed at selected environments, usually through `?env=<environmentId>`.
6. Optional agents connect back to the server through `/api/agent/ws`.

## Main Subsystems

- Docker operations: containers, images, volumes, networks, stacks, terminal, logs, events
- Kubernetes operations: pods, deployments, services, namespaces
- Automation: workflows, custom nodes, schedules, webhooks, blueprints, reconciler, Git
- Operations: dashboard widgets, alerts, notifications, activity, audit, scans, updates
- Access and governance: users, roles, groups, API keys, identity providers, settings

## Entry Points

- Backend entry point: `src/backend/main.go`
- Backend router: `src/backend/core/router/router.go`
- Frontend entry point: `src/frontend/main.tsx`
- Frontend route map: `src/frontend/core/router.tsx`
- Agent connection logic: `src/agent/agent.go`

## Related Docs

- [Backend Architecture](./backend.md)
- [Frontend Architecture](./frontend.md)
- [Deployment and Runtime](./deployment-and-runtime.md)
- [Configuration and Data](./configuration-and-data.md)
