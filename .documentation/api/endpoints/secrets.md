# Secrets vault

14 endpoints: stored credentials, their rotation history, what uses them, and pushing a
rotation out to the containers and stacks that run them.

| Key | Covers |
|---|---|
| `secrets.view` | Metadata, references, impact, version history |
| `secrets.manage` | Create, rotate, edit metadata, generate a value, roll back, prune references |
| `secrets.manage_global` | On top of `secrets.manage`, for a secret whose scope is `global`, checked in the service |
| `secrets.apply` | Recreate and redeploy what uses a secret |
| `secrets.delete` | Delete a secret |
| `secrets.reveal` | Read a plaintext value, with a step-up window |

## Things worth knowing first

**Reads never return a value.** List, get, references and versions are metadata only. The one
route that hands back a stored value is `POST /api/secrets/{id}/reveal`: it is a `POST` so the
value never lands in browser history or an access log, it needs `secrets.reveal`, which ships
in no role preset, and it needs a step-up window opened with `POST /api/auth/sudo`
(`scope: "secrets.reveal"`) on the same session.

**A generated value is shown once.** Creating or rotating with `generate: true` answers
`{secret, value}`; that response is the only place the value appears.

**Rotating and applying are separate authorities.** `PUT /api/secrets/{id}` replaces the value;
`POST /api/secrets/{id}/apply` takes running workloads down for the length of a redeploy so
they pick it up, and is gated by `secrets.apply` so the first does not silently grant the
second.

**The global-scope check is in the service.** The scope arrives in the body, which middleware
cannot see, so `secrets.manage_global` is enforced by the service for a `global` secret.

## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `GET` | `/api/secrets` | `secrets.view` | Returns a page of secret metadata, never values | `?group=`, `?includeSystem=`, `?search=` |
| `POST` | `/api/secrets` | `secrets.manage` | Stores a new secret. With `generate: true` the response is `{secret, value}` and the generated value is shown here only, once | — |
| `POST` | `/api/secrets/generate` | `secrets.manage` | Mints a value without storing it, so a client can show a strong password before deciding to keep it | — |
| `DELETE` | `/api/secrets/{id}` | `secrets.delete` | Removes a secret | `{id}`, `?force=` |
| `GET` | `/api/secrets/{id}` | `secrets.view` | Returns one secret's metadata, never its value | `{id}` |
| `PATCH` | `/api/secrets/{id}` | `secrets.manage` | Updates a secret's metadata only: description, kind, group, scope | `{id}` |
| `PUT` | `/api/secrets/{id}` | `secrets.manage` | Replaces a secret's value (rotation). With `generate: true` the new value is returned once, here only | `{id}` |
| `POST` | `/api/secrets/{id}/apply` | `secrets.apply` | Recreates containers and redeploys stacks so they run with the secret's current value; an empty `resources` means every stale resource | `{id}` |
| `GET` | `/api/secrets/{id}/impact` | `secrets.view` | Reports what uses a secret and what still runs an older version of it. A plugin's use (resource type `plugin`, such as a virtual machine's cloud-init password) is a one-time copy, reported `supported: false` and shown as **Copied once** | `{id}` |
| `GET` | `/api/secrets/{id}/references` | `secrets.view` | Lists what uses a secret | `{id}` |
| `POST` | `/api/secrets/{id}/references/prune` | `secrets.manage` | Deletes the reference rows for resources that no longer exist, or for the ones named in the body | `{id}` |
| `POST` | `/api/secrets/{id}/reveal` | `secrets.reveal` | Returns a secret's plaintext value. Needs a `secrets.reveal` step-up window on the session as well as the key | `{id}` |
| `POST` | `/api/secrets/{id}/rollback` | `secrets.manage` | Restores a historical value as the current one, without displaying it | `{id}` |
| `GET` | `/api/secrets/{id}/versions` | `secrets.view` | Lists a secret's rotation history, metadata only | `{id}` |

## Request bodies

**`POST` `/api/secrets`**: `name`, `description`, `kind`, `group`, `scope`, `environmentId`?, `stackName`?, `value`, `generate`, `generateLength`, `autoApply`

**`POST` `/api/secrets/generate`**: `length`

**`PATCH` `/api/secrets/{id}`**: `description`?, `kind`?, `group`?, `scope`?, `environmentId`?, `stackName`?, `autoApply`?

**`PUT` `/api/secrets/{id}`**: `value`, `generate`, `generateLength`

**`POST` `/api/secrets/{id}/apply`**: `resources`

**`POST` `/api/secrets/{id}/references/prune`**: `resources`

**`POST` `/api/secrets/{id}/reveal`**: `reason`

**`POST` `/api/secrets/{id}/rollback`**: `version`

## Related

- [Secrets](../../screens/security/secrets.md): the screen, kinds, groups and references
- [Authentication](./auth.md): `POST /api/auth/sudo`, the step-up window
