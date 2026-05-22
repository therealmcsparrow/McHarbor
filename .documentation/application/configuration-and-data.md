# Configuration and Data

This document covers runtime configuration, important environment variables, and
the main persistent data model.

## Environment Configuration

The backend configuration loader is in:

- `src/backend/core/config/config.go`

## Important Environment Variables

### Core app

- `PORT`
- `HOST`
- `MCHARBOR_SECRET`

### Database and data

- `DATABASE_PATH`
- `DATA_DIR`
- `ENCRYPTION_KEY`

### Docker and Kubernetes

- `DOCKER_HOST`
- `DOCKER_TLS_VERIFY`
- `DOCKER_CERT_PATH`
- `KUBECONFIG`

### Auth and cookies

- `AUTH_DISABLE`
- `FORCE_SECURE_COOKIES`

### Logging

- `LOG_LEVEL`
- `LOG_JSON`

### CORS

- `ALLOWED_ORIGINS`

### App store

- `APPSTORE_CATALOG_URL`
- `APPSTORE_SYNC_CRON`

## Default Paths

Defaults from config:

- database: `./data/mcharbor.db`
- data directory: `./data`

Container runtime paths:

- application working directory: `/app`
- frontend static assets: `/app/static`
- runtime data volume: `/app/data`

## Database Migrations

Migrations are embedded and applied at startup through:

- `src/backend/core/db/migrate.go`

The migration runner:

- creates the `_migrations` table if needed
- reads embedded `migrations/*.sql`
- sorts them by filename
- applies only unapplied migrations
- records success in the migrations table

## Persistent Data Categories

Examples of persisted application data:

- users, sessions, roles, groups, permissions
- environments and connection metadata
- stacks, workflows, runs, schedule data
- settings
- audit and activity history
- notifications and alerts
- scan and update state

Examples of filesystem-backed data:

- SQLite database file
- custom node definitions
- widget definitions
- TLS certificates
- encryption support files

## Static Assets and Website

Two separate documentation delivery models exist in the repo:

- production SPA assets served from the backend static directory
- standalone marketing website under `website/`

The standalone website is not the main application UI. It is a separate static site.
