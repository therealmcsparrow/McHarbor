# Secrets

**Secrets** is McHarbor's built-in credential vault. You store a credential once, reference
it as `${secret:NAME}` from a stack, a container or a workflow, and McHarbor substitutes
the real value server-side at the moment it invokes Docker, never before, and never in
anything it sends to your browser.

Reach it from **Security → Secrets**. The section requires `secrets.view`.

It sits with the rest of the application's access control rather than under Docker: the
vault holds registry passwords, git credentials, SMTP settings, TLS certificates, OIDC
client secrets and agent enrolment tokens, none of which is a Docker concept. Stack
environment variables were simply the first thing it served.

## What the vault does, and what it does not

**It does:**

- Encrypt every stored credential at rest with AES-256-GCM, so a copy of the database, a
  volume snapshot, a backup archive or a support bundle carries ciphertext rather than
  passwords.
- Put access behind RBAC, with reading a *value* gated far more tightly than reading the
  list.
- Record every disclosure in the tamper-evident audit log.
- Let you reuse one credential across many stacks and rotate it in one place, and tell you
  exactly what is still running the old value afterwards.
- Keep `${secret:NAME}` in your compose file and your database instead of the password.

**It does not, and cannot:**

- Hide a value from `docker inspect` once the container is running, *unless you turn on
  file mode*, below. By default the resolver hands the plaintext to Docker, so anyone
  holding `stacks.manage` or `containers.manage` can reference a secret in a throwaway
  workload, deploy it, and read the value out. The vault is a hard boundary against
  someone holding only `secrets.view`, against API keys, and against anyone reading the
  database directly. It is not a boundary against someone who can deploy.
- Protect against compromise of the McHarbor host itself. The master key lives beside the
  database (or comes from `ENCRYPTION_KEY`), so this protects exfiltration of the *data*,
  not of the *machine*.

## File mode: keeping the value out of the environment

By default a `${secret:NAME}` reference is replaced with the plaintext and handed to
Docker as an ordinary environment variable, where `docker inspect`, `docker compose
config` and the container's own `/proc/1/environ` all show it.

**File mode** changes the delivery. Turn it on and McHarbor instead:

1. writes each referenced value into a file inside a dedicated, read-only Docker volume
   (`mcharbor-secrets-<id>`), mode `0400`;
2. mounts that volume at `/run/secrets`;
3. sets `<VAR>_FILE` to the file's path, and does **not** set `<VAR>`.

So `POSTGRES_PASSWORD=${secret:PG_PW}` becomes
`POSTGRES_PASSWORD_FILE=/run/secrets/POSTGRES_PASSWORD`, and the password itself appears
in no inspect output, no `docker ps` view, and no process environment.

### It is opt-in, per workload

The toggle appears in three places, and defaults to off in all three:

| Where | Applies to |
|---|---|
| **Containers → Create**, Environment section | that container |
| **App Store → Install**, environment step | the stack the install creates |
| **Stack → Environment tab** | that stack, from its next deploy |
| **Container detail → Recreate** | an existing container, on the replacement |

### Changing it on a container that already exists

Docker fixes a container's environment and its mounts at creation, so switching file mode
on or off is always a recreate; there is no live toggle. The **Recreate** confirm dialog
on the container detail page therefore carries the switch, pre-set to how the container
was built. Confirming replaces the container with one built the other way.

For that to be possible at all the container has to remember its own references: a
running container's environment holds the *resolved* value, and `${secret:NAME}` is gone
from it. McHarbor stores the references (secret **names**, never values) in a
`com.mcharbor.secret-refs` label at create time and restores them before re-planning. The
switch is therefore only offered on containers that carry that label or are already in
file mode; on anything else it would have nothing to convert.

Turning it off removes the secret volume once the old container is gone, so the files do
not outlive the decision.

For a stack, McHarbor writes a generated `docker-compose.mcharbor-secrets.yml` beside your
own file and deploys that one. Your compose file is never edited, so a GitOps-managed stack
keeps a clean `git diff`. Turning file mode off deletes the generated file again.

### The image has to support it

