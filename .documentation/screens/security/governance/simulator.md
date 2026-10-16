# Policy simulator and conflicts

**Security → Governance → Policy simulator**, and the warnings on **Roles** and **Policies**

## Simulator

Pick a **User**, an **Environment** (or **No environment (global)**), optionally a **Stack**, and
then either a **Permission** or an **API route** (method and path, for example
`DELETE /api/stacks/web`). **Simulate** answers **Allowed** or **Denied** and lists why.

The answer comes from the live authorization code, not a copy of it:

- The decision per key is what the route gate would decide: for an environment-scoped gate the
  ABAC policy decision the gate consults first (when policies are enforced), then the role check;
  stack-scoped gates use the stack check; global gates the global check.
- For a route, every gate in its middleware chain is read from the registered router, so the
  simulator knows the same keys and scopes the route really enforces. All gates must pass; a gate
  that accepts several keys passes on any one of them ("Any one of these permissions passes this
  gate."). A route with no gate says so, and an exempt route shows its exemption reason.
- A caller's own API key never narrows the answer; the simulation is always of the user's
  interactive access.

### Reasons

| Reason | Meaning |
|---|---|
| Granted by role *name* | A role assigned directly, through a group, for the environment, or for the stack carries the key |
| Granted by the wildcard in role *name* | The role holds `*` |
| Granted by an approved, unexpired elevation | A just-in-time elevation is active |
| Granted by an open break-glass session | The user has opened emergency access |
| Granted / Denied by a per-user override | A `user_permissions` row applies |
| Allowed / Denied by policy *name* | The first matching ABAC policy (when policies are enforced) |
| No role, elevation or policy grants it | Nothing grants it |

When routes that use owner rules and per-user overrides would decide differently from the route
gate, a second line says so ("Routes that use owner rules and overrides would allow it.").

`POST /api/governance/simulate` `{userId, environmentId, stackName, permission, method, path}`
is gated by `permissions.view` or `rbac.policies.view`. Each simulation writes a
`policy.simulate` audit row naming the user whose access was read. An unknown route answers 404.

## Conflict warnings

`GET /api/governance/conflicts`, gated by `roles.view` or `rbac.policies.view`, reports:

| Finding | Severity | Shown on |
|---|---|---|
| A role grants every permission (`*`) | info for built-in roles, warning for custom ones | Roles |
| A role lists `category.*`; category wildcards are not expanded, so these grant nothing | warning | Roles |
| A policy never decides some permissions because an earlier policy (by priority, then name) always matches first (its expression is literally true, or identical) and covers the same resource type | warning | Policies |
| Two policies have the same priority and opposite effects on the same permission, so their names decide the order | info | Policies |

Disabled policies are ignored. The shadow check is deliberately literal: an expression that is
true only in practice is not guessed at.
