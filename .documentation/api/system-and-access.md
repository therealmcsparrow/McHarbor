# System and Access

## Public Routes

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/health` | Health probe for the backend service. |
| GET | `/api/about` | Returns application metadata including version details. |
| GET | `/api/auth/status` | Reports whether setup already exists and whether auth is required. |
| POST | `/api/auth/login` | Creates a session from `{ username, password }`. |
| POST | `/api/auth/logout` | Clears the current session cookie. |
| POST | `/api/auth/setup` | Bootstraps the first local account. |
| GET | `/api/identity-providers/enabled` | Lists enabled external identity providers. |
| GET | `/api/identity-providers/{id}/authorize` | Starts an external identity-provider login flow. |
| GET | `/api/identity-providers/callback` | Completes the external identity-provider callback flow. |

## Protected Routes

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/auth/session` | Returns the authenticated user and current session context. |
| GET | `/api/docs/` | Returns authenticated machine-readable API docs. |

## API Keys

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/api-keys/` | Lists API keys available to the current principal. |
| POST | `/api/api-keys/` | Creates an API key from `{ name, scopes, expiresAt? }`. |
| GET | `/api/api-keys/{id}` | Returns metadata for a specific API key. |
| DELETE | `/api/api-keys/{id}` | Revokes an API key. |

Create body:

```json
{
  "name": "CI access",
  "scopes": [
    "containers:read",
    "stacks:write"
  ],
  "expiresAt": "2026-12-31T23:59:59Z"
}
```

## Identity Providers

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/identity-providers/` | Lists configured external identity providers. |
| POST | `/api/identity-providers/` | Creates an identity-provider configuration. |
| GET | `/api/identity-providers/{id}` | Returns one provider configuration. |
| PUT | `/api/identity-providers/{id}` | Updates a provider configuration. |
| DELETE | `/api/identity-providers/{id}` | Deletes a provider configuration. |
| POST | `/api/identity-providers/{id}/test` | Tests a provider configuration. |
| GET | `/api/identity-providers/{id}/groups` | Fetches provider-side groups for mapping or review. |

## Notes

- Protected routes require valid auth and can still return `403` when RBAC denies access.
- Identity-provider admin routes are guarded by settings-management permissions.
