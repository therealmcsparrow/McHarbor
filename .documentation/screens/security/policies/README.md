# Policies

`/security/policies` · Security → Management → Policies

Attribute-based policies refine what roles grant. A role says *which* permission a user holds;
a policy says *when* and *on what* it applies: only production containers, only during office
hours, only from the office network, never through an API key.

## Permissions

| Key | What it unlocks |
|---|---|
| `rbac.policies.view` | The list, and the editor's reference panel (`GET /api/rbac/policy-reference`) |
| `rbac.policies.manage` | Creating, editing, enabling and deleting policies |

Policies only take effect when the **v3 authorization** rollout is on (setting `rbac_v3_enabled`).

## How a policy decides

Each policy has an **effect** (allow or deny), a **priority**, a **resource type**, the
**permissions** it governs, and a JavaScript **expression** that must return `true` to match.

When a request reaches a gated route, McHarbor works out which resource the route is about and
evaluates the enabled policies bound to that resource type and permission, lowest priority
number first. The first policy that matches decides: a deny refuses the request, an allow lets
it through. When no policy matches, the role grant decides as it always did.

A deny policy therefore narrows a role, and an allow policy can grant a permission on a
specific resource that the role does not carry.

An expression that errors or runs over its 50 ms budget does not match.

## Resource types

The **Resource type** picker lists every type the host and the loaded plugins declare,
grouped by category and labelled with where it comes from. A type is only listed while its
plugin is loaded.

| Source | Types |
|---|---|
| Host | Container, Image, Volume, Network, Stack, Environment, Workflow, Secret, Schedule, Alert rule, Registry, Storage location, Container backup plan, Git repository, Desired state, Incident, Communication channel, Webhook, User, Group, Role, API key, Identity provider |
| KVM plugin (`addon-kvm`) | Virtual machine |
| LXC plugin (`addon-lxc`) | System container |
| Compute plugin (`addon-compute`) | Storage pool, Guest network |

Each type binds to its routes (`/api/vms/{id}` for a virtual machine, `/api/stacks/{name}`
for a stack), and every route beneath one inherits it, so `/api/vms/{id}/start` is decided
against that machine.

System containers have no routes bound: their plugin serves them under `/api/containers/…`,
which is the Docker container path, so a policy on **System container** is evaluated by the
plugin's own checks but not at the route gate.

Plugin authors declare types through the `core/plugin/specs/abac` contract:
`ABACResourceTypes()` lists them with their routes and attributes, and
`ResolveABACResource()` fills in a resource's name, owner, state, labels and attributes.

## Permissions picker

The **Permissions** field works like the one in the role editor: the full catalog grouped by
category, searchable, with **Include** per permission and **Include all** / **Clear** per
category. Plugin permissions are listed under their plugin.

## Expression reference

The panel beside the expression has four tabs. Clicking an entry inserts it at the cursor.

- **Resource**: the attributes of the selected type, e.g. `resource.attrs.vcpus` for a
  virtual machine, plus `resource.id`, `resource.name`, `resource.owner`, `resource.state`,
  `resource.environment` and `resource.labels`.
- **Variables**: the caller, environment, request and clock:

| Variable | Meaning |
|---|---|
| `user.id`, `user.username`, `user.email` | The caller |
| `user.roles`, `user.groups` | Role names (direct or through groups) and group names |
| `user.labels` | Labels on the user; read with `hasLabel(user.labels, key)` |
| `env.id`, `env.name`, `env.type` | The environment of the request; empty for global actions. `env.type` is the environment's connection type (local, agent and so on). The route gate, which runs before the handler, resolves all three |
| `request.method`, `request.path` | The HTTP call |
| `request.ip` | Client address; forwarding headers count only from `MCHARBOR_TRUSTED_PROXY_CIDRS` |
| `request.apiKey` | The call was made with an API key |
| `request.readOnly` | `GET`, `HEAD` or `OPTIONS` |
| `now.hour`, `now.minute`, `now.weekday`, `now.day`, `now.month`, `now.year` | Server clock; weekday 0 is Sunday |
| `now.date`, `now.unixTime` | `YYYY-MM-DD` (UTC) and seconds since 1970 |

- **Functions**:

| Function | Example |
|---|---|
| `hasLabel(labels, key)` | `hasLabel(resource.labels, "env") == "prod"` |
| `labelIn(labels, key, value, …)` | `labelIn(resource.labels, "tier", "web", "api")` |
| `inList(value, item, …)` | `inList(env.name, "prod", "prod-eu")` |
| `hasRole(name)` | `hasRole("Platform Admin")` |
| `inGroup(name)` | `inGroup("oncall")` |
| `startsWith`, `endsWith`, `contains` | `startsWith(resource.name, "db-")` |
| `matches(text, pattern)` | `matches(resource.name, "^web-[0-9]+$")`: RE2, 256 characters at most |
| `inCIDR(ip, cidr, …)` | `inCIDR(request.ip, "10.0.0.0/8")` |
| `between(number, low, high)` | `between(now.hour, 8, 18)`, inclusive |
| `isWeekend()` | `!isWeekend()` |

- **Examples**: complete expressions; choosing one replaces the expression.

## What goes wrong

| Symptom | Cause |
|---|---|
| A policy never applies | v3 authorization is off, the policy is disabled, it names no permission the route checks, or another policy with a lower priority number matches first |
| A plugin's resource type is missing | The plugin is not loaded, or its build predates resource types; update it from the Plugin Store |
| `request.ip` is the proxy's address | The proxy is not in `MCHARBOR_TRUSTED_PROXY_CIDRS` |
| A policy on `env.id` or `env.name` ignores a call that clearly belongs to an environment | Earlier builds could not look the environment up at the route gate, so `env.id` and `env.name` were empty there and an environment-scoped policy never matched at the gate (a deny never fired, and `env.id != "prod"` allowed everything). The lookup is fixed; nothing needs re-saving |
| A policy that reads `env.name` or `env.type` stops applying when the environment lookup fails | On a real lookup error only `env.id` is usable. A policy that reads any other `env` attribute is not evaluated: a deny still applies and an allow is skipped, so the role check decides. A missing environment row is not an error: `env.id` is the bare id and the name is empty |
