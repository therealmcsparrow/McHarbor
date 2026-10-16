// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package contract

import (
	"context"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/plugin/services"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/ai"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/alerting"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/backup"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/identity"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/logging"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/notification"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/reporting"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/scanner"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/snapshot"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/storage"
	"github.com/therealmcsparrow/mcharbor/core/plugin/types"
)

// Plugin is the single value a plugin exports. Mirrors §6.
type Plugin interface {
	Manifest() Manifest

	RegisterRoutes(Registry) error
	RegisterMenu(MenuRegistry) error
	RegisterWidgets(WidgetRegistry) error
	RegisterNodes(NodeRegistry) error
	RegisterPermissions(AccessRegistry) error

	Init(ctx context.Context, host HostContext) error
	Shutdown(ctx context.Context) error
}

// Registry is the route registration surface handed to plugins at
// boot. Public/Auth/Protected correspond to the three middleware
// groups in core/router.
type Registry interface {
	Public(method, path string, h http.HandlerFunc)
	Auth(method, path string, h http.HandlerFunc)
	Protected(method, path string, h http.HandlerFunc)
}

// MenuRegistry lets plugins inject sidebar entries (§7.4).
type MenuRegistry interface {
	Add(item MenuItem) error
	Remove(labelKey string) error
}

// TabRegistry lets a plugin add a tab to one of the host's own pages.
//
// Registered through an optional interface rather than a method on Plugin:
// adding to Plugin would break every implementation at compile time, and a
// plugin with no tabs should not have to carry an empty method.
type TabRegistry interface {
	Add(item TabItem) error
	Remove(pageID, key string) error
}

// TabItem is one tab on a host page.
//
// PageID names the host surface — "settings", "security", "environment",
// "container" — and Key must be unique within that page for this plugin. The
// frontend resolves the label through the plugin's own i18n namespace and
// mounts the component the bundle exports as `tabs.<Key>`.
type TabItem struct {
	PageID   string
	Key      string
	LabelNS  string
	LabelKey string
	Icon     string
	Required string
	Priority int
}

// TabProvider is implemented by plugins that contribute page tabs. The loader
// type-asserts for it after RegisterMenu.
type TabProvider interface {
	RegisterTabs(TabRegistry) error
}

// WidgetRegistry lets plugins register dashboard widgets (§2 / §11).
type WidgetRegistry interface {
	Add(key string, descriptor types.WidgetDescriptor) error
	Remove(key string) error
}

// NodeRegistry lets plugins register workflow nodes (§2 / §11).
type NodeRegistry interface {
	Add(key string, descriptor types.NodeDescriptor) error
	Remove(key string) error
}

// AdapterContributor is an *optional* interface a plugin may implement
// to register as a runtime handler for one of the host's pluggable
// surfaces. The Plugin interface above does NOT require it —
// plugins that don't contribute adapters omit the method (or implement
// it as a no-op) without breaking the loader.
//
// The first user of this interface is the storage_locations module,
// which lets per-destination-type handlers (local, S3, SFTP, Samba,
// etc.) live as plugins. The architecture doc at
// .references/plugins/architecture.md §7.7 reserves room for the same
// pattern on notification channels, AI providers, and identity
// providers; those extensions land in a follow-up release.
//
// Plugins implementing AdapterContributor MUST register at least one
// adapter or return nil. The loader logs a warning if the call returns
// without registering anything, but does not fail the boot — an empty
// plugin is still a valid, side-effect-free install.
type AdapterContributor interface {
	RegisterAdapters(AdapterRegistry) error
}

