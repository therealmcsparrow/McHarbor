# Contribute facets to the shared reports

A platform plugin that owns workloads (a hypervisor, a cluster, a container runtime) does not
ship its own inventory, capacity or health report. It answers a **facet**, and the Reporting
plugin prints the answer beside every other platform in its shared reports. This page is the
contract a plugin author implements. The operator's view of the same thing is in
[Cross-platform facets](../../../reporting/facets/README.md).

Facets are separate from report types. A plugin can declare report types, facets, or both, and a
spec needs at least one of the two. Report types remain the right home for a document only one
platform can produce, such as a hypervisor's snapshot schedules or a cluster's storage estate.

## The contract

The types live in `core/plugin/specs/reporting/facets.go` and a plugin reaches them through the
`plugin` package re-exports (`plugin.ReportFacetDecl`, `plugin.ReportFacetContributor` and so
on), so plugin code imports nothing else.

```go
type FacetDecl struct {
    Facet, Platform, PlatformLabelKey string
}

// ReportProviderSpec.Facets []FacetDecl

type ReportFacetContributor interface {
    ContributeFacet(ctx context.Context, req FacetRequest) (FacetResult, error)
}
```

The provider client that already implements `ReportProviderClient` also implements
`ReportFacetContributor`. A separate interface means a provider with report types only is never
forced to answer facets.

### `FacetDecl`

| Field | Meaning |
|---|---|
| `Facet` | One of `inventory`, `capacity`, `protection`, `health`, `exposure`, `cost`. The set is closed: `reporting.Facets()` lists it in the order reports print it. |
| `Platform` | A short lowercase token matching `^[a-z][a-z0-9_-]{0,31}$`, carried by every row (`kvm`). |
| `PlatformLabelKey` | A key in the plugin's own i18n namespace (`vm:reports.platform`) naming the platform for the studio and the ad-hoc planner. Empty falls back to the spec's `DisplayName`. Ship it in all six locales. |

### `FacetRequest`

`Facet`, `EnvironmentID`, `Environment`, `PeriodStart`, `PeriodEnd`, `Cookie`, `Lang`, `User` and
`UserID`. `Cookie` and `Lang` come from the calling request: never log, store or render either.

### `FacetResult`

Fill only the slice for the requested facet.

| Slice | Row type | Fields beyond `FacetIdentity` |
|---|---|---|
| `Inventory` | `InventoryRow` | `State`, `Image`, `CPU` (cores), `MemoryMB`, `DiskGB`, `Created` |
| `Capacity` | `CapacityRow` | `Resource` (`cpu`, `memory`, `disk`), `Allocated`, `Used`, `HasUsed`, `Capacity`, `Unit` |
| `Protection` | `ProtectionRow` | `Protected`, `Method`, `LastGood` (zero is never), `OffHost`, `RPOTargetHours`, `Note` |
| `Health` | `HealthRow` | `Status` (`up`, `degraded`, `down`, `unknown`), `Restarts`, `Flaps`, `Since`, `Detail` |
| `Exposure` | `ExposureRow` | `Protocol`, `Port`, `TargetPort`, `Address`, `Scope` (`public`, `internal`, `cluster`), `Via` |
| `Cost` | `CostRow` | `CPUCoreHours`, `MemoryGBHours`, `DiskGBHours`, `Basis` (`allocated` or `measured`) |

Every row embeds `FacetIdentity`: `Platform`, `Kind` (`vm`, `ct`, `pod`, `deployment`, `service`,
`node`, `pool`, `host`, `container`), `ID`, `Name`, `EnvironmentID`, `Environment`, `Host` and
`Owner`.

Three fields exist so a report can be honest, and a contributor must set them truthfully:

- `CapacityRow.HasUsed` separates a measured zero from a platform that cannot measure use.
- `ProtectionRow.OffHost` is true only when a copy exists outside the object's own storage. A
  snapshot on the same pool as its disk is not a backup.
- `CostRow.Basis` is `allocated` when the platform can only report what was reserved. The report
  never adds allocated and measured hours without saying which is which.

`FacetResult` also carries three optional lists:

- `Blocks`: a passage printed under the platform's sub-heading for what a row cannot say (a
  Swarm quorum digest). Only reports that show passages print them.
- `Sources`: a `SourceStatus` per read, so the Methodology annex can name what was consulted
  and what failed.
- `Notes`: free lines for the annex.

## Permissions

The plugin must check its own view key for `req.UserID` through
`HostContext.RBAC.HasPermission`, exactly as `ReportRequest` already requires, because the
Reporting plugin collects as the generating user and the facet call bypasses the plugin's routes.
A denied caller gets no rows and one `SourceStatus{OK: false, Note: "not permitted"}`, so the
annex says the platform was withheld rather than empty. Return that as a normal result, not an
error.

## Validation at registration

`Register` refuses a spec when:

- `Facets` is non-empty and the client does not implement `ReportFacetContributor`.
- A facet is not in the closed set.
- A platform token does not match the pattern above.
- The same `(facet, platform)` pair appears twice in one spec, or is already owned by another
  provider. One pair has one owner, so two plugins cannot both be the KVM inventory.

Uninstalling a plugin drops its facets with it.

## How the Reporting plugin asks

The Reporting plugin asserts `ReportFacetHandle` on `HostContext.Reporting`. It is a second
interface on the same value, not new methods on `ReportingHandle`, so a consumer built before
facets existed keeps loading.

```go
FacetProviders(facet string) []FacetProvider
CollectFacet(ctx, facet string, req FacetRequest) []FacetContribution
```

`CollectFacet` calls every declaring provider concurrently under the caller's context and
returns one `FacetContribution` per provider, failures included. A provider that panics is
contained to its own contribution. A failed contribution is audited as
`plugin.adapter.reporting.facet` with the facet, the environment and the requesting user; a
success is not, because the report's own build row already records the run.

The consumer records each provider as an annex source, turns a failure into a fixed "the platform
did not answer" note (your error text is never printed), and lists the platforms that answered.

## Declaring it

```go
Facets: []plugin.ReportFacetDecl{
    {Facet: plugin.ReportFacetInventory, Platform: "kvm", PlatformLabelKey: "vm:reports.platform"},
    {Facet: plugin.ReportFacetHealth, Platform: "kvm", PlatformLabelKey: "vm:reports.platform"},
},
```

Declare only what you can answer from data the plugin actually holds. Every shipped platform
omits at least one facet for that reason, and the shared reports simply leave the platform out of
that table.

## Constraints that come with the contract

- **It is ABI.** `core/plugin/specs/reporting` is linked by every plugin, so a change to it
  changes every plugin's build identity. The facet contract shipped with one full catalog rebuild
  of every plugin. Do not widen these structs as a side effect of another change; see
  [Plugin ABI and libc](../../plugin/abi-and-libc.md).
- **Nothing is declared in `plugin.yaml`.** Facets are Go data, so `mcharbor plugin dev validate`
  has nothing to check. The registry validation above is the only gate, and the provider's tests
  should assert the declarations and each facet's rows.
- **The report key prefix rule still applies to report types**, not to facets: a facet has no key
  of its own, only the closed facet name plus your platform token.

## Related

- [Build a reporting plugin](../README.md)
- [Cross-platform facets](../../../reporting/facets/README.md)
- [Report catalog](../../../reporting/reports/README.md)
- [Reporting developer page](../../../development/add-ons/reporting.md)