This is the whole of the catch. `<VAR>_FILE` is a docker-library convention, not a Docker
feature: the image's entrypoint has to look for it. Most official database, proxy and
queue images do: `POSTGRES_PASSWORD_FILE`, `MYSQL_ROOT_PASSWORD_FILE`,
`MARIADB_PASSWORD_FILE`, `REDIS_PASSWORD_FILE` and similar. An image that does not read
the `_FILE` variant will start with no credential at all, and usually fail loudly on
first connection. Turn file mode on for a workload you have checked, not for everything.

Two further limits worth stating plainly:

- **Only a whole-value reference becomes a file.** `DB_PASSWORD=${secret:PG_PW}` does.
  `DATABASE_URL=postgres://app:${secret:PG_PW}@db/app` cannot: the value is a fragment of
  a larger string, so it is resolved inline as before and stays visible in the
  environment. The install and create forms tell you which variables were converted.
- **It is not a boundary against the Docker socket.** Anyone who can reach the daemon can
  `docker exec` into the container and `cat /run/secrets/POSTGRES_PASSWORD`. File mode
  removes the value from the *metadata* every read-only Docker viewer can see; it does not
  remove the daemon's authority over its own containers.

### The container Files tab needs `secrets.reveal`

The mount is real, so the container's *Files* tab could browse straight into it. It cannot.

- `/run/secrets` is **hidden** for anyone without `secrets.reveal`: the quick-access card
  is gone, and the directory is filtered out of a listing of `/run`, so the path is not
  even advertised.
- Asking for it directly returns **403** with *Reading the secret mount requires the
  secrets.reveal permission*. Reading a file under it, writing, renaming, chmod-ing and
  deleting are all gated the same way.
- Every attempt is audited, permitted or not, at **warning** severity:
  `secret_file_list`, `secret_file_read`, `secret_file_write`, each with a `_denied`
  counterpart. A permitted read is still a credential leaving the vault, so it is logged
  as loudly as a refused one.

This gate is the file browser only. It is not a second boundary against the Docker socket
(the caveat above still holds), and it does not yet ask for the step-up window and
written reason that *Reveal* on the secret itself requires.

### Every mount and volume is audited in the Files tab

Wider than secrets, and for the same reason: data on a volume outlives the container that
wrote it. When a browsed path falls inside any mount, the audit row names it.

| Action | Written when |
|---|---|
| `file_list` | a directory inside a mount is listed |
| `file_read` / `file_download` | a file inside a mount is opened or downloaded |
| `file_save`, `file_upload`, `file_mkdir`, `file_rename`, `file_chmod`, `file_delete` | the change succeeds, anywhere in the container |

Each row carries `mount=<destination>`, `volume=<name or bind source>` and `type=`, and
the innermost mount wins when they nest: `/data/pgdata` is never attributed to `/data`.
Reads **outside** a mount are deliberately not logged: browsing a container's own ephemeral
filesystem is the bulk of the traffic, and a log nobody can read through is a log nobody
reads. Mutations are logged wherever they land.

## Using a secret

1. **Create it.** *New secret* asks for a name (`DB_PASSWORD`; uppercase, digits and
   underscores, starting with a letter), a value (paste one, or press the generate button
   for a strong one from `crypto/rand`), a kind, and a scope.
2. **Reference it.** In a stack's environment variables, an App Store install form, or a
   workflow node, use the picker or type `${secret:DB_PASSWORD}`.
3. **Deploy.** McHarbor resolves the reference immediately before calling Docker. Nothing
   is cached; a rotation takes effect on the next deploy.

### Shift + right-click any field

Anywhere in McHarbor, hold **Shift** and right-click a text or number input and a picker
opens listing your stored secrets. Choose one and the field receives its `${secret:NAME}`
reference. The picker shows names and scopes only; it cannot read a value, and neither
can the endpoint behind it.

The list is a **tree** (scope, then kind, then group, then the secrets themselves), with
every branch collapsible, so a vault of eighty credentials is navigated rather than
scrolled. Scopes appear in resolution order (stack, environment, global, application) and
secrets with no group sit under **Ungrouped**, last within their kind. Typing in the search box
expands the whole tree for as long as the query is there: a collapsed branch hiding a match
is the one thing a search must never do.

A plain right-click, without Shift, gives you the browser's own menu as always, and the
picker never appears for an account without `secrets.view`.

