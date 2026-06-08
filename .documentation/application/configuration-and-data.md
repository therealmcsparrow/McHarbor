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
- `BACKUP_ENCRYPTION_KEY_FILE`
- `MCHARBOR_BACKUP_KEY_FILE`

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
- storage locations for backups and exports
- audit and activity history
- notifications and alerts
- scan and update state

Examples of filesystem-backed data:

- SQLite database file
- custom node definitions
- widget definitions
- TLS certificates
- encryption support files
- encrypted container backup archives

## Storage Locations

Storage locations are persisted in SQLite and are used as reusable external
destinations for backups, exports, and storage workflows. The storage-location
API supports:

- FTP and FTPS through one UI choice, with `location_type` stored as `ftp` or `ftps`
- SFTP over SSH
- Samba/SMB shares
- AWS S3 or S3-compatible endpoints
- Google Drive, OneDrive Personal, OneDrive Business, and SharePoint via provider consent

Credential material is encrypted before it is written to the database. Read
responses return metadata such as host, port, path, tenant ID, and provider type,
but they do not return passwords, keys, certificates, OAuth tokens, or client
secrets.

### FTP, FTPS, and SFTP fields

| Protocol | Stored type | Default ports | Security model | Auth fields |
| --- | --- | --- | --- | --- |
| FTP | `ftp` | TCP 21 control, TCP 20 or passive data ports | No encryption; credentials and files are cleartext. | `username`, `password`, `passive_mode` |
| FTPS | `ftps` | TCP 21 explicit TLS or TCP 990 implicit TLS, plus passive data ports | FTP wrapped in SSL/TLS. | `username`, `password`, `tls_mode`, `passive_mode`, optional certificates |
| SFTP | `sftp` | TCP 22 | SSH file-transfer subsystem over a single encrypted connection. | `username`, `auth_method`, `password`, `private_key`, `passphrase` |

Additional encrypted columns added for transfer authentication:

- `private_key`
- `passphrase`
- `ca_certificate`
- `client_certificate`
- `client_key`

Non-secret transfer mode columns:

- `auth_method`
- `tls_mode`
- `passive_mode`

## Container Backup Encryption

Container backups use a separate Docker-secret-backed master key. Users generate
the key from Settings > Storage, copy it once, and store it as the
`mcharbor_backup_key` Docker secret. McHarbor does not persist or log the
generated key.

At runtime, McHarbor reads the key from `BACKUP_ENCRYPTION_KEY_FILE`, which
defaults to:

- `/run/secrets/mcharbor_backup_key`

For Compose deployments, `docker-compose.secrets.yml` maps the secret from
`MCHARBOR_BACKUP_KEY_FILE`, defaulting to:

- `./secrets/mcharbor_backup_key`

Backup archives are written below:

- `<DATA_DIR>/backups/containers/<runId>/mcharbor.tar`

The `mcharbor.tar` file contains the encrypted backup envelope. The archive run
metadata records the encryption algorithm and non-secret key ID so operators can
verify which key version produced a backup.

## Static Assets and Website

Two separate documentation delivery models exist in the repo:

- production SPA assets served from the backend static directory
- standalone marketing website under `website/`

The standalone website is not the main application UI. It is a separate static site.
