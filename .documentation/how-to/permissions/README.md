# Contract permissions

Adding a capability is easy. Making it *governed* is the part people skip, and a half-gated
feature is worse than an ungated one: it looks governed and is not.

A capability is finished when three things are true: the key exists in the catalog, the
backend enforces it, and the frontend shows honestly that it exists and what would unlock it.
All three, or none.

## Before you mint a key

**Reuse before you invent.** A new screen over an existing resource almost always wants that
resource's existing `view` / `manage` / `delete`. A new key for every dialog produces a role
editor nobody can reason about.

**Name it `<category>.<action>`**, lowercase, with a `snake_case` category. Use `view`,
`manage` and `delete` unless the capability genuinely is not one of those. Then add a verb:
`run`, `restore`, `export`, `assign`, `approve`, `access`, `use`, `act`, `download`.

**Pick the narrowest scope that is actually enforceable.**

| Scope | Use when |
|---|---|
| `global` | It applies to the whole install |
| `environment` | It can be granted per Docker or Kubernetes environment |
| `stack` | It can be granted per Compose stack |
| `owner` | It applies to resources the user owns |
| `account` | It applies to an account, often the caller's own |

`global` on a per-environment resource is a privilege escalation waiting to happen.

**Split a key when the two halves are genuinely different decisions.** The AI assistant is the
model to copy: `ai_assistant.use` lets someone ask, `ai_assistant.act` lets them approve what
the assistant proposes. GitOps does the same with `manage` and `approve`: the person who can
edit a pipeline is not automatically the person who may wave a promotion through it.

## Host keys: five steps, one unit of work

A new host permission is not done until all five are present in the same change. The first
three are the ones people remember; the fourth and fifth are where drift comes from.

### 1. The constant

A `Permission` constant in the RBAC package:

```go
PermThingManage Permission = "thing.manage"
```

### 2. The catalog entry

An entry in the catalog seed, with its category, action, description and scope:

```go
{PermThingManage, "thing", "manage", "Create and edit things", "environment"},
```

**A constant with no catalog entry has no scope**, is invisible to the role editor, and fails
in the wrong direction, silently. This step is not optional.

### 3. The `AllPermissions` member

Append the constant to the list of all permissions. Missing it means the key is not offered
where the full set is enumerated.

### 4. The frontend mirror

Add the same key string to the hand-maintained constant map the frontend gates against. A key
missing here has no constant to gate a button with, so call sites fall back to bare string
literals, which no rename will ever catch.

### 5. A role preset migration, if it belongs in one

If a shipped preset should carry the key, add it in a **new** migration. Never edit an
existing migration in place; installs that already ran it will not see the change.

No migration is needed for the key itself. The catalog is written into the database
idempotently at startup from the same seed the authorisation layer builds its scope map from.

## The backend must enforce it

The check lives in `routes.go`, not in the handler body:

```go
r.With(rbac.RequirePermission(app.RBACService, rbac.PermThingManage)).Post("/", h.HandleCreate)
```

| Middleware | Use when |
|---|---|
| `RequirePermission` | The ordinary case; honours the request's environment scope |
| `RequireGlobalPermission` | The key must be held at global scope. An environment grant must not satisfy it |
| `RequireStackPermission` | The key applies to the stack named by a path parameter |
| `RequireAnyPermission` | Any one of several keys unlocks it |
| `RequireAnyGlobalPermission` | Any one of several, at global scope |
| `RequireAnyStackPermission` | Any one of several, for the named stack |

Rules that hold everywhere:

- `view` gates reads, `manage` gates create and update, `delete` gates removal. **A route that
  does two of those takes the stronger key.**
- Route middleware knows the key, not the row. Where a key is `owner`- or `stack`-scoped, the
  **service** re-checks ownership on the specific record. Both are needed.
- **Never ship a mutating route with no permission middleware.** The repository already
  carries a backlog of routes in exactly that state; do not add to it.

### Exempt, legitimately

Do not invent a key for: login, logout, session refresh; health and the OpenAPI document;
an SSE or log proxy already covered by its parent route's key; anything a user does to their
own account, which the self-management key already covers.