To write a literal `${secret:NAME}` that should *not* be substituted, escape it as
`$${secret:NAME}`.

**A missing reference fails the deploy.** McHarbor will not substitute an empty string;
that is how a database ends up running with a blank root password.

### Groups

A secret can carry an optional **group**: a free-text label such as `Databases`,
`Registries` or `Customer A`. It is a filing label and nothing more: it is not part of the
secret's name, it does not appear in a `${secret:NAME}` reference, and it never changes
what a reference resolves to. Two secrets in different groups still collide if they share a
name at the same scope, exactly as before.

What it changes is the list. The Secrets table gains a **Group** column, a row of group
chips above it that filters to one group (plus **Ungrouped** for everything with no label),
and the search box matches group names alongside the name, description and stack.

Setting one is optional everywhere:

- **On create**: the Group field sits next to Description. It is a selector over the
  groups already in use, and typing a name that does not exist yet offers **Create
  "<name>"**, so a new group is made by using it rather than by managing a list first.
  Leaving it empty is normal, and the secret lands under **Ungrouped**.
- **Afterwards**: the pencil button in the row's action column opens **Edit details**,
  which edits exactly two things: the group and the description. Not the name, not the
  value, not the scope: re-filing a credential must never be a path to reissuing it. It is
  a `PATCH`, so nothing that uses the secret is restarted. Choosing **Remove from group**
  clears the label.

Both need `secrets.manage`; seeing the column and the chips needs only `secrets.view`.
Application credentials (the `System` rows) cannot be edited here; they are filed and
described by the module that owns them.

**Grouped view.** The **Group** switch above the table swaps the flat list for collapsible
sections, one per group, each with its row count. Collapsing is per session and independent
of the chips: the chips narrow to one group, the switch changes how whatever is on screen
is drawn.

Description is no longer a table column. It is in the search index and in the Edit dialog,
which is where it is useful, rather than costing a column to show a sentence truncated.

### Kinds

**Kind** labels what a credential *is*: `env`, `password`, `token`, `cert`,
`private_key`, `kubeconfig`, `ssh_key`, `json`. Like the group it is presentation only:
every kind is encrypted, stored and resolved identically, and nothing branches on it.

Those eight are the defaults, not the permitted set. The Kind field on the create form is
the same creatable selector the group uses, so typing `api_key` and choosing **Create
"api_key"** adds it. A custom kind is normalised to lowercase with underscores for spaces
and must match `^[a-z][a-z0-9_]{0,31}$`; the eight built-ins are translated in the UI, a
custom one renders verbatim.

Kind is set when the secret is created and is not part of the Edit dialog.

### Scope

A reference resolves narrowest-first: **stack → environment → global → application**,
first match wins. So one name, `DB_PASSWORD`, can hold a different value per environment,
and a single stack can override the shared one without renaming anything.

| Scope | Visible to | Needs |
|---|---|---|
| `stack` | one stack in one environment | `secrets.manage` |
| `environment` | every stack in one environment | `secrets.manage` |
| `global` | every environment | `secrets.manage_global` |
| `application` | every environment, resolved last | `secrets.manage_global` |

`secrets.manage_global` is separate on purpose: it stops an environment-level administrator
from planting a credential that every other environment reads. `application` sits behind the
same key for the same reason: it is just as instance-wide as `global`.

**`application` is for credentials McHarbor itself uses**, rather than for a workload, and it
is deliberately **last** in the resolution chain. That ordering is what makes the scope safe
to introduce: an application-scoped secret can only ever resolve a name that resolves to
nothing today, so creating one can never change what an existing `${secret:NAME}` already
means. It is a fallback, not a broader `global`.

Do not confuse it with the **Application credentials** further down this page; those are
the `system = 1` rows a host module owns and manages for itself. The two are independent: a
system secret can carry any scope, and an application-scoped secret you create is an
ordinary secret you own.

## Reading a value back

Reading a stored value is deliberately expensive: it is the one thing on this page that
takes more than a click.

To reveal a secret you need **all** of:

1. **The `secrets.reveal` permission**, which ships in **no role preset**. You obtain it by
   requesting an *elevation*, which an approver grants for a bounded window, 15 minutes by
   default. The approver can widen it.
