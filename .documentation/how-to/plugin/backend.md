# Plugin backend

The Go side. A plugin's backend is a Go plugin: a `.so` the host opens at boot and looks a
single symbol up in.

## Prerequisites

- **Linux.** `-buildmode=plugin` does not exist on Windows or macOS.
- **The host's Go toolchain.** The version is pinned in the Dockerfile. A mismatch is
  rejected at load, not at build, so it surfaces as a runtime error rather than a compile
  one.
- **One module graph.** `go.mod` carries a replace directive resolving to the host's
  `src/backend`. That is what lets the plugin and the host share the shared packages; without
  it, `plugin.Open` fails with a version complaint that reads like a dependency problem and
  is actually a build-context problem.

## The symbol

```go
// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
    "context"

    "github.com/therealmcsparrow/mcharbor/core/plugin"
)

type MyPlugin struct {
    host plugin.HostContext
    svc  *Service
}

// Plugin is the symbol the host looks up. The name is fixed.
var Plugin plugin.Plugin = &MyPlugin{}
```

The package is `main`, and the exported variable is `Plugin`. Neither is negotiable.

## The lifecycle

The host calls these in order. Every one may return an error, and an error at any point
aborts the install.

```go
func (p *MyPlugin) Manifest() plugin.Manifest

func (p *MyPlugin) RegisterRoutes(r plugin.Registry) error
func (p *MyPlugin) RegisterMenu(r plugin.MenuRegistry) error
func (p *MyPlugin) RegisterWidgets(r plugin.WidgetRegistry) error
func (p *MyPlugin) RegisterNodes(r plugin.NodeRegistry) error
func (p *MyPlugin) RegisterPermissions(r plugin.AccessRegistry) error

func (p *MyPlugin) Init(ctx context.Context, host plugin.HostContext) error
func (p *MyPlugin) Shutdown(ctx context.Context) error
```

`Register*` runs **before** `Init`, so those methods have no host handle and must not do any
work beyond describing contributions. Anything that needs the database, the Docker pool or
the audit log belongs in `Init`.

`Shutdown` runs on uninstall and on host stop. Stop your goroutines there; the host will not
do it for you.

## Routes

```go
func (p *MyPlugin) RegisterRoutes(r plugin.Registry) error {
    r.Route("/api/my-plugin", func(r chi.Router) {
        r.Get("/items",       p.handleListItems)
        r.Post("/items",      p.handleCreateItem)
        r.Delete("/items/{id}", p.handleDeleteItem)
    })
    return nil
}
```

The path prefix must match the slug. The permission middleware is **not** written here. The
host wires it from the manifest's `requires_permission`, before the request reaches your
handler, which is why an undeclared route is refused rather than silently mounted ungated.

### Handler shape

The same discipline the host modules follow, for the same reasons:

```go
func (p *MyPlugin) handleCreateItem(w http.ResponseWriter, r *http.Request) {
    var req CreateItemInput
    if err := response.DecodeBody(r, &req); err != nil {
        response.BadRequestCode(w, r, i18n.ErrInvalidBody)
        return
    }
    if strings.TrimSpace(req.Name) == "" {
        response.BadRequestCode(w, r, i18n.ErrInvalidBody)
        return
    }

    item, err := p.svc.CreateItem(r.Context(), req)
    if err != nil {
        p.host.Logger.Error("creating item", "error", err)
        response.InternalErrorCode(w, r, i18n.ErrInternalServer)
        return
    }

    p.host.Audit.Emit(r, audit.Entry{
        Action:     "plugin.my-plugin.item.created",
        EntityType: "item",
        EntityID:   item.ID,
    })
    response.Created(w, item)
}
```

Parse, validate, call the service, audit, respond. Five steps, in that order.

- **No SQL in a handler.** Queries and logic live in `service.go`.
- **Never return raw error text.** Log the real error with context; return a code. Raw errors
  carry socket paths and host names.
- **Use the host's response helpers** so the envelope and the translated message match every
  other endpoint.

