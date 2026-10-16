# Host Service Contracts

What the host hands a plugin in `Init(ctx, host HostContext)`: one field per service, each typed as
an interface from `src/backend/core/plugin/services/`. Keep the `HostContext` on the plugin
struct and call through it at request time. The plugin never receives the host's database, key or
credentials, only these handles.

```go
func (p *MyPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	return nil
}

func (p *MyPlugin) handleList(w http.ResponseWriter, r *http.Request) {
	rows, err := p.host.DB.Query(r.Context(), "SELECT id, name FROM plugin_myplugin_items LIMIT 1000")
	// …
}
```

## Three rules for every handle

1. **Check for `nil`.** A field marked *may be nil* below is `nil` when the host wired no
   bridge for it, for example `AgentRPC` without the `agent.rpc` token and an `agent-rpc` capability. Treat `nil` as "not
   available" and say so to the operator. Never dereference it blind.
2. **Check `Available()`** where a handle has one (`AI`, `Storage`, `Comms`, `Catalogs`,
   `Reporting`). The handle can exist while there is nothing configured behind it.
3. **Reach extensions by type assertion.** A handle's method set is part of every built `.so`'s
   ABI, so the host never adds a method to an existing handle. New capabilities arrive as a
   second interface that you assert for. If the assertion fails, the host is older than the
   plugin: tell the operator, and don't offer a feature that cannot work.

```go
stream, ok := p.host.Storage.(plugin.StorageStreamHandle)
if !ok {
	// The host predates streamed storage uploads.
}
```

## The handles