2. **A step-up confirmation.** Immediately before the reveal, confirm your password or a
   code from your authenticator app. That opens a five-minute window bound to *this browser
   session*, so a stolen session cookie is not enough even when its owner holds a live
   elevation.
3. **A reason**, at least ten characters, recorded verbatim in the audit log.

Then, before the value is returned:

- an audit entry is written (actor, secret name, scope, reason, source IP, user agent, and
  never the value);
- administrators get an in-app notification;
- the response is marked `no-store`.

You are limited to **ten reveals per hour**. The value is shown in a copy panel that hides
itself after 30 seconds, and it is never written to browser storage, to the query cache, or
to the URL.

Two things are never revealable at all: **application credentials** (see below), and
historical versions. Rolling back restores a previous value as the current one; it does not
display it.

## Application credentials

The list hides rows marked **Application** unless you turn on *Show application
credentials*. These are the credentials McHarbor manages for itself: registry passwords,
git credentials, SMTP settings, storage backends, the agent enrolment token, your TLS
certificate and key. They were moved into the vault so there is one place to rotate, one
audit trail, and one future path to an external KMS.

They are managed where they were always managed (in the module that owns them), so from
this page they are read-only: not deletable, not rotatable, not revealable.

Two categories are deliberately **not** in the vault:

- **Your multi-factor secrets.** TOTP seeds and recovery codes stay where they are. They
  are not operator credentials; they are your second factor, and an administrator who could
  reveal them could impersonate your MFA indefinitely.
- **Machine-rotated OAuth tokens.** They refresh hourly and nobody needs to read or roll
  one back.

## Rotating

*Rotate* replaces the value and archives the previous one (the last five are kept, so a bad
rotation can be rolled back). **A rotation changes nothing that is already running**: a
container keeps the environment it started with, and Docker has no way to change it.

What happens next depends on how the credential is consumed, and the three cases behave
very differently:

| Consumer | Effect of a rotation |
|---|---|
| **Application credentials**: registries, git, agents, identity providers, storage backends, SMTP, AI providers | **Immediate.** They read the vault on every use, so the next connection uses the new value. Nothing to redeploy. |
| **Workflows** | **Immediate.** References resolve per execution. |
| **Containers and stacks** | **Deferred.** The resolved value was baked in at create or deploy time, so it takes effect when the resource is recreated or redeployed. |

McHarbor tracks that third case rather than leaving you to remember it.

### Out of date, and what it means

Every time a reference is resolved, McHarbor records the secret **version** the resource was
built with. A resource whose recorded version is behind the secret's current version is
**out of date**: it is still running an older value.

The *Out of date* column on the list is that count per secret; a resource with no recorded
version at all (anything deployed before this tracking existed) counts as out of date, because
its version is genuinely unknown.

### Reviewing the impact

The list's **Review what uses this** action, and the *Review and apply* button on the banner
after a rotation, open the impact dialog. It groups every reference by resource (a stack with
four variables pointing at one secret is one row, not four) and shows for each:

- the variables that reference the secret;
- whether it is out of date, and which version it was built with;
- whether McHarbor can do anything about it. A **Not applicable** resource reads the secret
  each time it runs, so the rotation already reached it and there is nothing to restart.

Reviewing needs only `secrets.view`.

### Applying

*Apply* makes the selected resources run with the current value. Containers are recreated,
stacks redeployed; both re-resolve every `${secret:NAME}` on the way through.

Four things about it are deliberate:

- **It is a separate permission.** `secrets.apply`, not `secrets.manage`. Rotating a value and
  restarting someone's production stack are different authorities, and one key covering both
  would hand out the second silently with the first.
- **The selection defaults to the out-of-date resources and nothing else.** A resource already
  running the current value has no reason to be restarted, though you may select one anyway.
- **It runs one resource at a time and finishes.** Recreating forty containers at once is a
  self-inflicted outage, so they go in turn, but a failure on one says nothing about the next,
  so the run continues and each resource gets its own verdict. The reason for a failure is in
  the server log, not in the browser. If *every* resource failed, suspect the rotated value
  itself rather than the resources.
