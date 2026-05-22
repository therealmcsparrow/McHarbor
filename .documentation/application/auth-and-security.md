# Authentication and Security

McHarbor uses local authentication first, with additional identity-provider support
and an RBAC layer on top of user authentication.

## Session Authentication

The core auth service lives in `src/backend/core/auth/auth.go`.

Key characteristics:

- session cookie name: `mcharbor_session`
- session duration: 24 hours
- password hashing: Argon2id
- first registered user is automatically granted the Admin role

## Login and Setup Flow

Public auth endpoints:

- `POST /api/auth/login`
- `POST /api/auth/logout`
- `POST /api/auth/setup`
- `GET /api/auth/status`

Protected auth endpoint:

- `GET /api/auth/session`

The frontend `AuthProvider` triggers a session check on mount so the SPA can decide
whether to show authenticated routes or auth screens.

## API Key Authentication

Protected API routes are wrapped by API key middleware before session middleware.

Behavior:

- reads `Authorization: Bearer ...`
- requires token prefix `mch_`
- hashes the token for DB lookup
- rejects revoked or expired keys
- resolves the owning user and continues request handling as that user

## RBAC

McHarbor uses permission-gated routes throughout the backend.

Examples:

- view vs manage permissions
- module-specific permissions for containers, stacks, settings, workflows, and other modules
- role and group assignment APIs

Key governance modules:

- users
- roles
- groups
- identity providers
- API keys

## Identity Providers

External identity-provider support is exposed through:

- public authorize and callback routes
- protected CRUD and test routes

This allows McHarbor to keep local auth as the baseline while supporting broader
identity integration.

## Encryption

Sensitive stored values are protected with the encryption layer in `core/encryption`.
The encryption key comes from environment configuration or generated runtime state
under the data directory.

## Security Middleware

The main router applies:

- recovery middleware
- request logging
- security headers
- CORS
- request body size limits
- auth-route rate limiting

## Operational Security Posture

Notable security-oriented design choices include:

- self-hosted deployment
- read-only Docker socket mount in the main compose file
- outbound remote agent model instead of exposed remote Docker daemons
- RBAC checks on many module routes
- activity and audit logging for governance visibility
