# Deployment and Runtime

McHarbor is designed to run as a self-hosted containerized application, with the
backend serving the built frontend in production.

## Local Development

### Frontend

```bash
cd src/frontend
npm install
npm run dev
```

Default dev URL:

- `http://localhost:8173`

### Backend

```bash
cd src/backend
go mod tidy
go run ./main.go
```

Default backend URL:

- `http://localhost:5474`

## Docker Compose Runtime

The root `docker-compose.yml` defines one main application service:

- service name: `mcharbor`
- published port: `8705:5474`
- Docker socket mounted read-only
- persistent volume mounted at `/app/data`

Quick start:

```bash
docker compose build
docker compose up -d
```

Default end-user URL:

- `http://localhost:8705`

## Production Image Build

The production container in `docker/Dockerfile` is a three-stage build:

1. Go backend build stage
2. Node frontend build stage
3. Alpine runtime stage

The final image includes:

- `mcharbor` backend binary
- built frontend assets in `/app/static`
- bundled blueprints in `/app/blueprints`
- Docker CLI and Compose plugin
- SQLite
- curl, bash, CA certs, timezone data
- Trivy and Grype scanners

## Runtime Modes

### HTTP

The app serves plain HTTP when TLS is not enabled or certificate files are unavailable.

### HTTPS

TLS can be enabled through DB-backed settings, with certificate files expected under:

- `<DATA_DIR>/tls/cert.pem`
- `<DATA_DIR>/tls/key.pem`

When enabled and certificates exist:

- the server listens with TLS
- optional HTTPS redirection middleware can be applied

## Background Runtime Tasks

The normal runtime includes several background components:

- metrics collector
- activity collector
- alerts engine
- workflow trigger service
- environment automation
- audit retention pruning
- agent heartbeat loop

## Persistent Data

The application expects persistent storage at the data directory, including:

- SQLite database
- encrypted secrets and supporting runtime data
- widget and custom-node storage
- TLS files

The default container path is:

- `/app/data`
