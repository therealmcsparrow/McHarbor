# Contract Tests

The tests under `src/` that hold two sides of McHarbor to the same fact. Each one exists because
that agreement broke once without a compile error. The contracts themselves, and how to use them,
are in [Contracts](./README.md).

## Running them

```bash
cd src/backend
GOTOOLCHAIN=auto GOWORK=off go test ./core/rbac/ ./core/apispec/ ./api/ ./core/pluginruntime/ ./core/agent/ ./core/db/
DATA_DIR="$(mktemp -d)" MCHARBOR_EMIT_OPENAPI=api/openapi.json GOTOOLCHAIN=auto GOWORK=off go run .   # after any route change
cd ../frontend && npx vitest run
```


## Permissions

A capability is governed only when five things agree: the `Permission` const in
`src/backend/core/rbac/permissions.go`, its `CatalogSeed()` entry, its `AllPermissions`
membership, the same key in `PERM` in `src/frontend/core/auth/permissionKeys.ts`, and a gate on
every route that uses it.

| Test | File | Holds |
|---|---|---|
| `TestEveryPermissionIsInTheCatalog` | `src/backend/core/rbac/contract_test.go` | Every const in `AllPermissions` has a catalog entry. Without one it has no scope and denies everyone. |
| `TestEveryCatalogEntryIsInAllPermissions` | same | The reverse |
| `TestCatalogEntriesAreWellFormed` | same | No duplicates, a description, a scope from `global`/`environment`/`stack`/`owner`/`account`, a category matching the prefix, lowercase dotted snake_case |
| `TestKeysFollowTheDocumentedConvention` | same | `<category>.<action>` |
| `TestFrontendMirrorMatchesBackendCatalog` | same | The backend catalog and `PERM` hold the same keys |
| `TestKnownDriftIsShrinkOnly` | same | The `knownDrift` allowance never grows |

## Routes and the OpenAPI document

Every host route is registered through `*apispec.Router` (`src/backend/core/apispec/`). Booting
with `MCHARBOR_EMIT_OPENAPI` writes `src/backend/api/openapi.json`, where every operation carries
`x-mcharbor-permission` or `x-mcharbor-exempt`. The route tests read that document.

| Test | File | Holds |
|---|---|---|
| `TestEveryMutatingOperationIsGatedOrExempt` | `src/backend/api/route_gating_test.go` | Every POST/PUT/PATCH/DELETE is gated or exempt |
| `TestEveryGatedKeyIsInTheCatalog` | same | A gate names a real catalog key |
| `TestProtectedOperationsDeclareSecurity` | same | A gated route sits in the protected band |
| `TestOnlyThePluginCatchAllIsUnrecorded` | same | `unrecorded-routes.txt` lists only the plugin dispatcher. Anything else bypassed the registry. |
| `TestExportsRequireTheirExportKey` | same | The audit log's signed export needs `audit.export` |
| `TestWorkflowReadsRequireWorkflowsView` | same | Workflow reads are gated |
| `TestScopedReadRoutesAreGated` | `src/backend/api/route_scoped_get_test.go` | A GET naming a specific environment or agent is gated |
| `TestExemptReasonsUseAKnownPrefix` | `src/backend/api/route_exempt_prefix_test.go` | An exemption starts with a reviewed prefix: `pre-auth:`, `first-run:`, `self-service:`, `gated in handler:`, `IdP callback:`, `inbound webhook:`, `public ITSM callback:`, `agent transfer:`, `node-to-node:`, `device sign-in`, `every user`, `probe:` |
| `TestPluginRoutesAreNotShadowedByHostRoutes` | `src/backend/api/plugin_route_collision_test.go` | No plugin declares a route under a host subtree |
| Router, schema, number and text-response tests | `src/backend/core/apispec/*_test.go` | Request and response types are reflected correctly, so a generated client gets the right shapes |