| Field | Interface | Methods | May be nil |
|---|---|---|---|
| `Logger` | inline | `Info`, `Warn`, `Error`, `Debug(msg string, args ...any)` | |
| `DB` | `DBHandle` | `Exec(ctx, query, args...) (int64, error)` · `Query(ctx, query, args...) (Rows, error)` · `KV() KVStore` | |
| `Encryption` | `EncryptionHandle` | `EncryptString(plain) (string, error)` · `DecryptString(cipher) (string, error)` | |
| `DockerPool`, `AgentPool` | `PoolHandle` | `ForKind(kind) any` | |
| `I18n` | `I18nHandle` | `Register(lang, namespace string, bundle map[string]string) error` | |
| `Audit` | `AuditHandle` | `Emit(event string, fields map[string]any) error` | |
| `Environments` | `EnvironmentHandle` | `Name(ctx, environmentID) string` | |
| `BackupLog` | `BackupLogHandle` | `Record(ctx, BackupLogEntry) error` | yes |
| `Lifecycle` | `LifecycleHandle` | `Record(ctx, LifecycleEvent) error` | yes |
| `RBAC` | `RBACHandle` | `HasPermission(userID, envID, perm) (bool, error)` · `RegisterLabel(key, ns, labelKey)` · `UnregisterLabel(key)` | |
| `Identity` | `IdentityProvisioner` | `ProvisionUser(ctx, ProvisionRequest) (*ProvisionedUser, error)` | |
| `Sessions` | `SessionIssuer` | `IssueSession(userID) (sessionID string, expiresInSec int, err error)` · `SessionCookieName() string` · `SessionIsSecure(r) bool` | |
| `AI` | `AIHandle` | `Complete(ctx, AIRequest) (AIResponse, error)` · `Available() bool` · `Connections() []AIConnection` | |
| `SnapshotRPC` | `SnapshotRPCHandle` | `Capability(ctx, envID)` · `Create(ctx, envID, SnapshotCreateRequest)` · `Remove(ctx, envID, SnapshotRef)` | yes |
| `AgentRPC` | `any`, asserted against a local interface | see [below](#agentrpc) | yes |
| `Secrets` | `any`, asserted against a local interface | see [below](#secrets) | yes |
| `Storage` | `StorageHandle` | `Upload(ctx, locationID, key string, data []byte) error` · `Locations() []StorageLocationRef` · `Available() bool` | yes |
| `Reporting` | `ReportingHandle` | `Providers()` · `Build(ctx, typeKey, req)` · `Subjects(ctx, typeKey, kind, req)` · `Available()` | yes |
| `Comms` | `CommsHandle` | `Send(ctx, channelID, CommsMessage) error` · `Channels() []CommsChannelRef` · `Available() bool` | yes |
| `Catalogs` | `CatalogHandle` | `Sources(ctx, kind) ([]CatalogSourceRef, error)` · `Available() bool` | yes, unless the manifest declares `catalog.source.register` |
| `Workflows` | `WorkflowHandle` | `TriggerBindings(ctx, key)` · `Fire(ctx, key, payload, match) (int, error)` · `DockerEnvironmentIDs(ctx)` | yes |
| `WorkDir` | `string` | The host data directory. Derive a subtree of your own, and never enter another plugin's. | |

### Notes per handle

- **`DB`** reaches only tables prefixed `plugin_<slug>_`. Anything else returns
  `ErrScopeViolation`. Values go through `?` placeholders. `KV()` is a small key-value store
  with `Get`, `Set` and `Delete`, for settings that do not deserve a table. `Rows` has `Next`,
  `Scan`, `Close` and `Err`.
- **`DockerPool.ForKind("docker")`** returns a `DockerPoolAccessor`, with
  `Get(envID) (any, error)` and `IsAgentEnv(envID) bool`. The host resolves the real client
  pool at call time.
- **`Audit.Emit`** writes an entry tagged `origin=<slug>`. For an entry attributed to the
  acting user, assert `plugin.AuditActorHandle` (`EmitEvent(AuditEvent) error`) and fall back
  to `Emit` when the assertion fails.
- **`RBAC.RegisterLabel`** is called in `Init` for each permission key the plugin declared, so
  the role editor shows a readable name from the plugin's own i18n namespace.
- **`AI.Complete`** is synchronous, rate-limited per plugin (60/min by default) with a 30 s
  per-call timeout. The plugin gets the assembled response, never the API key.
  `AIRequest.ConnectionID` pins a specific connection. Leave it empty to use the default.
- **`Storage` and `Comms`** take a location or channel **id**. The host resolves and decrypts it,
  so no credential crosses into the plugin. For artifacts too large to hold in memory, assert
  `plugin.StorageStreamHandle`: `UploadStream(ctx, StorageStreamRequest)`,
  `DownloadStream(ctx, StorageDownloadRequest) (io.ReadCloser, error)` and
  `DeleteObject(ctx, locationID, key) error`.
- **`Identity` and `Sessions`** go together for a sign-in plugin: provision the user, issue a
  session, then write the cookie named by `SessionCookieName()` with `Secure` set from
  `SessionIsSecure(r)`.
- **`Workflows.Fire`** runs every workflow bound to one of the plugin's trigger nodes. `match`
  filters the bindings, and the return value is how many fired.

### AgentRPC

The bridge to a remote agent, for plugins that run work on a host the McHarbor process cannot
reach. The field is typed `any`: the plugin declares a builtin-typed interface in its own code and
asserts the field against it, so the plugin ABI names no agent type. The field is `nil` unless the
manifest has the `agent.rpc` token **and** an `agent-rpc` capability.

| Method | Purpose |
|---|---|
| `Available(ctx, envID) (bool, string)` | Whether the environment takes agent calls, and `not_agent_environment`, `agent_offline` or `agent_too_old` when it does not. |
| `Call(ctx, envID, msgType, payload, onProgress) ([]byte, error)` | Sends one declared message; the payload and reply are the JSON of one envelope slot. |
| `Subscribe(sink)` | Receives the declared unsolicited frames on a 256-deep queue; `nil` removes the sink. |

The full contract, error codes, the ceiling of routable types and the goldens are in
[Agent RPC](./agent-rpc/README.md). The earlier typed handle, `ComputeRPCHandle`, with
`PoolFileHandle` and `ConsoleStream`, was removed.

### Secrets

The secret vault, read on a user's behalf. Typed `any` like `AgentRPC`: declare this interface in
your own code and assert the field against it. The field is `nil` unless the manifest lists the
`secrets.read` token.

| Method | Purpose |
|---|---|
| `Available() bool` | Proves the assertion matched; a `nil` field never reaches the plugin. |
| `Resolve(ctx, userID, environmentID, name, resourceID, resourceName, field) (string, error)` | The value of the secret called `name` as seen from `environmentID` (stack, environment, then global precedence, as `${secret:NAME}` resolves). The **user** must hold `secrets.view` there: the vault itself checks nobody, so the host checks here. With a `resourceID` the use is recorded as a reference of type `plugin`, id `<slug>:<resourceID>`, named `resourceName`, and the secret's usage list shows it as **Copied once**. |
| `Release(ctx, resourceID) error` | Forgets every reference recorded for that resource; call it when the resource is deleted. |

A refused `Resolve` returns an error carrying `SecretCode() string`: `denied`, `not_found` or
`invalid_name`. Assert `interface{ SecretCode() string }` and map it onto your own i18n codes. The
value reaches your backend only; never send it to a browser, and treat it as a one-time copy — the
vault does not know how to apply a rotation to a plugin's resource. List `secrets.view` under
`consumes_permissions` if your frontend lists secret names (`GET /api/secrets?env=`).

## Frontend host services

The browser-side equivalent is `window.McharborHost`, with `i18n`, `permissions`, `environment`
and `toast`. It is described in [Hooks](../hooks/README.md#host-services-windowmcharborhost).