// AdapterRegistry is the surface handed to a plugin's
// RegisterAdapters method. It exposes per-category sub-registries.
// Plugins call the ones they participate in (a storage destination
// plugin only touches Storage(); a comm-channel plugin only touches
// Notification(); etc.) and ignore the rest.
//
// The per-family return types live in core/plugin/specs/<family> so
// each family's spec types are reachable from both host and plugin
// builds with identical reach sets. See the package doc in this
// directory for the reach-set rationale.
type AdapterRegistry interface {
	Storage() storage.StorageAdapterRegistry
	Notification() notification.NotificationAdapterRegistry
	Identity() identity.IdentityAdapterRegistry
	Snapshot() snapshot.SnapshotAdapterRegistry
	AI() ai.AIAdapterRegistry
	Logging() logging.LogForwarderAdapterRegistry
	// Alerting registers alert condition sources. A plugin that owns
	// a resource worth alerting on (pods, VMs, printers) declares the
	// conditions here; the host's alert engine schedules, thresholds,
	// deduplicates and delivers them exactly as it does its own
	// built-in container / host / platform conditions, which register
	// through this same surface in-process.
	Alerting() alerting.AlertSourceAdapterRegistry
	// Reporting is the one family whose consumer is another plugin
	// rather than the host: a provider registered here appears on the
	// Reporting plugin's Report Studio shelf. See
	// core/plugin/specs/reporting for the split of responsibilities.
	Reporting() reporting.ReportAdapterRegistry
	// Scanner registers image vulnerability scanners. A plugin that
	// wraps a scanning engine (Trivy, Grype, Clair) declares it here.
	// The host keeps the scan rows, the findings, retention and the
	// history page, exactly as it did when the engines were built in —
	// what moves out is only the code that turns an image reference
	// into a list of CVEs.
	Scanner() scanner.ImageScannerAdapterRegistry
	// Backup registers a source of backups for the host's Backups page.
	// The host owns container backups itself; anything else that produces
	// backups — virtual machines, system containers — owns its own plans,
	// runs and restore, and appears on the same page by registering here
	// rather than by the host reading its tables.
	Backup() backup.BackupAdapterRegistry
}

