# Backend Architecture

The backend is a Go application composed around a shared dependency container and
feature modules mounted onto a chi router.

## Startup Flow

The backend startup sequence in `src/backend/main.go` is:

1. Load environment configuration.
2. Initialize structured logging.
3. Open SQLite.
4. Run embedded SQL migrations.
5. Initialize encryption.
6. Initialize authentication service.
7. Initialize the agent pool.
8. Initialize Docker and Kubernetes client pools.
9. Start background services:
   - metrics collector
   - activity collector
   - alerts engine
   - environment automation loop
   - audit pruning loop
   - agent ping loop
10. Mount API modules.
11. Start the workflow trigger service.
12. Serve HTTP or HTTPS depending on DB-backed TLS settings and certificate presence.

## Routing Model

The backend router in `src/backend/core/router/router.go` uses:

- recovery middleware
- request logging
- security headers
- CORS
- i18n language middleware
- auth-route rate limiting
- API key middleware for protected routes
- session auth middleware for protected routes

The API lives under `/api`.

### Route categories

- Public health route
- auth routes mounted without auth middleware
- protected routes mounted behind API key and session auth
- static SPA file serving for production builds

## Shared Services

Key backend core packages include:

- `core/config`: env-based configuration loading
- `core/db`: SQLite open/migrate helpers
- `core/auth`: user, password, and session management
- `core/rbac`: permissions and API key middleware
- `core/docker`: Docker connection and client pooling
- `core/kubernetes`: Kubernetes client pooling
- `core/agent`: WebSocket-based transport for remote Docker access
- `core/encryption`: AES-GCM encryption for stored secrets
- `core/response`: standard API response helpers
- `core/i18n`: message codes and language handling

## Module Pattern

Most backend features live under `src/backend/modules/<name>/`.

A typical module contains:

- `routes.go`: route registration
- `handler.go`: request parsing and response handling
- `service.go`: business logic
- `model.go`: request and response structures

Thin stream-style modules like logs, events, terminal, health, or openapi may not
need a heavy service layer.

## Mounted Modules

The application mounts a broad set of modules from `main.go`, including:

- auth and identity
- Docker runtime management
- Kubernetes workload management
- dashboard and metrics
- activity, audit, alerts, scans, updates, plugins
- blueprints, Git, reconciler, workflows, widgets, app store
- users, roles, groups, API keys, notifications, settings

## Background Services

Several long-running loops are part of normal backend runtime:

- metrics collector
- activity collector
- alerts engine
- workflow trigger service
- environment automation loop
- agent heartbeat / ping loop
- audit retention pruning

## Backend Build Output

The backend build produces the `mcharbor` binary. In containerized production,
the binary is copied into the final image and serves both the API and the built SPA.
