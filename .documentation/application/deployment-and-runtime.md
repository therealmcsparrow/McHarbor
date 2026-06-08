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

### Backup Encryption Secret

Encrypted container backups require a 32-byte key mounted as a Docker secret.
Generate the key from Settings > Storage, copy it once, and store the copied
value in a secret file outside source control:

```bash
mkdir -p secrets
printf '%s' '<copied-base64-key>' > secrets/mcharbor_backup_key
```

Run with the base Compose file plus the secret override:

```bash
docker compose -f docker-compose.yml -f docker-compose.secrets.yml up -d
```

The override mounts the secret as `/run/secrets/mcharbor_backup_key` and sets
`BACKUP_ENCRYPTION_KEY_FILE` accordingly. Use `MCHARBOR_BACKUP_KEY_FILE` to point
Compose at a different host-side secret file.

## Remote Agent Runtime

The remote agent can run as a container or a binary on a Docker host. Generated
install and deploy flows use the latest agent image by default:

```text
ghcr.io/therealmcsparrow/mcharbor-agent:latest
```

Minimum versions for selected capabilities:

| Capability | Minimum agent |
| --- | --- |
| Interactive terminal exec through the agent protocol | `1.1.0` |
| Staged Docker image-load support for remote container moves | `1.3.3` |
| Direct agent-to-agent image transfer for container moves | `1.3.5` |
| Settings direct-transfer reachability test | `1.3.7` |
| Agent-side Docker Compose stack execution | `1.4.0` |

Direct transfer is optional and must be configured on the target agent:

| Variable | Purpose |
| --- | --- |
| `MCHARBOR_TRANSFER_LISTEN` | TCP listen address for the target agent upload receiver. |
| `MCHARBOR_TRANSFER_ADVERTISE_URL` | URL reachable by the source agent during direct transfers. |

Example:

```bash
docker run -d \
  --name mcharbor-agent \
  --restart unless-stopped \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /var/lib/mcharbor-agent/compose:/var/lib/mcharbor-agent/compose \
  -p 8788:8788 \
  -e MCHARBOR_URL=http://mcharbor.example:8705 \
  -e MCHARBOR_AGENT_TOKEN=agt_xxx \
  -e DOCKER_HOST=unix:///var/run/docker.sock \
  -e MCHARBOR_COMPOSE_DIR=/var/lib/mcharbor-agent/compose \
  -e MCHARBOR_TRANSFER_LISTEN=0.0.0.0:8788 \
  -e MCHARBOR_TRANSFER_ADVERTISE_URL=http://target-host:8788 \
  ghcr.io/therealmcsparrow/mcharbor-agent:latest
```

When direct transfer is not configured or the source agent cannot reach the
advertised URL, container moves continue through the McHarbor relay path.
Settings > Agent includes a direct-transfer test that validates this source to
target route without loading a Docker image.

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