Say so explicitly when a change is genuinely not applicable ("no permission needed,
read-only public endpoint") rather than leaving the omission silent for the next reader to
re-derive.

## The frontend must show it

Backend enforcement is the security boundary. The frontend obligation is **honesty**: a user
should be able to see that a control exists and learn which permission would unlock it,
instead of finding out from a 403 toast.

- Pass the permission to the affordance. The button components take a permission prop and
  route it through the gate hook.
- Reference the key through the shared constant, never as a bare string literal.
- **Disabled, not hidden.** A denied control renders disabled with a tooltip naming the
  missing permission. Hiding it makes a table's action column change width between roles and
  leaves the user with no idea what to ask for.
- **Navigation is the one exception.** A nav item's permission hides the link, because a
  dead-end page is not informative.
- For anything the button components do not cover (a whole panel, a tab, a drag handle, a
  context-menu item), call the permission hooks directly. Never re-implement the check
  against a raw permissions array.
- Tooltip copy comes from the shared namespace, and any new explanatory string goes through
  `t()` like every other user-facing string.
- **Never use the gate as the only protection.** If defeating the disabled state in devtools
  would let a request through, the backend check is missing. Go back a section.

## Plugin keys

A plugin declares its own keys and does not touch the host catalog.

```yaml
adds_permissions:
  - key:         "my_plugin.read"
    description: "View data in My Plugin"
    scope:       "env"
    category:    "Plugins"
    presets:     [viewer, editor, operator]
```

The key must match `^[a-z][a-z0-9_.]{0,62}[a-z0-9]$`: lowercase letters, digits, underscore
and dot, with **no hyphens and no colons**. A hyphenated slug still takes an underscored key:
`my-plugin` declares `my_plugin.read`. A key that breaks the grammar fails the install after
download and signature verification, which is a long way to travel for a typo.

The host inserts them with the plugin's slug as their origin and merges them into the catalog
the role editor reads. **A key a plugin never declares is a key no role can be granted**,
leaving the feature reachable only by a wildcard role.

- Two plugins cannot share a key, and a plugin cannot shadow a host key; the host skips it.
  Prefix with the plugin family when in doubt.
- `consumes_permissions` names host keys the plugin relies on; they must already exist.
- Menu entries take their own required permission.
- The plugin route registry wraps every declared route in the permission middleware, so
  declaring correctly *is* enforcing.
- `presets` decides which shipped roles receive the key at install. Omit it and the key is
  reachable only by an administrator until someone edits a role by hand. That is right for a
  destructive capability, usually wrong for a read.

**Documented exception:** storage-adapter plugins ship with no RBAC declarations at all. The
host's settings key already gates every surface they mount.

## A worked split

The reporting add-on is a good model for splitting one feature into keys that match real
decisions:

| Key | Presets | Why separate |
|---|---|---|
| `reporting.read` | viewer, editor, operator | Browsing the library and reading finished reports |
| `reporting.generate` | editor, operator | Producing a new report, the expensive, data-touching act |
| `reporting.schedule` | editor, operator | Automating it. A schedule's last run carries a whole report, and the delivery-target list names configured channels, so this gates reads too |
| `reporting.manage` | operator | Deleting reports, editing schedules, changing settings |

Note the reasoning on `reporting.schedule`: the *reads* under it are gated more tightly than
`reporting.read` because of what those particular reads return. That is the scope question
asked properly: "what does this read expose", not "is this a read".

Note also the menu gate. The studio entry is gated on `generate`, not `read`, because every
route the studio calls needs `generate`; gating it on `read` let a read-only user browse the
catalogue and meet the wall at the button. Gate an entry on what its first action needs.

## Definition of done

- [ ] The key exists as a constant, a catalog entry, an `AllPermissions` member and a frontend
      constant, or, for a plugin, an `adds_permissions` declaration.
- [ ] Every new or changed mutating route has permission middleware in `routes.go`.
- [ ] Every new or changed user-facing action passes the permission, renders disabled with a
      tooltip, and is translated.
- [ ] The scope is the narrowest one that still works.
- [ ] If a preset should include the key, a **new** migration adds it.
- [ ] Where a change genuinely needs no permission, the summary says so explicitly.

## Related

- [API permissions](../../api/conventions/permissions.md): the full catalog, and the routes currently carrying no gate
- [Build a plugin](../plugin/README.md)
- [Plugin manifest](../plugin/manifest.md): the declaration syntax
- [Roles endpoints](../../api/endpoints/roles.md): impact analysis before a role edit lands