- **A container in file mode may not restart at all.** If every reference it holds is delivered
  as a file under `/run/secrets`, McHarbor rewrites those files in place, with no recreate and no
  downtime. Whether the process notices is up to the image: most read a `_FILE` variable once at
  start-up. Set the container label `com.mcharbor.secret-signal` to a signal name (`HUP`, say) and
  McHarbor sends it after the refresh, for a process that re-reads on one. Anything else (a
  stopped container, a reference interpolated into a larger string, a change to *which* variables
  are secrets) falls back to a recreate.

### References to things that no longer exist

Every resolve records which resource used the secret. Containers are recreated and removed all
the time, and a reference left behind by one is not an error; it is bookkeeping that nobody
cleaned up. Before this existed, one WordPress container could appear four times in the impact
list: once under its name, and once under the id of each incarnation a recreate had replaced.

The impact dialog now asks each resource whether it is still there, and marks the ones that are
not as **No longer exists**. A missing resource:

- is **not** counted as out of date, since there is nothing left to bring up to date;
- cannot be selected, and is skipped by *Apply* even if named explicitly;
- can be cleared with **Remove stale references**, which needs `secrets.manage`.

The prune is deliberate and never automatic. A page that silently deleted rows while you looked
at it would also delete the evidence that something upstream had stopped cleaning up after
itself.

The probe runs concurrently and is bounded: on an agent environment each check is a round trip
over the WebSocket transport, and asking about thirty containers one at a time is what makes a
dialog feel broken. A probe that fails leaves the resource alone: "could not tell" must never
read as "gone".

### Applying automatically

*Apply rotations automatically* on the secret's form propagates every rotation of that secret
without being asked. It is **off by default and should stay off for anything shared**: a global
secret referenced by forty containers would restart all forty the moment somebody rotated it.

When it is on, the work runs in the background rather than holding the rotation request open,
and the banner says so. The outcome of every resource is written to the audit log either way.

## Deleting

Deleting a secret that something still references will fail the next deploy of everything
that uses it, so the delete dialog shows the reference list and makes you type the name to
force past it.

## Back up your encryption key

> **This is the most important line on this page.**
>
> Every value in the vault is encrypted with the master key at `data/.encryption_key`, or
> with `ENCRYPTION_KEY` if you set one. **Lose that key and every secret is unrecoverable**,
> and since McHarbor's own module credentials now live here too, that means your registry
> passwords, git credentials, SMTP settings, storage backends, TLS private key and agent
> enrolment tokens as well.
>
> Back it up somewhere other than beside the database, and treat it with the same care as
> the database itself.

## Permissions

| Key | Grants | Scope |
|---|---|---|
| `secrets.view` | List names and metadata. **Never values.** | environment |
| `secrets.manage` | Create and rotate environment- and stack-scoped secrets | environment |
| `secrets.manage_global` | Create and rotate secrets every environment can read | global |
| `secrets.delete` | Remove secrets | environment |
| `secrets.reveal` | Read a stored value in plaintext, and browse `/run/secrets` in a container's Files tab | environment |
| `secrets.apply` | Recreate containers and redeploy stacks so they pick up a rotated secret | environment |

`secrets.reveal` is in no preset by design, and is unreachable via an API key; reveals are
interactive sessions only.

## Routes

```
GET    /api/secrets                  list metadata        secrets.view (?group= narrows)
POST   /api/secrets                  create               secrets.manage (+ manage_global if scope=global)
GET    /api/secrets/{id}             metadata             secrets.view
PUT    /api/secrets/{id}             rotate               secrets.manage
PATCH  /api/secrets/{id}             metadata only (incl. group)  secrets.manage
DELETE /api/secrets/{id}             delete               secrets.delete
GET    /api/secrets/{id}/references  list usages          secrets.view
GET    /api/secrets/{id}/impact      usages + staleness   secrets.view
POST   /api/secrets/{id}/apply       propagate a rotation secrets.apply
POST   /api/secrets/{id}/references/prune  drop dead references  secrets.manage
GET    /api/secrets/{id}/versions    rotation history     secrets.view
POST   /api/secrets/{id}/rollback    restore a version    secrets.manage
POST   /api/secrets/generate         mint a value         secrets.manage
POST   /api/secrets/{id}/reveal      read a value         secrets.reveal + a step-up window
POST   /api/auth/sudo                open a step-up window (self-service)
```