Declaring `audit_required` on the route makes the host record an entry too; emitting
explicitly gives you the entity id, which the automatic one cannot know.

## The host handle

`Init` receives a `HostContext`. Store what you need on the plugin struct.

| Handle | Use for |
|---|---|
| `DB` | Persistence. **Never open your own connection**; the host owns the file and the transaction discipline |
| `Audit` | One entry per mutating operation. Entries are tagged with the plugin origin so admins can filter |
| `Encryption` | Encrypting and decrypting anything sensitive you persist yourself |
| `DockerPool` | Per-environment Docker clients, when the plugin holds `docker.socket.read` |
| `Logger` | Structured logging. Never log a secret |

```go
func (p *MyPlugin) Init(ctx context.Context, host plugin.HostContext) error {
    p.host = host
    p.svc = NewService(host.DB, host.Encryption)
    return nil
}
```

## Rules that bite

**Bound every external call.**

```go
ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
defer cancel()
```

Docker SDK, HTTP, database: all of them. Over the agent transport an unbounded call does not
fail, it hangs.

**No global mutable state** beyond the plugin struct. The host may run more than one instance.

**Every `go func()` gets lifecycle management**, tied to a context you cancel in `Shutdown`.
A goroutine that outlives an uninstall is a leak the operator cannot see.

**Never decide permissions yourself** in place of the declared middleware. Where a permission
is scoped to an owner or a stack, the *service* re-checks the specific row, which is different
from re-implementing the route gate.

**Check every error.** Never discard one with a blank identifier, wrap with `%w`, and always
check the result of encrypt and decrypt.

## Reading the platform

A plugin that needs McHarbor's own data has two options, and they are not equivalent.

**Querying host tables directly** is fast and bypasses every access rule (roles, ABAC
policies, environment scoping). In a product where "who can see production" is a real
question, that is rarely a trade worth making.

**Calling the host's REST API over loopback, forwarding the requesting user's session**, is
slower and inherits the whole permission model for free. A user cannot produce a result
containing data they could not already see in the interface. A module the user lacks
permission for answers 403, and the plugin records the source as unavailable rather than
treating it as an error.

The reporting add-on takes the second path deliberately. See
[Build a reporting plugin](../reporting/README.md).

## Migrations

```
migrations/0001_init.sql
migrations/0002_add_index.sql
```

Numbered, applied in order, once per install, each wrapped in a transaction by the host.
Prefix every table and column so uninstall can drop them in one pass. A plugin can add tables,
indices and triggers; it cannot drop host tables.

Uninstall does **not** reverse your SQL; it drops the plugin's tables by prefix. Write
migrations knowing that is the rollback.

## Adapters

If the plugin fills one of the host's pluggable families (backup destination, notification
channel, identity provider, image scanner, AI provider, snapshot driver, log forwarder), it
registers an adapter instead of building a page. The host already owns the picker, the
configuration dialog, the test button and the per-step report.

Each family has its own spec type and its own registration call, and each shipped plugin in
that family is a working example. See the family's README under
[Plugins](../../plugins/README.md), and the `Developer` page inside the plugin closest to
what you are building.

### Optional contracts

An adapter can implement more than its family's required interface. The host discovers the
extras by type assertion, so a plugin that does not implement one is unaffected.

`StorageQuotaReporter` is the shipped example:

```go
func (c *Client) Quota(ctx context.Context, cfg map[string]string) (plugin.StorageQuota, error)
```

Return used and total bytes and the destination card in **Settings → Integrations →
Storage** grows a capacity bar. Return `plugin.ErrUnsupported` when the protocol has no
concept of capacity (FTP, or SFTP against a server that did not negotiate
`statvfs@openssh.com`), and the card says usage is not available instead of showing an
error nobody can act on. Any other error is audited as `plugin.adapter.storage.quota`.

## Related

- [Build a plugin](./README.md): the end-to-end path
- [Manifest](./manifest.md): declaring the routes this file implements
- [API conventions](../../api/conventions/README.md): the envelope and codes to match
- [Contract permissions](../permissions/README.md)