// HostContext is the slice of McHarbor the host hands to a plugin
// during Init(). Mirrors .references/plugins/architecture.md §6.
//
// The handle types (Logger, DBHandle, EncryptionHandle, PoolHandle,
// DockerPoolAccessor, I18nHandle, AuditHandle, RBACHandle,
// IdentityProvisioner, SessionIssuer, AIHandle, SnapshotRPCHandle)
// all live in core/plugin/services so host and plugin builds share
// the same transitive reach set for every field type.
type HostContext struct {
	Logger interface {
		Info(msg string, args ...any)
		Warn(msg string, args ...any)
		Error(msg string, args ...any)
		Debug(msg string, args ...any)
	}
	DB         services.DBHandle
	Encryption services.EncryptionHandle
	DockerPool services.PoolHandle
	AgentPool  services.PoolHandle
	I18n       services.I18nHandle
	Audit      services.AuditHandle
	// Environments resolves an environment id to its display name, which a
	// plugin needs to name anything after it -- a backup path, a report
	// heading -- and cannot read from the database itself.
	Environments services.EnvironmentHandle
	// BackupLog is where a backup provider writes its lifecycle events so they
	// appear in the Backups page's log beside the host's own. nil when the host
	// wired no backup log.
	BackupLog services.BackupLogHandle
	// Lifecycle records a subject's state change on the Logging timeline.
	// Before it, that table was written only by the Docker event collector,
	// so a guest starting or a pool being destroyed appeared nowhere on it.
	// nil when the host wired no recorder.
	Lifecycle services.LifecycleHandle
	RBAC      services.RBACHandle
	// Identity is the host callback for plugins that authenticate
	// users from external identity providers. Required for OIDC,
	// LDAP, SAML plugins. Identity-less plugins can ignore it.
	Identity services.IdentityProvisioner
	// Sessions is the host callback for creating authenticated
	// sessions. Plugins issue a session after successful external
	// auth and write the cookie themselves.
	Sessions services.SessionIssuer
	// AI is the host's AI router. Plugins may invoke it to summarize
	// logs, generate incident reports, etc. Provider implementations
	// (Anthropic, OpenAI, Mistral, Cohere, …) ship as separate plugins;
	// plugins that consume AI do NOT need to depend on any provider.
	AI services.AIHandle
	// SnapshotRPC is the host's bridge for plugins that need to call
	// snapshot operations through the agent transport. Used by
	// local-host plugins (zfs, btrfs, lvm) that wrap CLI tools on the
	// agent host. HTTP-based NAS plugins (synology, truenas)
	// don't need this — they call their provider's REST API
	// directly from the host process. nil when no agent transport
	// is wired (the plugin should treat that as "snapshot mode
	// unavailable" and let the host fall back to docker pause).
	SnapshotRPC services.SnapshotRPCHandle
	// AgentRPC is the host's agent-rpc bridge, narrowed to the message types
	// the plugin declares under its agent-rpc capability. It is `any` so the
	// bridge never widens this ABI: a plugin declares a builtin-typed
	// interface of its own (Available, Call, Subscribe) and asserts it.
	//
	// nil unless the manifest carries both the agent.rpc permission token and
	// an agent-rpc capability; a plugin treats nil as "no agent reachable".
	AgentRPC any
	// Secrets is the plugin's view of the secret vault: Resolve a secret by
	// name for a user who holds secrets.view where it is read, and Release the
	// references a deleted resource recorded. It is `any` for the same reason
	// as AgentRPC: the plugin asserts a builtin-typed interface of its own, so
	// a later method never widens this ABI. nil unless the manifest carries the
	// secrets.read permission token.
	Secrets any

	// Storage lets a plugin write files to the host's configured
	// storage locations. The plugin names a location by id; the host
	// resolves and decrypts it, so no credential crosses the boundary.
	// nil when the host wired no storage bridge.
	Storage services.StorageHandle

	// Reporting is the consumer half of the report-provider family: it
	// lets the Reporting plugin see and drive whatever report providers
	// other plugins registered, without either side importing the
	// other. Every plugin except a report *consumer* ignores it. nil
	// when the host wired no reporting bridge.
	Reporting reporting.ReportingHandle

	// Comms lets a plugin send messages — optionally carrying files —
	// to the host's configured communication channels. Same id-not-
	// credential discipline as Storage. nil when no bridge is wired.
	Comms services.CommsHandle

	// Catalogs lets a plugin read the enabled catalog sources of a kind it
	// contributes to, so the operator manages every source in Settings →
	// Catalogs rather than in a second registry the plugin grew. nil when
	// the manifest does not declare `catalog.source.register`.
	Catalogs services.CatalogHandle

	// Workflows lets a plugin fire its own workflow trigger nodes
	// (registered through NodeRegistry with no Execute) when
	// something it watches happens. nil when no bridge is wired.
	Workflows services.WorkflowHandle

	// WorkDir is the host's data directory (parent of plugins/).
	// Plugins that need a stable on-disk scratch area (e.g. Quadlet
	// writing unit files) read WorkDir to derive their own subtree.
	// Plugins must NOT escape into a foreign plugin's directory.
	WorkDir string
}

// Errors a plugin loader may surface to the host.
//
// ErrScopeViolation is returned by DBHandle.Exec / DBHandle.Query when
// a plugin statement touches a table outside `plugin_<slug>_*`.
// ErrPermissionExists / ErrSchemaExists surface install-time conflicts
// in the AccessRegistry (lives in `host` package). ErrUnknownHostAction
// and ErrPlatformUnsupported are reserved for future host-side
// surfaces (loader / platform-detection).
var (
	ErrScopeViolation      = constErr("plugin attempted to access out-of-scope table")
	ErrUnknownHostAction   = constErr("unknown host action requested by plugin")
	ErrPlatformUnsupported = constErr("plugin runtime not supported on this platform")
)

type constErr string

func (e constErr) Error() string { return string(e) }