A route a client calls with a body declares it with `apispec.Request(T{})` /
`apispec.Response(T{})`. `apispec.Scoped()`, `apispec.Paginated()`, `apispec.SSE()`,
`apispec.WebSocket()` and `apispec.Binary()` describe the rest, and `apispec.Exempt("<prefix>: …")`
states why a route has no gate. Never name a response type `<Verb>Response`, because it collides
in generated clients.

## Architecture

| Test | File | Holds |
|---|---|---|
| `TestModuleImportGraph` | `src/backend/api/arch_test.go` | No `modules/X` imports `modules/Y` outside `allowedEdges`. Run with `MCHARBOR_ARCH_STRICT=1`. |

## Plugins

| Test | File | Holds |
|---|---|---|
| `TestPluginManifestRoutesMatchMain` | `src/backend/core/pluginruntime/manifest_routes_test.go` | A plugin's `main.go` routes and its `plugin.yaml` routes are the same set. `knownRouteSetMismatches` may only shrink. |
| `TestPluginMutatingRoutesDeclareAudit` | same | Every mutating plugin route declares `audit_action` + `audit_entity_type`, or `audit_required: false` |
| `TestPluginRouteDeclarationDrift` | `src/backend/core/pluginruntime/route_declaration_drift_test.go` | No plugin registers a non-public route its manifest never declares. Such a route gets an empty key, which the middleware passes through to any signed-in user. |
| `TestVersionAgreementNamesTheDisagreeingPair`, `TestScanReportsInstalledVersionWithNoDirectory` | `src/backend/core/pluginruntime/version_agreement_test.go` | A version disagreement names the two copies that differ |
| `TestCLISpecRoundTrip` | `src/backend/modules/plugin_runtime/cli_roundtrip_test.go` | A declared CLI command set survives record, store and list intact |

## Remote agent

| Contract | Where | Holds |
|---|---|---|
| `TestStreamingResponseKeepsChunksThatArriveBeforeRoundTripWakes` | `src/backend/core/agent/readloop_order_test.go` | `ReadLoop` creates the `StreamReader` before dispatching `http_response_start`. Otherwise early chunks are dropped and a short response hangs. |
| Protocol sync | `src/backend/core/agent/protocol.go` and `src/agent/protocol.go` | The two files are identical apart from the package line, comments and whitespace. Change both together. |

## Database

| Test | File | Holds |
|---|---|---|
| `TestMigrateSQLite` and the external-driver conformance tests | `src/backend/core/db/migrate_driver_test.go` | The embedded migrations apply from scratch to an empty database. The other drivers run when their DSN is set. |

## Frontend

| Test | File | Holds |
|---|---|---|
| The test floor | `src/frontend/test/floor.test.ts` | Every registered guard file exists with at least its recorded number of tests. The floor only grows. |
| Page scroll | `src/frontend/test/page-scroll-contract.test.ts` | A route declaring `scroll: "data"` supplies its own scroller, otherwise the page is cut off at the fold |
| Node predicates | `src/frontend/modules/workflows/node-predicates.contract.test.ts` | Every condition operator the editor offers is implemented by the engine, whose default branch returns `true` |
| Destructive dialogs | every `*Dialog.test.tsx` on the floor | No mutation without confirmation. Denied means disabled with the key named. The server's message arrives intact. One request on a double click. |
| `usePermission`, `Button.permission`, `Button.tooltip` | `src/frontend/core/auth/`, `src/frontend/resources/components/ui/` | A denied control is disabled, stays disabled, and names the key it needs |

## When a contract fails

1. Read the message. Each one names the key, route, plugin or file, and what the other side
   expected.
2. Fix the code, not the test. The shrink-only allowances and the floor minimums exist so that
   fixing is the only way to green.
3. After a route change, regenerate `openapi.json` and run the set again.
4. A new contract test must be shown to fail: break the guarded code, watch the test fail for the
   reason it names, then restore the code.
