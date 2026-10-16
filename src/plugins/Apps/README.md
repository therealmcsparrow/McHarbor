# Apps/

Compose-stack templates consumed by McHarbor's App Store
(`modules/appstore/`). The format matches what `appstore/catalog.go`
already parses — `template.json` per app, optional `compose.yaml` for
reference, optional icons in `icons/`.

## Layout

```
Apps/
├── icons/
│   ├── nginx.svg
│   └── postgres.svg
├── nginx/
│   ├── template.json
│   └── 1.27.0/
│       └── compose.yaml
└── postgres/
    ├── template.json
    └── 16.3/
        └── compose.yaml
```

## template.json fields

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `id` | string | ✓ | Unique catalog id, e.g. `"nginx-1.27.0"` |
| `slug` | string | ✓ | Lowercase `[a-z0-9-]`, e.g. `"nginx"` |
| `name` | string | ✓ | Display name |
| `description` | string | ✓ | One-paragraph summary |
| `category` | string | ✓ | Grouping key for the App Store UI |
| `image` | string | ✓ | Docker image ref |
| `logo` | string | – | Path under this repo, served by App Store |
| `website`, `docsUrl` | string | – | External links |
| `version` | string | ✓ | semver |
| `updatedAt` | string | ✓ | RFC3339 timestamp |
| `source` | string | – | Repo URL |
| `minMemory` | string | – | Hint shown in the UI |
| `ports` | `PortMapping[]` | – | `{ host, container, protocol }` |
| `volumes` | `VolumeMount[]` | – | `{ host, container, readOnly }` |
| `envVars` | `EnvVarDef[]` | – | `{ key, default, description, secret }` |
| `composeOverride` | string | – | Raw compose YAML; bypasses generated compose |

## Versioning

Apps are versioned per-slug. The host installs the version that
matches the `id` field; admins see only the latest stable unless they
opt into pre-releases in App Store settings.

## Adding a new app

1. `mkdir -p Apps/<slug>/<version>`.
2. Copy `Apps/nginx/template.json` and edit.
3. Drop an icon into `Apps/icons/`.
4. Add an entry to the top-level `index.yaml` under `apps:`.
5. Open a PR.

## compose.yaml

The compose file in `<slug>/<version>/compose.yaml` is **reference
only** — the install flow renders compose from `template.json`. We
keep it in the repo so humans can read what gets deployed, and so
contributors can `docker compose -f Apps/<slug>/<version>/compose.yaml up`
locally to test their templates before opening a PR.