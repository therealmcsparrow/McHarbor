// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package pluginruntime hosts the *host-side* plugin loader: the bits
// that read appstore_plugins, dlopen each .so, walk its lifecycle
// methods, and dispatch adapter calls back into the host.
//
// This package is intentionally separate from `core/plugin`. The
// public types that plugins depend on (plugin.Plugin, plugin.AdapterContributor,
// plugin.HostContext, plugin.Manifest, the AdapterRegistry interfaces and their
// Spec/Client/Message siblings, etc.) all live in `core/plugin` â€”
// plugins only ever import that one package. The loader internals
// live here and are never visible to a plugin .so.
//
// Why split: Go's `plugin.Open` runtime checks each shared package's
// per-package ABI hash (the `pkghashes` table) against the host's
// runtime hash for the same package. If the plugin and host compiled
// `core/plugin` in different module contexts â€” even with identical
// source â€” the hash differs and plugin.Open rejects the .so with
// "different version of package <X>". The plugin linker prunes any
// package symbols it can't prove reachable from the plugin's exported
// entry points, so the plugin's view of `core/plugin` becomes a
// strict subset of the host's view, producing a different hash.
//
// Splitting the host-only loader code into `core/pluginruntime` keeps
// `core/plugin` itself small, stable, and reachable as a whole from
// the plugin â€” the linker sees the same package contents on both
// sides, so the hashes match.
package pluginruntime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	goplugin "plugin"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"log/slog"

	"github.com/therealmcsparrow/mcharbor/core/agentrpc"
	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/desiredstatesources"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/incidentsources"
	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/plugin/host"
	"github.com/therealmcsparrow/mcharbor/core/plugincaps"
	"github.com/therealmcsparrow/mcharbor/core/pluginslug"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/core/telemetry"
	"gopkg.in/yaml.v3"
)

// RouterDeps is the surface the loader needs from core/router.
// Kept narrow so test harnesses can wire minimal stubs.
type RouterDeps interface {
	MountProtectedRoutes(func(chi.Router))
	MountAuthRoutes(func(chi.Router))
	MountPublicRoutes(func(chi.Router))
	// HasPermission is the read-only RBAC surface the loader needs
	// to wire per-route permission checks on plugin-contributed
	// routes. The full rbac.Service is intentionally not exposed
	// here so plugins can't mutate host policies at runtime —
	// mutations go through AccessRegistry at install time only.
	HasPermission(userID, envID, perm string) (bool, error)
	// HasPermissionInContext is the same check with the request context,
	// so a plugin route called with a Bearer API key is fenced by that
	// key. The SDK-facing HasPermission stays for plugin-internal use.
	HasPermissionInContext(ctx context.Context, userID, envID, perm string) (bool, error)
	// SetPluginRoutes replaces the live route set for one plugin, or
	// removes it when routes is empty.
	//
	// The Mount*Routes calls above are drained once, when router.New
	// builds the tree, so they can only ever serve a plugin that was
	// already loaded at that moment — which is no plugin installed,
	// enabled or updated afterwards. Plugin routes go through here
	// instead, at boot and at runtime alike, so there is one code path
	// and the runtime case cannot silently diverge from the boot case
	// the way it did before.
	SetPluginRoutes(origin string, routes []router.PluginRoute)
}

// HostServices is the slice of host-side singletons the loader
// threads through to plugins via plugin.HostContext. Mirrors Â§6 of
// .references/plugins/architecture.md.
//
// Step 5 (Phase C prerequisite): DockerPool is exposed so Docker-adjacent
// plugins can drive container operations through the same pool the host uses.
//
// Step 6 (Phase C-extras, identity plugins): plugin.IdentityProvisioner
// and plugin.SessionIssuer are exposed so identity plugins (OIDC, LDAP,
// SAML) can authenticate external users without touching core
// `users` / `groups` / `sessions` tables directly.
type HostServices struct {
	DB         *sql.DB
	Encryption *encryption.Service
	Logger     *slog.Logger
	AuditLog   *audit.Logger
	DockerPool DockerPoolSource
	// Lifecycle is the host-side surface behind plugin.LifecycleHandle, so a
	// plugin's own objects reach the Logging timeline the Docker collector
	// used to be the only writer of.
	Lifecycle LifecycleRecorder

	// BackupLog is the host-side surface behind plugin.BackupLogHandle, so a
	// plugin that backs something up writes into the same feed the Backups
	// page already filters and paginates.
	BackupLog BackupLogRecorder

	// Environments resolves an environment id to its display name for the
	// plugins that name things after it.
	Environments EnvironmentNamer

	// SnapshotRPC lets a snapshot plugin reach an agent host. The
	// filesystem drivers a plugin wraps run on the machine holding the
	// data, which for an agent environment is not this process.
	SnapshotRPC plugin.SnapshotRPCHandle
	// AgentRPC is the process's agent-rpc bridge. A plugin receives a view of
	// it narrowed to its own agent-rpc declaration, never the bridge itself.
	AgentRPC *agentrpc.Bridge
	// Secrets is the vault a plugin reads through its secrets view, which
	// checks the acting user's secrets.view itself because the vault does not.
	Secrets  SecretsSource
	Identity IdentityProvisionerSource
	// ExternalLogin is the host half of a directory sign-in a plugin checks
	// itself (LDAP): lockout, provisioning and the same completion a password
	// login gets. nil leaves plugins with the plain identity handle.
	ExternalLogin ExternalLoginSource
	Sessions      SessionIssuerSource
	AI            AICompleterSource
	Storage       StorageDispatchSource
	Comms         CommsDispatchSource
	// Catalogs is the host-side surface behind plugin.CatalogHandle: the
	// enabled settings_catalogs rows of a kind, so a plugin reads the
	// sources an operator manages rather than keeping a registry of its own.
	Catalogs plugin.CatalogHandle
	// Workflows is the host-side surface behind plugin.WorkflowHandle.
	// Keys arriving here are already the full `plugin:<slug>:<key>`
	// canvas action; the loader adds the prefix per plugin.
	Workflows WorkflowTriggerSource
}

// WorkflowTriggerBindingInfo is the loader-level shape of one trigger
// node placement. Mirrors plugin.WorkflowTriggerBinding; the split
// keeps modules/workflows from importing the plugin package.
type WorkflowTriggerBindingInfo struct {
	WorkflowID string
	NodeID     string
	Config     map[string]any
}

// WorkflowTriggerSource is implemented by internal/bootstrap over the
// workflow trigger service.
type WorkflowTriggerSource interface {
	TriggerBindings(ctx context.Context, actionKey string) ([]WorkflowTriggerBindingInfo, error)
	Fire(ctx context.Context, actionKey string, payload map[string]any, match func(WorkflowTriggerBindingInfo) bool) (int, error)
	DockerEnvironmentIDs(ctx context.Context) ([]string, error)
}

// StorageDispatchSource is the host-side surface behind
// plugin.StorageHandle. The loader wraps it so plugins never import a
// host module. Resolution and decryption happen on the far side of
// this interface — the loader only forwards an id.
type StorageDispatchSource interface {
	Upload(ctx context.Context, pluginSlug, locationID, key string, data []byte) error
	Locations(ctx context.Context) []StorageLocationInfo
}

// StorageLocationInfo is the loader-level shape of a selectable
// storage destination. It mirrors plugin.StorageLocationRef; the split
// keeps modules/storage_locations from importing the plugin package.
type StorageLocationInfo struct {
	ID           string
	Name         string
	LocationType string
}

// StorageStreamDispatchSource is the streaming half of the storage
// bridge, asserted on the source rather than required of it.
//
// Separated from StorageDispatchSource so a host that wired only the
// buffered bridge keeps working: the loader offers streaming to plugins
// only when the source it was actually given implements this.
type StorageStreamDispatchSource interface {
	UploadStream(ctx context.Context, pluginSlug string, req StorageStreamDispatch) (StorageStreamDispatchResult, error)
	DownloadStream(ctx context.Context, pluginSlug string, req StorageDownloadDispatch) (io.ReadCloser, error)
	DeleteObject(ctx context.Context, pluginSlug, locationID, key string) error
}

// StorageDownloadDispatch is one streamed read crossing the bridge.
type StorageDownloadDispatch struct {
	LocationID string
	Key        string
	Decrypt    bool
}

// StorageStreamDispatch is one streamed upload crossing the bridge.
//
// It mirrors plugin.StorageStreamRequest, minus the plugin slug the
// loader supplies itself, for the same reason StorageLocationInfo
// mirrors StorageLocationRef.
type StorageStreamDispatch struct {
	LocationID string
	Key        string
	Body       io.Reader
	SizeHint   int64
	Encrypt    bool
	Progress   func(written int64)
}

// StorageStreamDispatchResult is where a streamed artifact landed.
type StorageStreamDispatchResult struct {
	RemotePath   string
	BytesWritten int64
	KeyID        string
}

// CommsDispatchSource is the host-side surface behind
// plugin.CommsHandle.
type CommsDispatchSource interface {
	Send(ctx context.Context, pluginSlug, channelID string, msg CommsDispatchMessage) error
	Channels(ctx context.Context) []CommsChannelInfo
}

// CommsDispatchMessage is the loader-level message shape.
type CommsDispatchMessage struct {
	Title   string
	Message string
	Level   string
	Files   []CommsDispatchFile

	// Recipient retargets this one send. Empty means the channel's
	// stored destination. The host bridge hands it to the dispatcher,
	// which writes it into the single config key the resolved adapter
	// declared - never into any other.
	Recipient string
}

// CommsDispatchFile is one attachment at the loader level.
type CommsDispatchFile struct {
	Filename    string
	ContentType string
	Data        []byte
}

// CommsChannelInfo is the loader-level shape of a selectable channel.
type CommsChannelInfo struct {
	ID                      string
	Name                    string
	ChannelType             string
	IsDefault               bool
	SupportsFileAttachments bool

	// Recipient mirrors the adapter's declared overridable
	// destination. Zero for a webhook- or token-bound provider.
	Recipient NotificationRecipientDecl
}

// NotificationRecipientDecl is one adapter's declared overridable
// destination field, as recorded by the loader and read back by the
// host bridge.
//
// An alias rather than its own struct so the loader, the installer's
// Contributor interface and the communications module all name the
// same shape without a conversion at each boundary.
type NotificationRecipientDecl = plugin.NotificationRecipientOverride

// IdentityProvisionerSource is the host-side callback surface for
// identity plugins. The loader wraps it in an adapter that satisfies
// plugin.IdentityProvisioner.
type IdentityProvisionerSource interface {
	ProvisionUser(ctx context.Context, req IdentityProvisionRequest) (*IdentityProvisionedUser, error)
}

// SessionIssuerSource is the host-side callback surface for session
// issuance from identity plugins. The loader wraps it in an adapter
// that satisfies plugin.SessionIssuer.
type SessionIssuerSource interface {
	IssueSession(userID string) (sessionID string, expiresInSec int, err error)
	SessionCookieName() string
	SessionIsSecure(r *http.Request) bool
}

// AICompleterSource is the host-side callback surface for the plugin
// AI bridge. The loader wraps it in an aiAdapter that satisfies
// plugin.AIHandle, converting between these loader-local types and
// the SDK's plugin.AIConnection / AIRequest / AIResponse — the same
// split IdentityProvisionerSource uses so this package never needs
// to import core/plugin/services types into the interface itself.
//
// The real implementation lives in modules/ai_providers (row
// resolution + provider_type dispatch already lives there) and is
// wired at the composition root (main.go's bootPluginRuntime),
// alongside DockerPool/Identity/Sessions. core/pluginruntime MUST
// NOT import modules/ai_assistant or modules/ai_providers directly
// — that would cycle back into this package.
type AICompleterSource interface {
	// Available reports whether the host can currently dispatch a
	// completion: at least one enabled AI connection exists AND a
	// plugin has registered a provider adapter for its provider_type.
	Available() bool
	// Connections lists the enabled AI connections a plugin may pin
	// to, as id/name/provider_type/is_default only — never a
	// credential.
	Connections() []AIConnectionInfo
	// Complete resolves connectionID (empty = the host's default /
	// favourite connection) and dispatches to whichever plugin
	// registered that connection's provider_type. An unknown,
	// disabled, or missing connectionID is a hard error — it must
	// never silently fall back to a different connection.
	//
	// pluginSlug is the calling plugin's identity, supplied by the
	// loader (buildHostContext knows it from the Init call it is
	// building a HostContext for) — never by the plugin itself, so a
	// plugin can never spoof another plugin's identity. It exists so
	// the implementation can enforce AIHandle's documented per-plugin
	// rate limit; it is host-side plumbing only and is NOT part of
	// the plugin-facing plugin.AIHandle.Complete(ctx, req) signature.
	Complete(ctx context.Context, pluginSlug, connectionID string, req AICompletionRequest) (AICompletionResponse, error)
}

// AIConnectionInfo mirrors plugin.AIConnection but lives in the
// loader package to avoid an import cycle. No credential field,
// ever — id/name/provider_type/is_default only.
type AIConnectionInfo struct {
	ID           string
	Name         string
	ProviderType string
	IsDefault    bool
}

// AICompletionRequest mirrors plugin.AIRequest but lives in the
// loader package to avoid an import cycle.
type AICompletionRequest struct {
	Model        string
	Messages     []AICompletionMessage
	Temperature  *float32
	MaxTokens    *int
	Stop         []string
	Metadata     map[string]string
	ConnectionID string
}

// AICompletionMessage mirrors plugin.AIMessage.
type AICompletionMessage struct {
	Role    string
	Content string
	Name    string
}

// AICompletionResponse mirrors plugin.AIResponse.
type AICompletionResponse struct {
	Content  string
	Model    string
	Provider string
	Usage    AICompletionUsage
	Metadata map[string]string
}

// AICompletionUsage mirrors plugin.AIUsage.
type AICompletionUsage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

// IdentityProvisionRequest mirrors plugin.ProvisionRequest but lives
// in the loader package to avoid an import cycle.
type IdentityProvisionRequest struct {
	ProviderSlug  string
	ProviderID    string
	Subject       string
	Username      string
	Email         string
	DisplayName   string
	Groups        []string
	AutoProvision bool
	DefaultRoleID string
	// UsernamePrefix is the plugin-chosen username prefix; empty means
	// the host default. See plugin.ProvisionRequest.
	UsernamePrefix string
}

// IdentityProvisionedUser mirrors plugin.ProvisionedUser.
type IdentityProvisionedUser struct {
	ID          string
	Username    string
	DisplayName string
	Email       string
}

// DockerPoolSource is the untyped slice of *docker.ClientPool that
// the loader hands to plugins via plugin.PoolHandle.Plugins type-assert
// host.DockerPool.ForKind("docker") to *docker.ClientPool and call
// Get / IsAgentEnv on it. Declared as an interface here so the
// loader doesn't import the docker SDK directly.
type DockerPoolSource interface {
	Get(envID string) (any, error)
	IsAgentEnv(envID string) bool
}

// RBACReader is the read-only permission-check surface exposed to
// plugins. The host's full RBAC service is intentionally NOT
// exposed â€” mutations go through the AccessRegistry at Install.
type RBACReader interface {
	HasPermission(userID, envID, perm string) (bool, error)
}

// Loaded describes one plugin currently resident in memory after
// a successful Boot. Status is mirrored in the loader status route.
type Loaded struct {
	Slug     string    `json:"slug"`
	Version  string    `json:"version"`
	Path     string    `json:"path"`
	Loaded   bool      `json:"loaded"`
	Err      string    `json:"error,omitempty"`
	LoadedAt time.Time `json:"loadedAt,omitempty"`
}

// Runtime is the live handle returned by Boot. The plugin installer
// calls Enable/Disable to flip rows in the `appstore_plugins` table;
// the loader reads that table again on the next Boot. There is no
// hot-reload in Phase A.
type Runtime struct {
	mu        sync.RWMutex
	dataDir   string
	logger    *slog.Logger
	host      HostServices
	deps      RouterDeps
	manifests map[string]plugin.Manifest
	loaded    map[string]*Loaded
	open      map[string]*goplugin.Plugin

	// tracker is the function the loader calls to record every
	// contribution (routes, menu, widgets, nodes) into the DB.
	// Set at construction; defaults to nil (no DB tracking).
	tracker ContributionTracker

	// cliRoots maps a claimed CLI root name to the plugin slug that owns it, so
	// a second plugin asking for the same name is refused rather than silently
	// shadowing the first in every operator's command tree.
	cliRoots map[string]string

	// cliErrors records why a plugin's command set was refused, so
	// /api/plugins/loader/status can say so. A refused set never disables the
	// plugin, which is exactly why the reason has to be reportable somewhere.
	cliErrors map[string]string

	// labels is the permission-label store behind
	// HostContext.RBAC.RegisterLabel. Plugins call it for every key
	// they declare so the role editor can show a translated label
	// instead of a raw permission string.
	labels *plugin.LabelRegistry

	// adapters is the registry of plugin-contributed runtime
	// adapters. Plugins implementing plugin.AdapterContributor register
	// here at boot; the host's pluggable surfaces
	// (modules/storage_locations, modules/communications,
	// modules/ai_assistant, modules/identity) consult the registry
	// at runtime to dispatch to a plugin when one is available.
	adapters *adapterRegistry

	// nodeExecutors holds every workflow node executor a plugin
	// attached to a NodeDescriptor, keyed by the canvas action
	// `plugin:<slug>:<key>`. The workflow engine reaches it through
	// plugin.WorkflowNodeExecutorSource. Guarded by mu.
	nodeExecutors map[string]plugin.NodeExecFunc
}

// ContributionTracker persists plugin contributions across restarts
// so the diagnostic loader routes can report them after a fresh Boot.
// Implemented by modules/appstore/plugins/service.go in Step 5.
type ContributionTracker interface {
	RecordRoute(ctx context.Context, origin, method, path, requiresPermission string, isPublic bool) error
	RecordMenu(ctx context.Context, origin, parent, labelNS, labelKey, path, icon, permission string, priority int) error
	RecordTab(ctx context.Context, origin, pageID, key, labelNS, labelKey, icon, permission string, sortOrder int) error
	RecordWidget(ctx context.Context, origin, key, title, category, defaultSize, settingsJSON string) error
	RecordNode(ctx context.Context, origin, key, category, displayName, icon, configSchemaJSON, inputPortsJSON, outputPortsJSON, requiresPermission string) error
	RecordTheme(ctx context.Context, origin, id, name, description, cssPath string, swatchHue int, defaultVariant string, hasDark, hasLight bool) error
	RecordCatalog(ctx context.Context, origin, catalogID, kind, name, url string) error
	RecordNotificationAdapter(ctx context.Context, origin, channelType, displayName string, supportsFiles, supportsRich, encryptedCredentials bool, recipient NotificationRecipientDecl) error
	RecordCLIRoot(ctx context.Context, origin, name, short, aliasesJSON string) error
	RecordCLICommand(ctx context.Context, origin string, cmd CLICommandRow) error
	DropCLI(ctx context.Context, origin string) error
	ClearAll(ctx context.Context, origin string) error
}

// Boot walks the plugin data directory, opens every enabled plugin,
// calls its hooks, and returns a Runtime ready to be queried.
//
// The host calls this exactly once during main(); subsequent calls
// would attempt to re-open the same .so files and are rejected.
//
// If a single plugin fails to load, Boot logs the error and
// continues with the next one. The boot sequence never aborts
// because one plugin is broken â€” that is the whole point of
// isolation.
func Boot(ctx context.Context, dataDir string, deps RouterDeps, host HostServices, tracker ContributionTracker) (*Runtime, error) {
	if err := checkPlatformSupported(); err != nil {
		return nil, err
	}

	rt := &Runtime{
		dataDir:       dataDir,
		logger:        host.Logger,
		host:          host,
		deps:          deps,
		manifests:     make(map[string]plugin.Manifest),
		loaded:        make(map[string]*Loaded),
		open:          make(map[string]*goplugin.Plugin),
		nodeExecutors: make(map[string]plugin.NodeExecFunc),
		tracker:       tracker,
		adapters:      newAdapterRegistry(),
		labels:        plugin.NewLabelRegistry(),
	}
	rt.adapters.tracker = tracker
	rt.adapters.logger = host.Logger
	rt.adapters.audit = newAdapterAuditor(host.AuditLog, host.Logger)

	if err := rt.scanAndLoad(ctx); err != nil {
		return rt, err
	}
	return rt, nil
}

// PlatformError is returned by Boot when the host is running on a
// platform where Go plugins are unsupported (Windows, macOS, or a
// musl-libc Linux). The runtime still returns a non-nil Runtime so
// callers can detect "no plugins loaded" without nil-checking.
type PlatformError struct {
	Reason string
}

func (e *PlatformError) Error() string { return "plugin runtime unsupported: " + e.Reason }

// checkPlatformSupported returns nil on supported platforms and an
// error otherwise. Go's plugin package is Linux-only as of Go 1.23.
func checkPlatformSupported() error {
	if runtime.GOOS != "linux" {
		return &PlatformError{Reason: "Go plugins require Linux (host GOOS=" + runtime.GOOS + ")"}
	}
	return nil
}

// scanAndLoad walks the data dir, opens each plugin, and dispatches
// the lifecycle calls. Each plugin runs in its own deferred recover
// so a panic in one cannot kill the loop.
func (r *Runtime) scanAndLoad(ctx context.Context) error {
	if r.dataDir == "" {
		r.logger.Debug("plugin runtime: no data dir configured, skipping")
		return nil
	}
	root := filepath.Join(r.dataDir, "plugins")
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			r.logger.Debug("plugin runtime: data dir does not exist yet", "path", root)
			return nil
		}
		return fmt.Errorf("reading plugin dir %s: %w", root, err)
	}

	enabled, err := r.loadInstalledSlugs(ctx)
	if err != nil {
		return fmt.Errorf("loading installed plugin rows: %w", err)
	}

	matched := make(map[string]bool, len(enabled))
	onDisk := make(map[string][]string, len(enabled))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		slug, version, ok := splitSlugVersion(entry.Name())
		if !ok {
			r.logger.Warn("plugin runtime: skipping malformed directory", "path", dir)
			continue
		}
		wantVersion, isEnabled := enabled[slug]
		if !isEnabled {
			r.logger.Debug("plugin runtime: skipping disabled plugin", "slug", slug)
			continue
		}
		onDisk[slug] = append(onDisk[slug], version)
		if wantVersion == "" || wantVersion == version {
			matched[slug] = true
		}
		// Skip stale disk dirs whose version doesn't match the DB row.
		// Update leaves the old <slug>@<oldVer> directory on disk after
		// writing the new row — without this filter, Go's plugin.Open
		// rejects the second .so for the same slug with "plugin already
		// loaded". The first match (old version) would silently win
		// against the new version the DB row says we want.
		if err := checkVersionAgreement(slug, wantVersion, version, "", ""); err != nil {
			r.logger.Info("plugin runtime: skipping stale plugin directory", "slug", slug, "diskVersion", version, "enabledVersion", wantVersion, "reason", err)
			continue
		}
		if err := r.loadOne(ctx, slug, version, dir); err != nil {
			r.logger.Error("plugin runtime: load failed", "slug", slug, "version", version, "err", err)
			r.mu.Lock()
			r.loaded[slug] = &Loaded{Slug: slug, Version: version, Path: dir, Loaded: false, Err: err.Error()}
			r.mu.Unlock()
		}
	}
	r.recordMissingInstallDirs(root, enabled, matched, onDisk)
	return nil
}

// recordMissingInstallDirs gives every installed plugin with no directory at its row's
// version a failed status entry; skipping it silently left the plugin unloaded with
// nothing in the status endpoint to say why.
func (r *Runtime) recordMissingInstallDirs(root string, enabled map[string]string, matched map[string]bool, onDisk map[string][]string) {
	for slug, wantVersion := range enabled {
		if matched[slug] {
			continue
		}
		err := missingInstallDirError(slug, wantVersion, onDisk[slug])
		r.logger.Error("plugin runtime: install directory missing", "slug", slug, "version", wantVersion, "err", err)
		r.mu.Lock()
		if current, ok := r.loaded[slug]; !ok || !current.Loaded {
			r.loaded[slug] = &Loaded{
				Slug:    slug,
				Version: wantVersion,
				Path:    filepath.Join(root, slug+"@"+wantVersion),
				Loaded:  false,
				Err:     err.Error(),
			}
		}
		r.mu.Unlock()
	}
}

// loadInstalledSlugs returns the set of slugs present in
// appstore_plugins (one row per install). Every installed plugin
// is loaded — install and load are the same lifecycle event, with
// uninstall being the only way to stop loading.
//
// Returns slug -> version (not slug -> bool) so the disk walk can
// skip stale <slug>@<oldVersion> directories left behind by an Update.
// Without the version, the loader would dlopen every disk entry for
// each slug and Go's plugin.Open would fail with "plugin already
// loaded" on the second one — even though the DB only has one row.
//
// The enabled column is preserved on appstore_plugins for backward
// compatibility (always 1) but is no longer consulted by the loader.
func (r *Runtime) loadInstalledSlugs(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string)
	// No LIMIT: this is the installed set the loader reconciles against, not a
	// page of it. A cap would unload whatever plugin fell past it.
	rows, err := r.host.DB.QueryContext(ctx, `SELECT slug, version FROM appstore_plugins`)
	if err != nil {
		return nil, fmt.Errorf("querying installed plugins: %w", err)
	}
	defer closeRows(r.logger, rows, "installed-slug")
	for rows.Next() {
		var s, v string
		if err := rows.Scan(&s, &v); err != nil {
			return nil, fmt.Errorf("scanning installed plugin row: %w", err)
		}
		out[s] = v
	}
	return out, rows.Err()
}

// splitSlugVersion parses "<slug>@<version>" directory names.
func splitSlugVersion(dir string) (string, string, bool) {
	idx := strings.LastIndex(dir, "@")
	if idx <= 0 || idx == len(dir)-1 {
		return "", "", false
	}
	return dir[:idx], dir[idx+1:], true
}

// LoadCounts reports how many plugins are loaded and how many failed, from the
// in-memory table only, so a metrics scrape never touches disk or the database.
func (r *Runtime) LoadCounts() (loaded, failed int) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, l := range r.loaded {
		if l.Loaded {
			loaded++
		} else {
			failed++
		}
	}
	return loaded, failed
}

// loadOne opens a single .so, looks up the plugin.Plugin symbol, and walks
// its lifecycle methods. Panics are caught and converted into a
// non-nil err so the outer loop can record them.
func (r *Runtime) loadOne(ctx context.Context, slug, version, dir string) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic during load: %v", rec)
		}
		if err != nil {
			telemetry.PluginLoadFailed(slug)
		}
	}()

	manifest, err := readManifestFile(filepath.Join(dir, "plugin.yaml"))
	if err != nil {
		return err
	}
	if manifest.Slug != slug {
		return fmt.Errorf("manifest slug %q does not match directory %q", manifest.Slug, slug)
	}
	if err := checkVersionAgreement(slug, "", version, manifest.Version, ""); err != nil {
		return err
	}

	soPath := filepath.Join(dir, "backend", slug+".so")
	if _, statErr := os.Stat(soPath); statErr != nil {
		if errors.Is(statErr, fs.ErrNotExist) && len(manifest.Contribs.Themes) > 0 {
			return r.loadManifestOnly(ctx, slug, version, dir, manifest)
		}
		return fmt.Errorf("checking %s: %w", soPath, statErr)
	}
	if err := checkToolchain(soPath, runtime.Version()); err != nil {
		return err
	}
	p, err := goplugin.Open(soPath)
	if err != nil {
		return fmt.Errorf("opening %s: %w", soPath, err)
	}
	// Plugins compiled with `package main` export the symbol as `Plugin`.
	// Plugins that pick a custom package name (`package foo`) export it
	// as `foo.Plugin`. Try the unqualified form first, then a small set
	// of conventional names, then the slug itself.
	candidates := []string{
		"Plugin",
		"plugin.Plugin",
		// Conventional: plugin authors who name the package after the
		// slug get a `<slug>.Plugin` exported symbol. Title-case the
		// first letter (e.g. "email-smtp" → "Email-smtp.Plugin") since
		// the import path uses lower-case but the package name in
		// the .so does not.
		strings.ToUpper(slug[:1]) + slug[1:] + ".Plugin",
		slug + ".Plugin",
	}
	var sym goplugin.Symbol
	var lastLookupErr error
	for _, name := range candidates {
		s, lookupErr := p.Lookup(name)
		if lookupErr == nil {
			sym = s
			break
		}
		lastLookupErr = lookupErr
	}
	if sym == nil {
		return fmt.Errorf("looking up Plugin symbol (tried %v): %w", candidates, lastLookupErr)
	}
	// The plugin's `var Plugin plugin.Plugin = &EmailSMTPPlugin{}` line
	// means the exported symbol's Go type is `*plugin.Plugin` — a pointer
	// to the interface, not the interface value itself. Plugin authors
	// who declare `var Plugin = &EmailSMTPPlugin{}` get a `**plugin.Plugin`
	// because Go infers `Plugin` from the right-hand side. Handle both.
	var plg plugin.Plugin
	switch v := sym.(type) {
	case plugin.Plugin:
		plg = v
	case *plugin.Plugin:
		if v != nil {
			plg = *v
		}
	default:
		return fmt.Errorf("plugin symbol has unexpected type %T", sym)
	}
	if plg == nil {
		return fmt.Errorf("plugin symbol resolved to nil")
	}

	pluginManifest := plg.Manifest()
	if pluginManifest.Slug != manifest.Slug {
		return fmt.Errorf("manifest slug %q does not match plugin symbol %q", manifest.Slug, pluginManifest.Slug)
	}
	if err := checkVersionAgreement(slug, "", version, manifest.Version, pluginManifest.Version); err != nil {
		return err
	}

	// Capability gate (architecture Â§9). Hard-veto net.bind and
	// fs.write: the host owns the network namespace and the data
	// directory; any plugin claiming otherwise is rejected with
	// plugin.ErrScopeViolation and an audit event.
	if err := enforceCapabilities(slug, manifest.Caps); err != nil {
		if r.host.AuditLog != nil {
			if emitErr := r.host.AuditLog.EmitPluginSeverityContext(ctx, slug, "access.plugin.capability_violation", "", map[string]any{
				"caps": manifest.Caps,
				"err":  err.Error(),
			}); emitErr != nil {
				r.logger.Warn("plugin runtime: writing capability-violation audit row failed", "slug", slug, "error", emitErr)
			}
		}
		return err
	}

	// Clear any prior contribution rows for this slug so re-loading
	// after an update doesn't leave stale route/menu/widget entries.
	if r.tracker != nil {
		if err := r.tracker.ClearAll(ctx, slug); err != nil {
			return fmt.Errorf("clearing prior contributions: %w", err)
		}
		if err := r.recordManifestThemes(ctx, slug, manifest); err != nil {
			return err
		}
		if err := r.recordManifestCatalogs(ctx, slug, manifest); err != nil {
			return err
		}
	}

	// The manifest has to be published BEFORE RegisterRoutes, not with the
	// `loaded` bookkeeping at the end of this function.
	//
	// routeRegistry.mount resolves each route's declared
	// requires_permission through lookupRouteDecl, which reads exactly
	// this map. Publishing the manifest afterwards made every one of those
	// lookups miss, so PluginPermissionMiddleware was constructed with an
	// empty perm — and it short-circuits to a pass-through on empty. The
	// result was that every plugin route in the product was recorded with
	// no permission and enforced none: session-protected, but ungated,
	// however carefully its plugin.yaml declared a key. The empty
	// requires_permission column on every plugin_routes row is the visible
	// symptom.
	//
	// Removed again if the rest of the load fails, so a plugin that never
	// finished loading leaves no contract behind for shutdownOne or a
	// later lookup to find.
	r.mu.Lock()
	r.manifests[slug] = manifest
	r.mu.Unlock()
	defer func() {
		if err != nil {
			r.mu.Lock()
			delete(r.manifests, slug)
			r.mu.Unlock()
		}
	}()

	rr := &routeRegistry{runtime: r, origin: slug}
	if err := plg.RegisterRoutes(rr); err != nil {
		return fmt.Errorf("RegisterRoutes: %w", err)
	}
	if err := plg.RegisterMenu(&menuRegistry{runtime: r, origin: slug}); err != nil {
		return fmt.Errorf("RegisterMenu: %w", err)
	}
	// Optional: a plugin that contributes no page tabs implements nothing.
	if provider, ok := plg.(plugin.TabProvider); ok {
		if err := provider.RegisterTabs(&tabRegistry{runtime: r, origin: slug}); err != nil {
			return fmt.Errorf("RegisterTabs: %w", err)
		}
	}
	// Optional, and deliberately non-fatal: a command set that fails validation
	// is dropped whole and the reason recorded, because a bad declaration must
	// never take a working plugin's routes, pages and widgets offline.
	if provider, ok := plg.(plugin.CLIProvider); ok {
		reg := &cliRegistry{runtime: r, origin: slug, paths: map[string]bool{}}
		if err := provider.RegisterCLI(reg); err != nil || reg.err != nil {
			reason := err
			if reason == nil {
				reason = reg.err
			}
			r.recordCLIError(slug, reason.Error())
			if r.tracker != nil {
				if dropErr := r.tracker.DropCLI(context.WithoutCancel(ctx), slug); dropErr != nil {
					r.logger.Warn("dropping a refused plugin command set failed",
						"origin", slug, "error", dropErr)
				}
			}
			r.releaseCLIRoots(slug)
			r.logger.Warn("plugin CLI commands refused; the plugin itself is unaffected",
				"origin", slug, "reason", reason.Error())
		}
	}
	if err := plg.RegisterWidgets(&widgetRegistry{runtime: r, origin: slug}); err != nil {
		return fmt.Errorf("RegisterWidgets: %w", err)
	}
	if err := plg.RegisterNodes(&nodeRegistry{runtime: r, origin: slug, dir: dir}); err != nil {
		return fmt.Errorf("RegisterNodes: %w", err)
	}
	if err := plg.RegisterPermissions(&accessRegistry{runtime: r, origin: slug}); err != nil {
		return fmt.Errorf("RegisterPermissions: %w", err)
	}

	// Optional: plugins implementing plugin.AdapterContributor register
	// runtime adapters here. The plugin.Plugin interface does not require
	// this method; plugins that don't contribute omit it. We use a
	// type assertion so plugins built against older SDK versions
	// continue to load without recompilation.
	if ac, ok := plg.(plugin.AdapterContributor); ok {
		beforeStorage := r.adapters.storageCount()
		beforeNotification := r.adapters.notificationCount()
		beforeIdentity := r.adapters.identityCount()
		beforeAI := r.adapters.aiCount()
		beforeSnapshot := r.adapters.snapshotCount()
		beforeLogging := r.adapters.loggingCount()
		beforeAlerting := r.adapters.alertingCount()
		beforeReporting := r.adapters.reportingCount()
		beforeScanner := r.adapters.scannerCount()
		// Tag every adapter this plugin registers with its slug so
		// UnloadOne can drop them on uninstall. Single writer —
		// loadOne is never called concurrently with itself — so a
		// plain store is safe (and cheaper than a mutex round-trip).
		r.adapters.origin = slug
		if err := ac.RegisterAdapters(r.adapters); err != nil {
			r.adapters.origin = ""
			return fmt.Errorf("RegisterAdapters: %w", err)
		}
		r.adapters.origin = ""
		storageRegistered := r.adapters.storageCount() - beforeStorage
		notificationRegistered := r.adapters.notificationCount() - beforeNotification
		identityRegistered := r.adapters.identityCount() - beforeIdentity
		aiRegistered := r.adapters.aiCount() - beforeAI
		snapshotRegistered := r.adapters.snapshotCount() - beforeSnapshot
		loggingRegistered := r.adapters.loggingCount() - beforeLogging
		alertingRegistered := r.adapters.alertingCount() - beforeAlerting
		reportingRegistered := r.adapters.reportingCount() - beforeReporting
		scannerRegistered := r.adapters.scannerCount() - beforeScanner
		totalRegistered := storageRegistered + notificationRegistered + identityRegistered + aiRegistered + snapshotRegistered + loggingRegistered + alertingRegistered + reportingRegistered + scannerRegistered
		if totalRegistered == 0 {
			r.logger.Warn("plugin runtime: adapter contributor registered no adapters", "slug", slug)
		} else {
			r.logger.Info("plugin runtime: adapters registered",
				"slug", slug,
				"storage", storageRegistered,
				"notification", notificationRegistered,
				"identity", identityRegistered,
				"ai", aiRegistered,
				"snapshot", snapshotRegistered,
				"logging", loggingRegistered,
				"alerting", alertingRegistered,
				"reporting", reportingRegistered,
				"scanner", scannerRegistered,
			)
		}
	}

	hctx := r.buildHostContext(slug, manifest.TablePrefix, manifest.Perms, agentRPCDeclFor(slug, dir))
	if err := plg.Init(ctx, hctx); err != nil {
		return fmt.Errorf("init: %w", err)
	}
	registerABACResources(slug, plg)
	registerIncidentSource(slug, plg)
	registerDesiredStateSource(slug, plg)

	// Publish the routes only now. Everything above can still fail, and
	// a plugin that never finished loading must not be reachable over
	// HTTP — the registry collected the routes, this is what makes them
	// answer. The call is idempotent: a reload replaces the whole set,
	// so a route the new version dropped stops serving.
	r.deps.SetPluginRoutes(slug, rr.collected)

	r.mu.Lock()
	// r.manifests[slug] was published earlier, before RegisterRoutes — see
	// the comment there. Setting it again here would be harmless but would
	// invite someone to "simplify" by deleting the earlier one, which is
	// the exact bug that ordering fixes.
	r.open[soPath] = p
	r.loaded[slug] = &Loaded{Slug: slug, Version: version, Path: dir, Loaded: true, LoadedAt: time.Now().UTC()}
	r.mu.Unlock()
	r.recordCapabilities(slug, dir)
	r.logger.Info("plugin runtime: loaded", "slug", slug, "version", version, "path", dir)
	return nil
}

func readManifestFile(path string) (plugin.Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return plugin.Manifest{}, fmt.Errorf("reading plugin manifest: %w", err)
	}
	var manifest plugin.Manifest
	if err := yaml.Unmarshal(b, &manifest); err != nil {
		return plugin.Manifest{}, fmt.Errorf("parsing plugin manifest: %w", err)
	}
	if err := plugin.ValidateManifestSchema(&manifest); err != nil {
		return plugin.Manifest{}, err
	}
	// A plugin installed before the slug was reserved still never loads under it.
	if err := pluginslug.Check(manifest.Slug); err != nil {
		return plugin.Manifest{}, err
	}
	return manifest, nil
}

func (r *Runtime) loadManifestOnly(ctx context.Context, slug, version, dir string, manifest plugin.Manifest) error {
	if r.tracker != nil {
		if err := r.tracker.ClearAll(ctx, slug); err != nil {
			return fmt.Errorf("clearing prior contributions: %w", err)
		}
		if err := r.recordManifestThemes(ctx, slug, manifest); err != nil {
			return err
		}
		if err := r.recordManifestCatalogs(ctx, slug, manifest); err != nil {
			return err
		}
	}
	r.mu.Lock()
	r.manifests[slug] = manifest
	r.loaded[slug] = &Loaded{Slug: slug, Version: version, Path: dir, Loaded: true, LoadedAt: time.Now().UTC()}
	r.mu.Unlock()
	r.recordCapabilities(slug, dir)
	r.logger.Info("plugin runtime: loaded manifest-only plugin", "slug", slug, "version", version, "path", dir)
	return nil
}

// recordCapabilities publishes the plugin's contributes.capabilities once it has
// finished loading, so a registry entry always means a reachable plugin. A bad
// declaration is dropped whole and logged: it must never take a working plugin offline.
func (r *Runtime) recordCapabilities(slug, dir string) {
	raw, err := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
	if err == nil {
		var caps []plugincaps.Capability
		if caps, err = plugincaps.Parse(slug, raw); err == nil {
			plugincaps.Default.Set(slug, caps)
			return
		}
	}
	plugincaps.Default.Remove(slug)
	r.logger.Warn("plugin capabilities refused; the plugin itself is unaffected", "origin", slug, "reason", err.Error())
}

func (r *Runtime) recordManifestThemes(ctx context.Context, slug string, manifest plugin.Manifest) error {
	for _, theme := range manifest.Contribs.Themes {
		defaultVariant := theme.DefaultVariant
		if defaultVariant == "" {
			defaultVariant = "dark"
		}
		if err := r.tracker.RecordTheme(ctx, slug, theme.ID, theme.Name, theme.Description, theme.CSSPath, theme.SwatchHue, defaultVariant, theme.HasDark, theme.HasLight); err != nil {
			return fmt.Errorf("recording plugin theme %s: %w", theme.ID, err)
		}
	}
	return nil
}

// recordManifestCatalogs persists the catalog sources the manifest declares.
// Re-run on every boot after ClearAll, so an edited manifest re-points its
// source rather than leaving the old URL behind.
func (r *Runtime) recordManifestCatalogs(ctx context.Context, slug string, manifest plugin.Manifest) error {
	for _, c := range manifest.Contribs.Catalogs {
		if err := r.tracker.RecordCatalog(ctx, slug, c.ID, c.Kind, c.Name, c.URL); err != nil {
			return fmt.Errorf("recording plugin catalog %s: %w", c.ID, err)
		}
	}
	return nil
}

// buildHostContext constructs the plugin.HostContext handed to a plugin
// during Init. Phase A exposes DB, Encryption, Logger, Audit, AI.
// Step 5 (Phase C prerequisite) wires DockerPool so Docker-adjacent plugins
// can drive container ops through the same pool the host uses.
// Step 6 (Phase C-extras, identity plugins) wires Identity and
// Sessions for plugins that authenticate external users.
func (r *Runtime) buildHostContext(slug, tablePrefix string, perms []string, agentRPC *plugincaps.Capability) plugin.HostContext {
	var dockerPool plugin.PoolHandle
	if r.host.DockerPool != nil {
		dockerPool = &dockerPoolAdapter{source: r.host.DockerPool}
	}
	identity := r.identityHandleFor(slug)
	var sessions plugin.SessionIssuer
	if r.host.Sessions != nil {
		sessions = &sessionIssuerAdapter{source: r.host.Sessions}
	}
	return plugin.HostContext{
		Logger:       slogAdapter{runtimeLogger: r.logger, slug: slug},
		DB:           &pluginDB{host: r.host.DB, slug: slug, prefix: pluginTablePrefix(slug, tablePrefix)},
		Encryption:   encryptionAdapter{service: r.host.Encryption},
		Audit:        &auditAdapter{log: r.host.AuditLog, slug: slug},
		Lifecycle:    lifecycleHandleFor(r.host.Lifecycle, slug),
		BackupLog:    backupLogHandleFor(r.host.BackupLog, slug),
		Environments: environmentHandleFor(r.host.Environments),
		AI:           &aiAdapter{source: r.host.AI, slug: slug},
		Storage:      storageHandleFor(r.host.Storage, slug),
		Comms:        commsHandleFor(r.host.Comms, slug),
		Workflows:    workflowHandleFor(r.host.Workflows, slug),
		// RBAC is how a plugin re-checks its *own* permission keys.
		// It matters most for a plugin whose data is reached through
		// another plugin's surface: the report-provider family runs a
		// provider's Build() behind the Reporting plugin's
		// `reporting.generate` gate, which says nothing about whether
		// the caller may read the provider's data. Without this handle
		// a provider has no way to ask, and a report becomes a way
		// around the provider's own read permission.
		RBAC: rbacHandleFor(r.deps, r.labels, slug),
		// The reporting handle reads the loader's own adapter registry
		// rather than a host module bridge: this family's providers and
		// its consumer are both plugins, and the registry they meet in
		// is already in this process.
		Reporting:   reportingHandleFor(r.adapters),
		DockerPool:  dockerPool,
		SnapshotRPC: r.host.SnapshotRPC,
		AgentRPC:    agentRPCFor(r.host.AgentRPC, slug, perms, agentRPC),
		Secrets:     secretsFor(r.host.Secrets, r.deps, slug, perms),
		Catalogs:    r.host.Catalogs,
		Identity:    identity,
		Sessions:    sessions,
		WorkDir:     r.dataDir,
	}
}

// dockerPoolAdapter wraps DockerPoolSource to satisfy plugin.PoolHandle. The
// pool is returned as `any` (typed as *docker.ClientPool on the host
// side) so plugins can call Get / IsAgentEnv through the typed pool
// type without the SDK importing docker directly.
type dockerPoolAdapter struct {
	source DockerPoolSource
}

func (a *dockerPoolAdapter) ForKind(kind string) any {
	if kind != "docker" {
		return nil
	}
	// Return a thin typed accessor instead of the pool itself. Plugins
	// call host.DockerPool.ForKind("docker").(plugin.DockerPoolAccessor) and
	// then .Get / .IsAgentEnv. Keeps the SDK free of docker imports.
	return plugin.NewDockerPoolAccessor(a.source.Get, a.source.IsAgentEnv)
}

// identityProvisionerAdapter converts the loader-level request type
// to the SDK-level one. The split exists so the loader package
// doesn't import the plugin package (which would cycle).
type identityProvisionerAdapter struct {
	source IdentityProvisionerSource
}

func (a *identityProvisionerAdapter) ProvisionUser(ctx context.Context, req plugin.ProvisionRequest) (*plugin.ProvisionedUser, error) {
	out, err := a.source.ProvisionUser(ctx, IdentityProvisionRequest{
		ProviderSlug:   req.ProviderSlug,
		ProviderID:     req.ProviderID,
		Subject:        req.Subject,
		Username:       req.Username,
		Email:          req.Email,
		DisplayName:    req.DisplayName,
		Groups:         req.Groups,
		AutoProvision:  req.AutoProvision,
		DefaultRoleID:  req.DefaultRoleID,
		UsernamePrefix: req.UsernamePrefix,
	})
	if err != nil {
		return nil, err
	}
	return &plugin.ProvisionedUser{
		ID:          out.ID,
		Username:    out.Username,
		DisplayName: out.DisplayName,
		Email:       out.Email,
	}, nil
}

// sessionIssuerAdapter forwards session operations to the host's
// SessionIssuerSource.
type sessionIssuerAdapter struct {
	source SessionIssuerSource
}

func (a *sessionIssuerAdapter) IssueSession(userID string) (string, int, error) {
	return a.source.IssueSession(userID)
}

func (a *sessionIssuerAdapter) SessionCookieName() string {
	return a.source.SessionCookieName()
}

func (a *sessionIssuerAdapter) SessionIsSecure(r *http.Request) bool {
	return a.source.SessionIsSecure(r)
}

// LoadOne hot-loads a single plugin by slug. The installer calls this
// from Install and Enable so the dialog's storage-adapter list, etc.
// picks up the new plugin without requiring a host restart. The slug
// must be present in appstore_plugins with enabled=1 and the .so file
// must already exist at <dataDir>/plugins/<slug>@<version>/backend/<slug>.so.
//
// Loading is idempotent: if the plugin is already loaded, the call
// returns nil without doing anything. If loading fails the plugin's
// Loaded entry is set to {Loaded: false, Err: ...} so the status
// endpoint surfaces the failure to the admin UI.
func (r *Runtime) LoadOne(ctx context.Context, slug string) error {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	if l, ok := r.loaded[slug]; ok && l.Loaded {
		r.mu.RUnlock()
		return nil
	}
	r.mu.RUnlock()

	var version string
	if r.host.DB != nil {
		var v string
		err := r.host.DB.QueryRowContext(ctx,
			`SELECT version FROM appstore_plugins WHERE slug = ?`, slug).Scan(&v)
		if err != nil {
			return fmt.Errorf("resolving version for %s: %w", slug, err)
		}
		version = v
	}
	dir := filepath.Join(r.dataDir, "plugins", slug+"@"+version)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("plugin directory missing: %w", err)
	}

	if err := r.loadOne(ctx, slug, version, dir); err != nil {
		r.mu.Lock()
		r.loaded[slug] = &Loaded{Slug: slug, Version: version, Path: dir, Loaded: false, Err: err.Error()}
		r.mu.Unlock()
		return err
	}
	return nil
}

// UnloadOne drops the runtime state for a single plugin. The .so
// stays dlopen'd — goplugin doesn't expose a release handle — but
// the adapter maps are pruned so a subsequent plugin listing no
// longer surfaces this slug. The installer calls this from Uninstall.
func (r *Runtime) UnloadOne(_ context.Context, slug string) error {
	if r == nil {
		return nil
	}
	r.adapters.removeFor(slug)
	rbac.UnregisterResourceTypes(slug)
	incidentsources.Unregister(slug)
	desiredstatesources.Unregister(slug)
	plugincaps.Default.Remove(slug)
	if r.host.AgentRPC != nil {
		r.host.AgentRPC.Unsubscribe(slug)
	}
	// Drop the plugin's routes from the live router. goplugin cannot
	// unload the .so, but the handlers must stop answering the moment
	// the plugin is disabled or uninstalled — leaving them mounted was
	// the mirror image of the bug that made a freshly installed plugin
	// serve nothing.
	r.deps.SetPluginRoutes(slug, nil)
	r.mu.Lock()
	delete(r.loaded, slug)
	delete(r.manifests, slug)
	prefix := workflowNodeActionKey(slug, "")
	for key := range r.nodeExecutors {
		if strings.HasPrefix(key, prefix) {
			delete(r.nodeExecutors, key)
		}
	}
	r.mu.Unlock()
	return nil
}

// workflowNodeActionKey is the canvas action the SPA assigns a plugin
// node: `plugin:<slug>:<key>` (see usePluginNodes.ts). Both ends must
// agree on it or the engine never finds the executor.
func workflowNodeActionKey(slug, key string) string {
	return "plugin:" + slug + ":" + key
}

// HasWorkflowNode implements plugin.WorkflowNodeExecutorSource.
func (r *Runtime) HasWorkflowNode(actionKey string) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.nodeExecutors[actionKey]
	return ok
}

// ExecuteWorkflowNode implements plugin.WorkflowNodeExecutorSource.
// A panic inside a plugin executor is recovered here so one faulty
// node fails its own step instead of taking the host's workflow
// engine down with it.
func (r *Runtime) ExecuteWorkflowNode(ctx context.Context, actionKey string, in plugin.NodeExecInput) (out plugin.NodeExecOutput, err error) {
	if r == nil {
		return plugin.NodeExecOutput{}, fmt.Errorf("plugin runtime: not booted")
	}
	r.mu.RLock()
	exec, ok := r.nodeExecutors[actionKey]
	r.mu.RUnlock()
	if !ok || exec == nil {
		return plugin.NodeExecOutput{}, fmt.Errorf("plugin runtime: no executor for workflow node %q", actionKey)
	}
	defer func() {
		if rec := recover(); rec != nil {
			r.logger.Error("plugin runtime: workflow node executor panicked", "action", actionKey, "panic", fmt.Sprint(rec))
			out = plugin.NodeExecOutput{}
			err = fmt.Errorf("plugin runtime: workflow node %q panicked", actionKey)
		}
	}()
	return exec(ctx, in)
}

// Shutdown runs each loaded plugin's Shutdown(ctx) with a 30s
// deadline. Phase A is best-effort: goplugin doesn't expose a way
// to re-look up symbols after Open without keeping the plugin
// handle, so real per-plugin shutdown is deferred until Step 5.
func (r *Runtime) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	slugs := make([]string, 0, len(r.loaded))
	for s, l := range r.loaded {
		if l.Loaded {
			slugs = append(slugs, s)
		}
	}
	r.mu.Unlock()

	var firstErr error
	for _, s := range slugs {
		shutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := r.shutdownOne(shutCtx, s)
		cancel()
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (r *Runtime) shutdownOne(_ context.Context, slug string) error {
	r.mu.RLock()
	manifest, ok := r.manifests[slug]
	r.mu.RUnlock()
	if !ok {
		return nil
	}
	r.logger.Info("plugin runtime: shutting down plugin", "slug", slug, "name", manifest.Name)
	return nil
}

// Loaded returns a copy of the loaded-plugin map for the diagnostic
// route handler. Never returns nil.
func (r *Runtime) Loaded() map[string]Loaded {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]Loaded, len(r.loaded))
	for k, v := range r.loaded {
		if v != nil {
			out[k] = *v
		}
	}
	return out
}

// Rescan re-syncs the runtime with the on-disk + database state.
// It's the hot-reload entry point called by:
//
//   - the installer's onChange callback after a successful install
//     (so a fresh plugin's adapters / menu / widgets / routes
//     appear without a container restart)
//   - the /api/plugins/loader/reload endpoint (admin-triggered
//     rescan, useful after manual file copies or as a recovery
//     step)
//
// Algorithm:
//
//  1. Read the current set of (slug → version) rows from
//     appstore_plugins (single SELECT, no filtering — the loader
//     loads every installed plugin regardless of enabled, and
//     UnloadOne is safe to call for non-loaded slugs).
//
//  2. For each (slug, version) pair present in the DB but not yet
//     loaded: call LoadOne(slug). This is a no-op if the .so is
//     missing or its dlopen fails — the error is recorded in
//     r.loaded[slug].Err and the next status call surfaces it.
//
//  3. For each slug present in the runtime but no longer in the
//     DB: call UnloadOne(slug) to drop its adapter / menu /
//     widgets / routes / ABAC rows.
//
// Per-plugin errors are logged and accumulated; the function
// returns the first non-nil one so the caller (the /reload handler
// or the installer callback) can surface a failure. The runtime
// is left in a partially-loaded state on failure — every
// successful plugin stays loaded, every failed one is recorded
// with the diagnostic the next /plugins/loader/status call exposes.
func (r *Runtime) Rescan(ctx context.Context) error {
	return r.reconcileInstalled(ctx)
}

// desiredPlugins reads the slug-to-version set the store says should be loaded, releasing the cursor before any plugin loads.
func (r *Runtime) desiredPlugins(ctx context.Context) (map[string]string, error) {
	desired := make(map[string]string)
	// No LIMIT: the desired set for reconciliation; see loadInstalledSlugs.
	rows, err := r.host.DB.QueryContext(ctx,
		`SELECT slug, version FROM appstore_plugins`)
	if err != nil {
		return nil, fmt.Errorf("querying installed plugins: %w", err)
	}
	defer closeRows(r.logger, rows, "installed-slug")
	for rows.Next() {
		var s, v string
		if scanErr := rows.Scan(&s, &v); scanErr != nil {
			return nil, fmt.Errorf("scanning installed plugin row: %w", scanErr)
		}
		desired[s] = v
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("iterating installed plugins: %w", rowsErr)
	}
	return desired, nil
}

// reconcileInstalled reconciles the loaded plugin set against the store, releasing the installed-slug cursor before any load starts.
func (r *Runtime) reconcileInstalled(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if r.host.DB == nil {
		return nil
	}

	// 1. Snapshot the desired set.
	desired, err := r.desiredPlugins(ctx)
	if err != nil {
		return err
	}

	// 2. Load anything new. Use the existing per-slug LoadOne so
	// the error-recording path is identical to boot-time loads.
	r.mu.RLock()
	loadedSlugs := make(map[string]string, len(r.loaded))
	for s, l := range r.loaded {
		if l != nil {
			loadedSlugs[s] = l.Version
		}
	}
	r.mu.RUnlock()

	var firstErr error
	for slug, version := range desired {
		if curVer, ok := loadedSlugs[slug]; ok && curVer == version {
			continue
		}
		if err := r.LoadOne(ctx, slug); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("loading %s: %w", slug, err)
		}
	}

	// 3. Unload anything that's no longer in the DB.
	for slug := range loadedSlugs {
		if _, keep := desired[slug]; keep {
			continue
		}
		if err := r.UnloadOne(ctx, slug); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("unloading %s: %w", slug, err)
		}
	}

	if firstErr != nil {
		r.logger.Error("plugin runtime: rescan failed", "error", firstErr.Error())
	}
	return firstErr
}

// StorageAdapter returns the registered storage adapter for the
// given location type, along with its spec and a boolean reporting
// whether the adapter is registered. The host's storage dispatcher
// calls this before each Test/Upload/Download/Delete/List to decide
// whether to delegate to a plugin or fall back to the built-in
// implementation. Local destinations never have a plugin adapter â€”
// the host retains built-in local handling per the architecture
// reversal of 2026-08-01.
func (r *Runtime) StorageAdapter(locationType string) (plugin.StorageAdapterClient, plugin.StorageAdapterSpec, bool) {
	if r.adapters == nil {
		return nil, plugin.StorageAdapterSpec{}, false
	}
	return r.adapters.storage(locationType)
}

// BackupProviders returns every registered backup provider.
//
// Satisfies host.BackupProviderSource. Returns nothing rather than an error
// on a runtime with no adapters: a host with no backup plugins installed has
// a Backups page showing its own container backups, which is correct.
func (r *Runtime) BackupProviders() []host.BackupProviderEntry {
	if r.adapters == nil {
		return nil
	}
	return r.adapters.backupProviders()
}

// StorageAdapterSpecs returns a copy of all registered storage adapter
// specs, keyed by LocationType. The host uses this to render the
// admin UI's destination type dropdown with plugin-contributed types
// alongside the built-in ones.
func (r *Runtime) StorageAdapterSpecs() map[string]plugin.StorageAdapterSpec {
	if r.adapters == nil {
		return map[string]plugin.StorageAdapterSpec{}
	}
	return r.adapters.specs()
}

// NotificationAdapter returns the registered notification adapter
// for the given channel type, along with its spec and a boolean
// reporting whether the adapter is registered. The host's
// communications dispatcher calls this before Test/Send to decide
// whether to delegate to a plugin or fall back to the built-in
// switch. The "internal" channel type always uses the host's
// built-in broadcaster; the registry rejects plugins that try to
// register for that type.
func (r *Runtime) NotificationAdapter(channelType string) (plugin.NotificationAdapterClient, plugin.NotificationAdapterSpec, bool) {
	if r.adapters == nil {
		return nil, plugin.NotificationAdapterSpec{}, false
	}
	return r.adapters.notification(channelType)
}

// NotificationAdapterSpecs returns a copy of all registered
// notification adapter specs, keyed by ChannelType. The host uses
// this to render the channel_type dropdown in the admin UI with
// plugin-contributed entries alongside the built-in ones.
func (r *Runtime) NotificationAdapterSpecs() map[string]plugin.NotificationAdapterSpec {
	if r.adapters == nil {
		return map[string]plugin.NotificationAdapterSpec{}
	}
	return r.adapters.notificationSpecs()
}

// IdentityAdapter returns the registered identity adapter for the
// given provider type, along with its spec and a boolean reporting
// whether the adapter is registered. The host's identity module
// dispatcher calls this from the public /authorize and /callback
// endpoints to delegate per-provider-type runtime work to the
// plugin that owns the ProviderType.
func (r *Runtime) IdentityAdapter(providerType string) (plugin.IdentityAdapterClient, plugin.IdentityAdapterSpec, bool) {
	if r.adapters == nil {
		return nil, plugin.IdentityAdapterSpec{}, false
	}
	return r.adapters.identity(providerType)
}

// IdentityAdapterSpecs returns a copy of all registered identity
// adapter specs, keyed by ProviderType. The host uses this to render
// the provider_type dropdown in the admin UI with plugin-contributed
// entries alongside the built-in ones.
func (r *Runtime) IdentityAdapterSpecs() map[string]plugin.IdentityAdapterSpec {
	if r.adapters == nil {
		return map[string]plugin.IdentityAdapterSpec{}
	}
	return r.adapters.identitySpecs()
}

// AIAdapter returns the AIAdapterClient registered against the given
// provider_type (e.g. "anthropic", "openai", "ollama", "mistral").
// modules/ai_providers and modules/ai_assistant call this to dispatch
// Test / Complete / Models to the plugin that owns the provider.
// Returns the client, its spec, and a bool reporting whether a
// plugin registered for this type. The `ai_assistant` bridge casts
// against the aiRuntimeView interface declared in
// modules/ai_assistant/plugin_dispatch_bridge.go:24, so the return
// shape (concrete types, not `any`) is required to satisfy that
// assertion.
func (r *Runtime) AIAdapter(providerType string) (plugin.AIAdapterClient, plugin.AIAdapterSpec, bool) {
	if r.adapters == nil {
		return nil, plugin.AIAdapterSpec{}, false
	}
	cli, spec, ok := r.adapters.ai(providerType)
	if !ok {
		return nil, spec, false
	}
	return cli, spec, true
}

// AIAdapterSpecs returns a copy of all registered AI provider adapter
// specs, keyed by ProviderType. The host uses this to render the
// provider_type dropdown in the AI admin UI and the chat composer
// with plugin-contributed entries alongside the built-in ones.
func (r *Runtime) AIAdapterSpecs() map[string]plugin.AIAdapterSpec {
	if r.adapters == nil {
		return map[string]plugin.AIAdapterSpec{}
	}
	return r.adapters.aiSpecs()
}

// SnapshotAdapter resolves a snapshot adapter for the given driver.
// Returns (nil, zero-spec, false) when no plugin has registered the
// driver. Satisfies plugin.SnapshotAdapterSource so consumers
// (container_backups, settings) can type-assert the runtime to the
// SDK source interface and reach into it without depending on this
// package's internals.
func (r *Runtime) SnapshotAdapter(driver string) (plugin.SnapshotAdapterClient, plugin.SnapshotAdapterSpec, bool) {
	if r.adapters == nil {
		return nil, plugin.SnapshotAdapterSpec{}, false
	}
	cli, spec, ok := r.adapters.snapshot(driver)
	if !ok {
		return nil, spec, false
	}
	return cli, spec, true
}

// SnapshotAdapterSpecs returns a copy of all registered snapshot
// adapter specs, keyed by Driver. The host uses this to render the
// driver dropdown in the Snapshots settings tab and the backup
// plan editor.
func (r *Runtime) SnapshotAdapterSpecs() map[string]plugin.SnapshotAdapterSpec {
	if r.adapters == nil {
		return map[string]plugin.SnapshotAdapterSpec{}
	}
	return r.adapters.snapshotSpecs()
}

// LogForwarderAdapter resolves an audit log forwarder by its
// ForwarderType ("siem", "syslog"). Returns (nil, zero-spec, false)
// when no plugin has registered it. Satisfies the by-key half of
// host.LogForwarderAdapterSource.
func (r *Runtime) LogForwarderAdapter(forwarderType string) (plugin.LogForwarderAdapterClient, plugin.LogForwarderAdapterSpec, bool) {
	if r.adapters == nil {
		return nil, plugin.LogForwarderAdapterSpec{}, false
	}
	cli, spec, ok := r.adapters.logging(forwarderType)
	if !ok {
		return nil, spec, false
	}
	return cli, spec, true
}

// LogForwarderAdapters enumerates every registered forwarder spec in
// registration order. The audit emit path uses it to fan out; the
// Logging settings tab uses it to render one tile per installed
// plugin. Satisfies the enumeration half of
// host.LogForwarderAdapterSource.
func (r *Runtime) LogForwarderAdapters() []plugin.LogForwarderAdapterSpec {
	if r.adapters == nil {
		return nil
	}
	return r.adapters.loggingSpecList()
}

// ImageScannerAdapter resolves an image vulnerability scanner by its
// ScannerType ("trivy", "grype", "clair"). Returns (nil, zero-spec,
// false) when no plugin has registered it — the normal state for a
// `scanner_configs` row whose plugin was uninstalled. Satisfies the
// by-key half of host.ImageScannerAdapterSource.
func (r *Runtime) ImageScannerAdapter(scannerType string) (plugin.ImageScannerAdapterClient, plugin.ImageScannerAdapterSpec, bool) {
	if r.adapters == nil {
		return nil, plugin.ImageScannerAdapterSpec{}, false
	}
	cli, spec, ok := r.adapters.scanner(scannerType)
	if !ok {
		return nil, spec, false
	}
	return cli, spec, true
}

// ImageScannerAdapters enumerates every registered scanner spec in
// registration order. The Security > Scanners tab uses it to render
// one Add-picker tile per installed engine. Satisfies the enumeration
// half of host.ImageScannerAdapterSource.
func (r *Runtime) ImageScannerAdapters() []plugin.ImageScannerAdapterSpec {
	if r.adapters == nil {
		return nil
	}
	return r.adapters.scannerSpecList()
}

// AlertSourceAdapter resolves the alert source that owns one
// SourceType. Returns (nil, zero, false) when no plugin has
// registered it - the normal state for a rule whose plugin was
// uninstalled. Satisfies the lookup half of
// host.AlertSourceAdapterSource.
func (r *Runtime) AlertSourceAdapter(sourceType string) (plugin.AlertSourceAdapterClient, plugin.AlertSourceAdapterSpec, bool) {
	if r.adapters == nil {
		return nil, plugin.AlertSourceAdapterSpec{}, false
	}
	cli, spec, ok := r.adapters.alerting(sourceType)
	if !ok {
		return nil, spec, false
	}
	return cli, spec, true
}

// AlertSourceAdapters enumerates every registered alert source spec in
// registration order. The alerts module reads it to build the
// condition catalog the rule builder renders. Satisfies the
// enumeration half of host.AlertSourceAdapterSource.
func (r *Runtime) AlertSourceAdapters() []plugin.AlertSourceAdapterSpec {
	if r.adapters == nil {
		return nil
	}
	return r.adapters.alertingSpecList()
}

// ReportProviders returns every registered report provider spec in
// registration order. The Reporting plugin reads it to merge
// plugin-contributed report types into its own catalog.
func (r *Runtime) ReportProviders() []plugin.ReportProviderSpec {
	if r.adapters == nil {
		return nil
	}
	return r.adapters.reportingSpecList()
}

// ReportProvider resolves the provider that owns one report type key.
// Returns (nil, false) when no plugin has registered it.
func (r *Runtime) ReportProvider(typeKey string) (plugin.ReportProviderClient, bool) {
	if r.adapters == nil {
		return nil, false
	}
	return r.adapters.reportingFor(typeKey)
}

// --- adapter registry ---------------------------------------------

// newAdapterRegistry returns an empty registry. The loader creates
// one per Runtime and threads it through to plugins that implement
// plugin.AdapterContributor.
func newAdapterRegistry() *adapterRegistry {
	return &adapterRegistry{
		backupClients:       make(map[string]plugin.BackupProviderClient),
		backupSpecs:         make(map[string]plugin.BackupProviderSpec),
		storageClients:      make(map[string]plugin.StorageAdapterClient),
		storageSpecs:        make(map[string]plugin.StorageAdapterSpec),
		storageOrigin:       make(map[string]string),
		notificationClients: make(map[string]plugin.NotificationAdapterClient),
		notificationSpecMap: make(map[string]plugin.NotificationAdapterSpec),
		notificationOrigin:  make(map[string]string),
		identityClients:     make(map[string]plugin.IdentityAdapterClient),
		identitySpecMap:     make(map[string]plugin.IdentityAdapterSpec),
		identityOrigin:      make(map[string]string),
		aiClients:           make(map[string]plugin.AIAdapterClient),
		aiSpecMap:           make(map[string]plugin.AIAdapterSpec),
		aiOrigin:            make(map[string]string),
		snapshotClients:     make(map[string]plugin.SnapshotAdapterClient),
		snapshotSpecMap:     make(map[string]plugin.SnapshotAdapterSpec),
		snapshotOrigin:      make(map[string]string),
		loggingClients:      make(map[string]plugin.LogForwarderAdapterClient),
		loggingSpecMap:      make(map[string]plugin.LogForwarderAdapterSpec),
		loggingOrigin:       make(map[string]string),
		alertingClients:     make(map[string]plugin.AlertSourceAdapterClient),
		alertingSpecMap:     make(map[string]plugin.AlertSourceAdapterSpec),
		alertingOrigin:      make(map[string]string),
		reportingClients:    make(map[string]plugin.ReportProviderClient),
		reportingSpecMap:    make(map[string]plugin.ReportProviderSpec),
		reportingTypeOwner:  make(map[string]string),
		scannerClients:      make(map[string]plugin.ImageScannerAdapterClient),
		scannerSpecMap:      make(map[string]plugin.ImageScannerAdapterSpec),
		scannerOrigin:       make(map[string]string),
	}
}

// adapterRegistry is the loader-side implementation of
// AdapterRegistry. It is intentionally not exported; plugins only
// see it through the typed sub-registry interfaces.
//
// The origin* maps track which plugin slug registered each adapter
// key, so UnloadOne(slug) can drop every entry the plugin contributed
// without affecting adapters registered by other plugins that happen
// to share the same key (extremely unlikely in practice — Register
// already rejects duplicates — but the bookkeeping is cheap).
//
// The `origin` field is the slug of the plugin currently inside
// RegisterAdapters; loadOne sets it before calling the plugin's hook
// and the sub-registry captures it at construction time. Only one
// plugin is inside RegisterAdapters at a time so a plain (non-mutex)
// read is safe.
type adapterRegistry struct {
	mu     sync.RWMutex
	origin string

	// tracker and logger let adapter registration mirror declared
	// capabilities into the contribution tables. Both may be nil —
	// the in-memory maps below are the authority for dispatch, and a
	// missing tracker only costs the out-of-process view.
	tracker ContributionTracker
	logger  *slog.Logger

	// audit wraps every client a plugin registers so the work the host
	// asks that plugin to do leaves a trail. May be nil (tests, hosts
	// booted without an audit logger); the wrappers then step aside and
	// the bare client is stored. See adapter_audit.go.
	audit *adapterAuditor

	// Backup providers are held in registration order rather than keyed by
	// a dispatch value, because the Backups page asks every one of them
	// rather than resolving one — the opposite access shape from the other
	// families here.
	backupClients map[string]plugin.BackupProviderClient
	backupSpecs   map[string]plugin.BackupProviderSpec
	backupOrder   []string

	storageClients      map[string]plugin.StorageAdapterClient
	storageSpecs        map[string]plugin.StorageAdapterSpec
	storageOrigin       map[string]string
	notificationClients map[string]plugin.NotificationAdapterClient
	notificationSpecMap map[string]plugin.NotificationAdapterSpec
	notificationOrigin  map[string]string
	identityClients     map[string]plugin.IdentityAdapterClient
	identitySpecMap     map[string]plugin.IdentityAdapterSpec
	identityOrigin      map[string]string
	aiClients           map[string]plugin.AIAdapterClient
	aiSpecMap           map[string]plugin.AIAdapterSpec
	aiOrigin            map[string]string
	snapshotClients     map[string]plugin.SnapshotAdapterClient
	snapshotSpecMap     map[string]plugin.SnapshotAdapterSpec
	snapshotOrigin      map[string]string
	loggingClients      map[string]plugin.LogForwarderAdapterClient
	loggingSpecMap      map[string]plugin.LogForwarderAdapterSpec
	loggingOrigin       map[string]string
	alertingClients     map[string]plugin.AlertSourceAdapterClient
	alertingSpecMap     map[string]plugin.AlertSourceAdapterSpec
	alertingOrigin      map[string]string
	// The reporting family is keyed by provider slug rather than by
	// dispatch key, because one provider answers several report types.
	// reportingTypeOwner is the inverted index the Reporting plugin's
	// dispatch actually uses: report type key -> provider slug. Both
	// are pruned together in removeFor.
	reportingClients   map[string]plugin.ReportProviderClient
	reportingSpecMap   map[string]plugin.ReportProviderSpec
	reportingTypeOwner map[string]string
	// reportingFacetClients and reportingFacetOwner are the facet half:
	// provider slug -> contributor, and "facet/platform" -> provider slug.
	reportingFacetClients map[string]plugin.ReportFacetContributor
	reportingFacetOwner   map[string]string
	// reportingOrder preserves registration order so the studio's
	// shelves do not reshuffle between renders, for the same reason
	// loggingOrder exists.
	reportingOrder []string

	// alertingOrder preserves registration order for the same reason
	// loggingOrder does: the alert engine enumerates every source to
	// build the condition catalog the rule builder renders, and an
	// unstable order would reshuffle the condition picker's groups
	// between renders.
	alertingOrder []string

	// loggingOrder preserves registration order. The other families
	// are dispatched by key lookup, so map iteration order never
	// shows; logging fans every audit event out to all forwarders,
	// and an unstable order would make the Logging tab's tile list
	// reshuffle on every render.
	loggingOrder []string

	scannerClients map[string]plugin.ImageScannerAdapterClient
	scannerSpecMap map[string]plugin.ImageScannerAdapterSpec
	scannerOrigin  map[string]string
	// scannerOrder preserves registration order for the same reason
	// loggingOrder does: the Scanners tab enumerates every installed
	// engine to build its Add picker, and tiles that reshuffle
	// between renders make an operator distrust the list.
	scannerOrder []string
}

// Storage implements AdapterRegistry.
func (a *adapterRegistry) Storage() plugin.StorageAdapterRegistry {
	return &storageAdapterRegistry{reg: a, origin: a.origin}
}

// Notification implements AdapterRegistry.
func (a *adapterRegistry) Notification() plugin.NotificationAdapterRegistry {
	return &notificationAdapterRegistry{reg: a, origin: a.origin}
}

// Identity implements AdapterRegistry.
func (a *adapterRegistry) Identity() plugin.IdentityAdapterRegistry {
	return &identityAdapterRegistry{reg: a, origin: a.origin}
}

// AI implements AdapterRegistry.
func (a *adapterRegistry) AI() plugin.AIAdapterRegistry {
	return &aiAdapterRegistry{reg: a, origin: a.origin}
}

// Snapshot implements AdapterRegistry.
func (a *adapterRegistry) Snapshot() plugin.SnapshotAdapterRegistry {
	return &snapshotAdapterRegistry{reg: a, origin: a.origin}
}

// Logging implements AdapterRegistry.
func (a *adapterRegistry) Logging() plugin.LogForwarderAdapterRegistry {
	return &logForwarderAdapterRegistry{reg: a, origin: a.origin}
}

// Alerting implements AdapterRegistry.
func (a *adapterRegistry) Alerting() plugin.AlertSourceAdapterRegistry {
	return &alertSourceAdapterRegistry{reg: a, origin: a.origin}
}

// Reporting implements AdapterRegistry.
func (a *adapterRegistry) Reporting() plugin.ReportAdapterRegistry {
	return &reportingAdapterRegistry{reg: a, origin: a.origin}
}

// Scanner implements AdapterRegistry.
func (a *adapterRegistry) Scanner() plugin.ImageScannerAdapterRegistry {
	return &imageScannerAdapterRegistry{reg: a, origin: a.origin}
}

// Backup implements AdapterRegistry.
func (a *adapterRegistry) Backup() plugin.BackupAdapterRegistry {
	return &backupAdapterRegistry{reg: a, origin: a.origin}
}

// backupAdapterRegistry is the per-category sub-registry handed to a plugin.
type backupAdapterRegistry struct {
	reg    *adapterRegistry
	origin string
}

// Register implements BackupAdapterRegistry.
//
// One provider per plugin slug. Re-registering the same slug is an error
// rather than an overwrite: the page attributes every row it shows to a
// provider, and a silent replacement would relabel history that the previous
// registrant produced.
func (b *backupAdapterRegistry) Register(spec plugin.BackupProviderSpec, client plugin.BackupProviderClient) error {
	if spec.Slug == "" {
		return errors.New("backup provider: Slug is required")
	}
	if spec.SubjectKind == "" {
		return fmt.Errorf("backup provider %s: SubjectKind is required", spec.Slug)
	}
	if spec.DisplayName == "" {
		return fmt.Errorf("backup provider %s: DisplayName is required", spec.Slug)
	}
	if client == nil {
		return fmt.Errorf("backup provider %s: client is nil", spec.Slug)
	}

	b.reg.mu.Lock()
	defer b.reg.mu.Unlock()
	if _, exists := b.reg.backupClients[spec.Slug]; exists {
		return fmt.Errorf("backup provider %s: already registered", spec.Slug)
	}
	b.reg.backupClients[spec.Slug] = client
	b.reg.backupSpecs[spec.Slug] = spec
	b.reg.backupOrder = append(b.reg.backupOrder, spec.Slug)
	return nil
}

// backupProviders returns every registered provider in registration order.
func (a *adapterRegistry) backupProviders() []host.BackupProviderEntry {
	a.mu.RLock()
	defer a.mu.RUnlock()

	out := make([]host.BackupProviderEntry, 0, len(a.backupOrder))
	for _, slug := range a.backupOrder {
		client, ok := a.backupClients[slug]
		if !ok {
			continue
		}
		out = append(out, host.BackupProviderEntry{
			Spec:   a.backupSpecs[slug],
			Client: client,
		})
	}
	return out
}

// storageCount returns the number of currently-registered storage
// adapters. The loader uses this to detect plugins that returned
// from RegisterAdapters without registering anything (a configuration
// bug worth logging).
func (a *adapterRegistry) storageCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.storageClients)
}

// notificationCount returns the number of currently-registered
// notification adapters. The loader uses this to detect plugins
// that returned from RegisterAdapters without registering anything.
func (a *adapterRegistry) notificationCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.notificationClients)
}

// identityCount returns the number of currently-registered identity
// adapters. The loader uses this to detect plugins that returned
// from RegisterAdapters without registering anything.
func (a *adapterRegistry) identityCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.identityClients)
}

// aiCount returns the number of currently-registered AI provider
// adapters. The loader uses this to detect plugins that returned
// from RegisterAdapters without registering anything.
func (a *adapterRegistry) aiCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.aiClients)
}

// snapshotCount returns the number of currently-registered snapshot
// adapters. The loader uses this to detect plugins that returned
// from RegisterAdapters without registering anything.
func (a *adapterRegistry) snapshotCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.snapshotClients)
}

// alertingCount returns the number of currently-registered alert
// sources. The loader uses this to detect plugins that returned from
// RegisterAdapters without registering anything.
func (a *adapterRegistry) alertingCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.alertingClients)
}

// loggingCount returns the number of currently-registered log
// forwarder adapters. The loader uses this to detect plugins that
// returned from RegisterAdapters without registering anything.
func (a *adapterRegistry) loggingCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.loggingClients)
}

// reportingCount returns the number of currently-registered report
// providers. The loader uses this to detect plugins that returned from
// RegisterAdapters without registering anything.
func (a *adapterRegistry) reportingCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.reportingClients)
}

// scannerCount returns the number of currently-registered image
// scanner adapters. The loader uses this to detect plugins that
// returned from RegisterAdapters without registering anything.
func (a *adapterRegistry) scannerCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.scannerClients)
}

// removeFor drops every adapter entry the named plugin contributed.
// The plugin's .so stays dlopen'd — goplugin doesn't expose a release
// handle — but the in-memory maps are pruned so a subsequent
// listing (StorageAdapterSpecs, etc.) no longer surfaces the plugin's
// LocationType / channelType / providerType / serverType. The
// installer calls this from Uninstall.
func (a *adapterRegistry) removeFor(slug string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for k, o := range a.storageOrigin {
		if o == slug {
			delete(a.storageClients, k)
			delete(a.storageSpecs, k)
			delete(a.storageOrigin, k)
		}
	}
	for k, o := range a.notificationOrigin {
		if o == slug {
			delete(a.notificationClients, k)
			delete(a.notificationSpecMap, k)
			delete(a.notificationOrigin, k)
		}
	}
	for k, o := range a.identityOrigin {
		if o == slug {
			delete(a.identityClients, k)
			delete(a.identitySpecMap, k)
			delete(a.identityOrigin, k)
		}
	}
	for k, o := range a.aiOrigin {
		if o == slug {
			delete(a.aiClients, k)
			delete(a.aiSpecMap, k)
			delete(a.aiOrigin, k)
		}
	}
	for k, o := range a.snapshotOrigin {
		if o == slug {
			delete(a.snapshotClients, k)
			delete(a.snapshotSpecMap, k)
			delete(a.snapshotOrigin, k)
		}
	}
	for k, o := range a.loggingOrigin {
		if o == slug {
			delete(a.loggingClients, k)
			delete(a.loggingSpecMap, k)
			delete(a.loggingOrigin, k)
			a.loggingOrder = slices.DeleteFunc(a.loggingOrder, func(t string) bool { return t == k })
		}
	}
	for k, o := range a.alertingOrigin {
		if o == slug {
			delete(a.alertingClients, k)
			delete(a.alertingSpecMap, k)
			delete(a.alertingOrigin, k)
			a.alertingOrder = slices.DeleteFunc(a.alertingOrder, func(t string) bool { return t == k })
		}
	}
	// Only the engine goes. `scanner_configs` rows naming this type
	// stay in the host's database and keep listing as unavailable, so
	// reinstalling the plugin restores a working scanner rather than
	// asking the operator to reconstruct a config they never chose to
	// delete.
	for k, o := range a.scannerOrigin {
		if o == slug {
			delete(a.scannerClients, k)
			delete(a.scannerSpecMap, k)
			delete(a.scannerOrigin, k)
			a.scannerOrder = slices.DeleteFunc(a.scannerOrder, func(t string) bool { return t == k })
		}
	}
	// The reporting family is keyed by provider slug, so the origin is
	// the key. The inverted type index has to be swept separately or an
	// uninstalled provider's report types would keep resolving to a
	// client that no longer exists.
	if _, ok := a.reportingClients[slug]; ok {
		delete(a.reportingClients, slug)
		delete(a.reportingSpecMap, slug)
		a.reportingOrder = slices.DeleteFunc(a.reportingOrder, func(t string) bool { return t == slug })
	}
	for typeKey, owner := range a.reportingTypeOwner {
		if owner == slug {
			delete(a.reportingTypeOwner, typeKey)
		}
	}
	a.pruneFacets(slug)
}

// storage is the runtime-side accessor. Returns the client + spec
// + boolean (true when registered).
func (a *adapterRegistry) storage(locationType string) (plugin.StorageAdapterClient, plugin.StorageAdapterSpec, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	cli, ok := a.storageClients[locationType]
	if !ok {
		return nil, plugin.StorageAdapterSpec{}, false
	}
	return cli, a.storageSpecs[locationType], true
}

// specs returns a copy of the registered specs. Safe for concurrent
// use; callers can mutate the returned map without affecting the
// registry.
func (a *adapterRegistry) specs() map[string]plugin.StorageAdapterSpec {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make(map[string]plugin.StorageAdapterSpec, len(a.storageSpecs))
	maps.Copy(out, a.storageSpecs)
	return out
}

// notification is the runtime-side accessor for notification
// adapters. Returns the client + spec + boolean.
func (a *adapterRegistry) notification(channelType string) (plugin.NotificationAdapterClient, plugin.NotificationAdapterSpec, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	cli, ok := a.notificationClients[channelType]
	if !ok {
		return nil, plugin.NotificationAdapterSpec{}, false
	}
	return cli, a.notificationSpecMap[channelType], true
}

// notificationSpecs returns a copy of all registered notification
// adapter specs. The host uses this to render the channel_type
// dropdown in the admin UI with plugin-contributed entries alongside
// built-in ones.
func (a *adapterRegistry) notificationSpecs() map[string]plugin.NotificationAdapterSpec {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make(map[string]plugin.NotificationAdapterSpec, len(a.notificationSpecMap))
	maps.Copy(out, a.notificationSpecMap)
	return out
}

// identity is the runtime-side accessor for identity adapters.
// Returns the client + spec + boolean.
func (a *adapterRegistry) identity(providerType string) (plugin.IdentityAdapterClient, plugin.IdentityAdapterSpec, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	cli, ok := a.identityClients[providerType]
	if !ok {
		return nil, plugin.IdentityAdapterSpec{}, false
	}
	return cli, a.identitySpecMap[providerType], true
}

// identitySpecs returns a copy of all registered identity adapter
// specs. The host uses this to render the provider_type dropdown in
// the admin UI with plugin-contributed entries alongside any
// built-in ones.
func (a *adapterRegistry) identitySpecs() map[string]plugin.IdentityAdapterSpec {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make(map[string]plugin.IdentityAdapterSpec, len(a.identitySpecMap))
	maps.Copy(out, a.identitySpecMap)
	return out
}

// ai is the runtime-side accessor for AI provider adapters. Returns
// the client + spec + boolean (true when registered).
func (a *adapterRegistry) ai(providerType string) (plugin.AIAdapterClient, plugin.AIAdapterSpec, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	cli, ok := a.aiClients[providerType]
	if !ok {
		return nil, plugin.AIAdapterSpec{}, false
	}
	return cli, a.aiSpecMap[providerType], true
}

// aiSpecs returns a copy of all registered AI provider adapter
// specs. The host uses this to render the provider_type dropdown in
// the admin UI and the chat composer dropdown with plugin-contributed
// entries alongside any built-in ones.
func (a *adapterRegistry) aiSpecs() map[string]plugin.AIAdapterSpec {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make(map[string]plugin.AIAdapterSpec, len(a.aiSpecMap))
	maps.Copy(out, a.aiSpecMap)
	return out
}

// storageAdapterRegistry is the per-category sub-registry handed to
// plugins in their RegisterAdapters call.
type storageAdapterRegistry struct {
	reg    *adapterRegistry
	origin string
}

// Register implements StorageAdapterRegistry. Plugins call this once
// per location type they handle. Re-registering the same LocationType
// from the same plugin (or two different plugins) returns an error
// rather than overwriting the existing adapter â€” the host wants a
// deterministic dispatch.
func (s *storageAdapterRegistry) Register(spec plugin.StorageAdapterSpec, client plugin.StorageAdapterClient) error {
	if spec.LocationType == "" {
		return errors.New("storage adapter: LocationType is required")
	}
	if spec.DisplayName == "" {
		return fmt.Errorf("storage adapter %s: DisplayName is required", spec.LocationType)
	}
	if client == nil {
		return fmt.Errorf("storage adapter %s: client is nil", spec.LocationType)
	}
	s.reg.mu.Lock()
	defer s.reg.mu.Unlock()
	if _, exists := s.reg.storageClients[spec.LocationType]; exists {
		return fmt.Errorf("storage adapter %s: already registered", spec.LocationType)
	}
	s.reg.storageClients[spec.LocationType] = wrapStorageClient(s.reg.audit, s.origin, spec.LocationType, client)
	s.reg.storageSpecs[spec.LocationType] = spec
	s.reg.storageOrigin[spec.LocationType] = s.origin
	return nil
}

// notificationAdapterRegistry is the per-category sub-registry
// handed to plugins for outbound notification adapters.
type notificationAdapterRegistry struct {
	reg    *adapterRegistry
	origin string
}

// Register implements NotificationAdapterRegistry. Plugins call
// this once per channel type they handle. Re-registering the same
// ChannelType from the same plugin (or two different plugins)
// returns an error rather than overwriting the existing adapter.
//
// ChannelType "internal" is intentionally rejected here: the host
// owns the in-app broadcast path and ships it as a built-in. Plugins
// contribute external channels only.
func (n *notificationAdapterRegistry) Register(spec plugin.NotificationAdapterSpec, client plugin.NotificationAdapterClient) error {
	if spec.ChannelType == "" {
		return errors.New("notification adapter: ChannelType is required")
	}
	if spec.ChannelType == "internal" {
		return errors.New("notification adapter: ChannelType \"internal\" is reserved for the host's built-in in-app broadcaster")
	}
	if spec.DisplayName == "" {
		return fmt.Errorf("notification adapter %s: DisplayName is required", spec.ChannelType)
	}
	if client == nil {
		return fmt.Errorf("notification adapter %s: client is nil", spec.ChannelType)
	}
	n.reg.mu.Lock()
	defer n.reg.mu.Unlock()
	if _, exists := n.reg.notificationClients[spec.ChannelType]; exists {
		return fmt.Errorf("notification adapter %s: already registered", spec.ChannelType)
	}
	n.reg.notificationClients[spec.ChannelType] = wrapNotificationClient(n.reg.audit, n.origin, spec.ChannelType, client)
	n.reg.notificationSpecMap[spec.ChannelType] = spec
	n.reg.notificationOrigin[spec.ChannelType] = n.origin

	// Mirror the declared capabilities so callers outside this process
	// can ask whether a channel carries files without holding the
	// registry. Best-effort: a tracker failure must not refuse an
	// otherwise valid adapter, since the registry above is the
	// authority for actually sending.
	if n.reg.tracker != nil {
		if err := n.reg.tracker.RecordNotificationAdapter(
			context.Background(), n.origin, spec.ChannelType, spec.DisplayName,
			spec.Capabilities.SupportsFileAttachments,
			spec.Capabilities.SupportsRichAttachments,
			spec.Capabilities.EncryptedCredentials,
			spec.Capabilities.RecipientOverride,
		); err != nil && n.reg.logger != nil {
			n.reg.logger.Warn("plugin runtime: recording notification adapter capabilities",
				"error", err, "plugin", n.origin, "channel_type", spec.ChannelType)
		}
	}
	return nil
}

// identityAdapterRegistry is the per-category sub-registry handed to
// plugins for external identity provider adapters.
type identityAdapterRegistry struct {
	reg    *adapterRegistry
	origin string
}

// Register implements IdentityAdapterRegistry. Plugins call this
// once per provider type they handle. Re-registering the same
// ProviderType from the same plugin (or two different plugins)
// returns an error rather than overwriting the existing adapter.
//
// All identity provider types are plugin-driven (Phase E, 2026-08-01):
// the host's modules/identity retains only the CRUD admin surface
// and the public callback dispatcher, and dispatches per-provider-
// type runtime work to whichever plugin owns the ProviderType.
func (i *identityAdapterRegistry) Register(spec plugin.IdentityAdapterSpec, client plugin.IdentityAdapterClient) error {
	if spec.ProviderType == "" {
		return errors.New("identity adapter: ProviderType is required")
	}
	if spec.DisplayName == "" {
		return fmt.Errorf("identity adapter %s: DisplayName is required", spec.ProviderType)
	}
	if client == nil {
		return fmt.Errorf("identity adapter %s: client is nil", spec.ProviderType)
	}
	i.reg.mu.Lock()
	defer i.reg.mu.Unlock()
	if _, exists := i.reg.identityClients[spec.ProviderType]; exists {
		return fmt.Errorf("identity adapter %s: already registered", spec.ProviderType)
	}
	i.reg.identityClients[spec.ProviderType] = wrapIdentityClient(i.reg.audit, i.origin, spec.ProviderType, client)
	i.reg.identitySpecMap[spec.ProviderType] = spec
	i.reg.identityOrigin[spec.ProviderType] = i.origin
	return nil
}

// aiAdapterRegistry is the per-category sub-registry handed to
// plugins for AI provider adapters (Anthropic, OpenAI, Ollama,
// Mistral, etc.).
type aiAdapterRegistry struct {
	reg    *adapterRegistry
	origin string
}

// Register implements AIAdapterRegistry. Plugins call this once per
// provider_type they handle. Re-registering the same ProviderType
// from the same plugin (or two different plugins) returns an error
// rather than overwriting the existing adapter — the host uses
// ProviderType as the lookup key, matching
// ai_provider_connections.provider_type.
//
// All AI providers are plugin-driven (2026-08-01): the host's
// modules/ai_providers and modules/ai_assistant retain only the
// row-resolution code and dispatch per-provider-type Test / Complete
// / Models to whichever plugin owns the ProviderType.
func (a *aiAdapterRegistry) Register(spec plugin.AIAdapterSpec, client plugin.AIAdapterClient) error {
	if spec.ProviderType == "" {
		return errors.New("ai adapter: ProviderType is required")
	}
	if spec.Slug == "" {
		return fmt.Errorf("ai adapter %s: Slug is required", spec.ProviderType)
	}
	if spec.DisplayName == "" {
		return fmt.Errorf("ai adapter %s: DisplayName is required", spec.ProviderType)
	}
	if client == nil {
		return fmt.Errorf("ai adapter %s: client is nil", spec.ProviderType)
	}
	a.reg.mu.Lock()
	defer a.reg.mu.Unlock()
	if _, exists := a.reg.aiClients[spec.ProviderType]; exists {
		return fmt.Errorf("ai adapter %s: already registered", spec.ProviderType)
	}
	a.reg.aiClients[spec.ProviderType] = wrapAIClient(a.reg.audit, a.origin, spec.ProviderType, client)
	a.reg.aiSpecMap[spec.ProviderType] = spec
	a.reg.aiOrigin[spec.ProviderType] = a.origin
	return nil
}

// snapshotAdapterRegistry is the per-category sub-registry handed to
// plugins for filesystem/driver-level snapshot adapters (zfs, btrfs,
// lvm, synology, truenas, …).
type snapshotAdapterRegistry struct {
	reg    *adapterRegistry
	origin string
}

// Register implements SnapshotAdapterRegistry. Plugins call this once
// per driver they handle. Re-registering the same Driver from the
// same plugin (or two different plugins) returns an error rather
// than overwriting the existing adapter — the host uses Driver as
// the lookup key, matching container_backup_snapshot_configs.driver.
func (s *snapshotAdapterRegistry) Register(spec plugin.SnapshotAdapterSpec, client plugin.SnapshotAdapterClient) error {
	if spec.Driver == "" {
		return errors.New("snapshot adapter: Driver is required")
	}
	if spec.Slug == "" {
		return fmt.Errorf("snapshot adapter %s: Slug is required", spec.Driver)
	}
	if spec.DisplayName == "" {
		return fmt.Errorf("snapshot adapter %s: DisplayName is required", spec.Driver)
	}
	if client == nil {
		return fmt.Errorf("snapshot adapter %s: client is nil", spec.Driver)
	}
	s.reg.mu.Lock()
	defer s.reg.mu.Unlock()
	if _, exists := s.reg.snapshotClients[spec.Driver]; exists {
		return fmt.Errorf("snapshot adapter %s: already registered", spec.Driver)
	}
	s.reg.snapshotClients[spec.Driver] = wrapSnapshotClient(s.reg.audit, s.origin, spec.Driver, client)
	s.reg.snapshotSpecMap[spec.Driver] = spec
	s.reg.snapshotOrigin[spec.Driver] = s.origin
	return nil
}

// snapshot is the runtime-side accessor for snapshot adapters.
// Returns the client + spec + boolean (true when registered).
func (a *adapterRegistry) snapshot(driver string) (plugin.SnapshotAdapterClient, plugin.SnapshotAdapterSpec, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	cli, ok := a.snapshotClients[driver]
	if !ok {
		return nil, plugin.SnapshotAdapterSpec{}, false
	}
	return cli, a.snapshotSpecMap[driver], true
}

// snapshotSpecs returns a copy of all registered snapshot adapter
// specs. The host uses this to render the driver dropdown in the
// admin UI with plugin-contributed entries alongside any built-in
// ones.
func (a *adapterRegistry) snapshotSpecs() map[string]plugin.SnapshotAdapterSpec {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make(map[string]plugin.SnapshotAdapterSpec, len(a.snapshotSpecMap))
	maps.Copy(out, a.snapshotSpecMap)
	return out
}

// imageScannerAdapterRegistry is the per-category sub-registry handed
// to plugins for image vulnerability scanners (trivy, grype, clair).
type imageScannerAdapterRegistry struct {
	reg    *adapterRegistry
	origin string
}

// Register implements ImageScannerAdapterRegistry. Plugins call this
// once per scanner type they handle. Re-registering the same
// ScannerType returns an error rather than overwriting: the host
// stores the type on every scan row it has ever written, so a silent
// overwrite would retroactively reattribute historical findings to an
// engine that never produced them.
func (s *imageScannerAdapterRegistry) Register(spec plugin.ImageScannerAdapterSpec, client plugin.ImageScannerAdapterClient) error {
	if spec.ScannerType == "" {
		return errors.New("scanner adapter: ScannerType is required")
	}
	if spec.Slug == "" {
		return fmt.Errorf("scanner adapter %s: Slug is required", spec.ScannerType)
	}
	if spec.DisplayName == "" {
		return fmt.Errorf("scanner adapter %s: DisplayName is required", spec.ScannerType)
	}
	if client == nil {
		return fmt.Errorf("scanner adapter %s: client is nil", spec.ScannerType)
	}
	s.reg.mu.Lock()
	defer s.reg.mu.Unlock()
	if _, exists := s.reg.scannerClients[spec.ScannerType]; exists {
		return fmt.Errorf("scanner adapter %s: already registered", spec.ScannerType)
	}
	s.reg.scannerClients[spec.ScannerType] = wrapScannerClient(s.reg.audit, s.origin, spec.ScannerType, client)
	s.reg.scannerSpecMap[spec.ScannerType] = spec
	s.reg.scannerOrigin[spec.ScannerType] = s.origin
	s.reg.scannerOrder = append(s.reg.scannerOrder, spec.ScannerType)
	return nil
}

// scanner is the runtime-side accessor for image scanner adapters.
// Returns the client + spec + boolean (true when registered).
func (a *adapterRegistry) scanner(scannerType string) (plugin.ImageScannerAdapterClient, plugin.ImageScannerAdapterSpec, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	cli, ok := a.scannerClients[scannerType]
	if !ok {
		return nil, plugin.ImageScannerAdapterSpec{}, false
	}
	return cli, a.scannerSpecMap[scannerType], true
}

// scannerSpecList returns every registered scanner spec in
// registration order - see the scannerOrder field comment.
func (a *adapterRegistry) scannerSpecList() []plugin.ImageScannerAdapterSpec {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]plugin.ImageScannerAdapterSpec, 0, len(a.scannerOrder))
	for _, t := range a.scannerOrder {
		if spec, ok := a.scannerSpecMap[t]; ok {
			out = append(out, spec)
		}
	}
	return out
}

// logForwarderAdapterRegistry is the per-category sub-registry handed
// to plugins for audit log forwarders (siem, syslog).
type logForwarderAdapterRegistry struct {
	reg    *adapterRegistry
	origin string
}

// Register implements LogForwarderAdapterRegistry. Plugins call this
// once per forwarder type they handle. Re-registering the same
// ForwarderType from the same plugin (or two different plugins)
// returns an error rather than overwriting - the host uses
// ForwarderType as the dispatch key, and a silent overwrite would
// send an operator's audit trail to a sink they never configured.
func (l *logForwarderAdapterRegistry) Register(spec plugin.LogForwarderAdapterSpec, client plugin.LogForwarderAdapterClient) error {
	if spec.ForwarderType == "" {
		return errors.New("logging adapter: ForwarderType is required")
	}
	if spec.Slug == "" {
		return fmt.Errorf("logging adapter %s: Slug is required", spec.ForwarderType)
	}
	if spec.DisplayName == "" {
		return fmt.Errorf("logging adapter %s: DisplayName is required", spec.ForwarderType)
	}
	if client == nil {
		return fmt.Errorf("logging adapter %s: client is nil", spec.ForwarderType)
	}
	l.reg.mu.Lock()
	defer l.reg.mu.Unlock()
	if _, exists := l.reg.loggingClients[spec.ForwarderType]; exists {
		return fmt.Errorf("logging adapter %s: already registered", spec.ForwarderType)
	}
	l.reg.loggingClients[spec.ForwarderType] = wrapLoggingClient(l.reg.audit, l.origin, spec.ForwarderType, client)
	l.reg.loggingSpecMap[spec.ForwarderType] = spec
	l.reg.loggingOrigin[spec.ForwarderType] = l.origin
	l.reg.loggingOrder = append(l.reg.loggingOrder, spec.ForwarderType)
	return nil
}

// logging is the runtime-side accessor for log forwarder adapters.
// Returns the client + spec + boolean (true when registered).
func (a *adapterRegistry) logging(forwarderType string) (plugin.LogForwarderAdapterClient, plugin.LogForwarderAdapterSpec, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	cli, ok := a.loggingClients[forwarderType]
	if !ok {
		return nil, plugin.LogForwarderAdapterSpec{}, false
	}
	return cli, a.loggingSpecMap[forwarderType], true
}

// loggingSpecList returns every registered forwarder spec in
// registration order. Order matters here in a way it does not for
// the other families - see the loggingOrder field comment.
func (a *adapterRegistry) loggingSpecList() []plugin.LogForwarderAdapterSpec {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]plugin.LogForwarderAdapterSpec, 0, len(a.loggingOrder))
	for _, t := range a.loggingOrder {
		if spec, ok := a.loggingSpecMap[t]; ok {
			out = append(out, spec)
		}
	}
	return out
}

// alertSourceAdapterRegistry is the per-category sub-registry handed
// to plugins that contribute alert conditions.
type alertSourceAdapterRegistry struct {
	reg    *adapterRegistry
	origin string
}

// Register implements AlertSourceAdapterRegistry. A plugin calls this
// once per source it owns. Re-registering a SourceType is an error
// rather than an overwrite: the host stores source_type on every rule
// and resolves it back to code at evaluation time, so a silent
// overwrite would quietly evaluate an operator's rule with a
// different implementation than the one they configured against.
func (l *alertSourceAdapterRegistry) Register(spec plugin.AlertSourceAdapterSpec, client plugin.AlertSourceAdapterClient) error {
	if spec.SourceType == "" {
		return errors.New("alerting adapter: SourceType is required")
	}
	if spec.DisplayName == "" {
		return fmt.Errorf("alerting adapter %s: DisplayName is required", spec.SourceType)
	}
	if client == nil {
		return fmt.Errorf("alerting adapter %s: client is nil", spec.SourceType)
	}
	// A duplicate condition key inside one source would make the
	// (source, condition) pair on a rule ambiguous, so it is caught
	// here rather than at evaluation time, where it would present as
	// a rule silently measuring the wrong thing.
	seen := make(map[string]struct{}, len(spec.Conditions))
	for _, c := range spec.Conditions {
		if c.Key == "" {
			return fmt.Errorf("alerting adapter %s: condition Key is required", spec.SourceType)
		}
		if c.Kind == "" {
			return fmt.Errorf("alerting adapter %s: condition %s has no Kind", spec.SourceType, c.Key)
		}
		if _, dup := seen[c.Key]; dup {
			return fmt.Errorf("alerting adapter %s: condition %s declared twice", spec.SourceType, c.Key)
		}
		seen[c.Key] = struct{}{}
	}
	l.reg.mu.Lock()
	defer l.reg.mu.Unlock()
	if _, exists := l.reg.alertingClients[spec.SourceType]; exists {
		return fmt.Errorf("alerting adapter %s: already registered", spec.SourceType)
	}
	l.reg.alertingClients[spec.SourceType] = wrapAlertingClient(l.reg.audit, l.origin, spec.SourceType, client)
	l.reg.alertingSpecMap[spec.SourceType] = spec
	l.reg.alertingOrigin[spec.SourceType] = l.origin
	l.reg.alertingOrder = append(l.reg.alertingOrder, spec.SourceType)
	return nil
}

// alerting is the runtime-side accessor for alert sources.
func (a *adapterRegistry) alerting(sourceType string) (plugin.AlertSourceAdapterClient, plugin.AlertSourceAdapterSpec, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	cli, ok := a.alertingClients[sourceType]
	if !ok {
		return nil, plugin.AlertSourceAdapterSpec{}, false
	}
	return cli, a.alertingSpecMap[sourceType], true
}

// alertingSpecList returns every registered source spec in
// registration order - see the alertingOrder field comment.
func (a *adapterRegistry) alertingSpecList() []plugin.AlertSourceAdapterSpec {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]plugin.AlertSourceAdapterSpec, 0, len(a.alertingOrder))
	for _, t := range a.alertingOrder {
		if spec, ok := a.alertingSpecMap[t]; ok {
			out = append(out, spec)
		}
	}
	return out
}

// reportingAdapterRegistry is the per-category sub-registry handed to
// plugins that contribute report types to the Reporting plugin.
type reportingAdapterRegistry struct {
	reg    *adapterRegistry
	origin string
}

// Register implements ReportAdapterRegistry. A plugin calls this once,
// with every report type it offers: a provider is one client answering
// several questions, and registering per type would make a plugin build
// the same client five times.
//
// Two failures are rejected rather than resolved: a provider slug that
// is already registered, and a report type key another provider already
// claimed. The Reporting plugin dispatches on the type key, and a
// silent second winner would make which report ran depend on map
// iteration order — with the operator's library rows still pointing at
// the key either of them could have produced.
func (rp *reportingAdapterRegistry) Register(spec plugin.ReportProviderSpec, client plugin.ReportProviderClient) error {
	if spec.Slug == "" {
		return errors.New("reporting adapter: Slug is required")
	}
	if spec.DisplayName == "" {
		return fmt.Errorf("reporting adapter %s: DisplayName is required", spec.Slug)
	}
	if client == nil {
		return fmt.Errorf("reporting adapter %s: client is nil", spec.Slug)
	}
	if len(spec.Types) == 0 && len(spec.Facets) == 0 {
		return fmt.Errorf("reporting adapter %s: at least one report type or facet is required", spec.Slug)
	}
	facetClient, err := validateFacetDecls(spec, client)
	if err != nil {
		return err
	}
	for _, t := range spec.Types {
		if t.Key == "" {
			return fmt.Errorf("reporting adapter %s: report type key is required", spec.Slug)
		}
		if t.Category == "" {
			return fmt.Errorf("reporting adapter %s: report type %s: Category is required", spec.Slug, t.Key)
		}
		if len(t.Levels) == 0 {
			return fmt.Errorf("reporting adapter %s: report type %s: at least one level is required", spec.Slug, t.Key)
		}
	}

	rp.reg.mu.Lock()
	defer rp.reg.mu.Unlock()
	if _, exists := rp.reg.reportingClients[spec.Slug]; exists {
		return fmt.Errorf("reporting adapter %s: already registered", spec.Slug)
	}
	for _, t := range spec.Types {
		if owner, taken := rp.reg.reportingTypeOwner[t.Key]; taken {
			return fmt.Errorf("reporting adapter %s: report type %s already registered by %s", spec.Slug, t.Key, owner)
		}
	}
	if err := rp.reg.facetOwnersFree(spec); err != nil {
		return err
	}
	rp.reg.reportingClients[spec.Slug] = wrapReportingClient(rp.reg.audit, rp.origin, spec.Slug, client)
	rp.reg.reportingSpecMap[spec.Slug] = spec
	rp.reg.reportingOrder = append(rp.reg.reportingOrder, spec.Slug)
	for _, t := range spec.Types {
		rp.reg.reportingTypeOwner[t.Key] = spec.Slug
	}
	rp.reg.storeFacets(spec, wrapFacetContributor(rp.reg.audit, rp.origin, spec.Slug, facetClient))
	return nil
}

// reportingSpecList returns every registered provider spec in
// registration order, so the studio's shelves keep a stable order.
func (a *adapterRegistry) reportingSpecList() []plugin.ReportProviderSpec {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]plugin.ReportProviderSpec, 0, len(a.reportingOrder))
	for _, slug := range a.reportingOrder {
		if spec, ok := a.reportingSpecMap[slug]; ok {
			out = append(out, spec)
		}
	}
	return out
}

// reportingFor resolves the provider that owns one report type key.
func (a *adapterRegistry) reportingFor(typeKey string) (plugin.ReportProviderClient, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	slug, ok := a.reportingTypeOwner[typeKey]
	if !ok {
		return nil, false
	}
	cli, ok := a.reportingClients[slug]
	return cli, ok
}

// --- registry implementations ------------------------------------

// routeRegistry collects plugin-contributed HTTP routes and mounts them
// into the host's chi router at the right layer (public / auth /
// protected) with the right per-route middleware stack.
//
// The Public/Auth/Protected methods each set an authClass field on the
// registry before delegating to mount(); mount() reads that class to
// pick the correct Mount*Routes sink and to set the isPublic bit on
// the contribution tracker. The manifest's RouteDecl is consulted for
// the per-route requires_permission override; the plugin's Go code can
// request a stronger or weaker perm than the manifest declares and the
// loader still records the manifest's value (which is the
// audit-visible contract).
type routeRegistry struct {
	runtime   *Runtime
	origin    string
	authClass string // "public" | "auth" | "protected"
	// collected is what the plugin registered during RegisterRoutes.
	// It is handed to the router in one call once the plugin has
	// finished loading, rather than route by route as it registers:
	// a plugin whose Init later fails must not leave half a surface
	// answering requests.
	collected []router.PluginRoute
}

func (r *routeRegistry) Public(method, path string, h http.HandlerFunc) {
	r.authClass = "public"
	r.mount(method, path, h)
}

func (r *routeRegistry) Auth(method, path string, h http.HandlerFunc) {
	r.authClass = "auth"
	r.mount(method, path, h)
}

func (r *routeRegistry) Protected(method, path string, h http.HandlerFunc) {
	r.authClass = "protected"
	r.mount(method, path, h)
}

// lookupRouteDecl finds the manifest entry that matches (method, path)
// for this plugin. Falls back to empty Requires / non-public when the
// plugin code registered a route that isn't in its manifest — the
// tracker still gets called so the route is queryable, but it has no
// declared permission so the middleware is a pass-through.
func (r *routeRegistry) lookupRouteDecl(method, path string) (requires string, isPublic bool, auditRequired bool, auditAction string, auditEntityType string, auditSeverity string) {
	manifest, ok := r.runtime.manifests[r.origin]
	if !ok {
		return "", false, methodNeedsPluginAudit(method), "", "", ""
	}
	for _, rt := range manifest.Contribs.Routes {
		if strings.EqualFold(rt.Method, method) && rt.Path == path {
			required := methodNeedsPluginAudit(method)
			if rt.AuditRequired != nil {
				required = *rt.AuditRequired
			}
			return rt.Requires, rt.Public, required, rt.AuditAction, rt.AuditEntityType, rt.AuditSeverity
		}
	}
	return "", false, methodNeedsPluginAudit(method), "", "", ""
}

func (r *routeRegistry) mount(method, path string, h http.HandlerFunc) {
	requires, manifestPublic, auditRequired, auditAction, auditEntityType, auditSeverity := r.lookupRouteDecl(method, path)
	isPublic := r.authClass == "public" || manifestPublic
	// A non-public route the manifest never declared gets an empty permission
	// key, and PluginPermissionMiddleware short-circuits an empty key to a
	// pass-through — so the route is reachable by any authenticated user
	// whatever their role. Six routes shipped in that state before 2026-09-27.
	// Logging it here is the only check that covers a plugin the repo has never
	// seen; TestPluginRouteDeclarationDrift is the in-tree half.
	if !isPublic && requires == "" {
		if lg := r.runtime.logger; lg != nil {
			lg.Error("plugin route registered with no declared permission: any authenticated user can call it",
				"origin", r.origin, "method", method, "path", path,
				"remedy", "declare the route under contributes.routes in plugin.yaml with a requires_permission key")
		}
	}
	if r.runtime.tracker != nil {
		if recErr := r.runtime.tracker.RecordRoute(context.Background(), r.origin, method, path, requires, isPublic); recErr != nil {
			r.runtime.logger.Warn("plugin runtime: recording route failed", "origin", r.origin, "path", path, "error", recErr)
		}
	}
	// Wrap with the per-route permission middleware when the manifest
	// declares a requires_permission string. PluginPermissionMiddleware
	// short-circuits to a pass-through when perm == "", so the call
	// site is unconditional.
	wrapped := router.PluginPermissionMiddlewareFor(r.runtime.deps, requires, path)(http.HandlerFunc(h))
	wrapped = audit.PluginFallbackMiddlewareWithSeverity(r.runtime.host.AuditLog, r.origin, auditAction, auditEntityType, auditSeverity, auditRequired)(wrapped)
	// The three Mount*Routes sinks all live inside router.New's
	// `r.Route("/api", ...)` group, so the pattern handed to chi is
	// relative to /api — host modules register "/containers", not
	// "/api/containers". A plugin manifest, by contrast, must declare
	// the absolute "/api/<slug>/..." form (manifest_schema.go). Passing
	// that through verbatim served every plugin route at /api/api/... and
	// left it 404 at its declared address.
	mountPath := subrouterPath(path)
	authClass := r.authClass
	switch authClass {
	case router.PluginAuthPublic, router.PluginAuthAuth, router.PluginAuthProtected:
	default:
		// An unexpected class lands in the authenticated group, which
		// preserves the behaviour legacy plugins had before the three
		// classes existed — and fails closed rather than publishing an
		// endpoint nobody meant to publish.
		authClass = router.PluginAuthProtected
	}
	r.collected = append(r.collected, router.PluginRoute{
		Method:    method,
		Pattern:   mountPath,
		AuthClass: authClass,
		Handler:   wrapped,
	})
}

// subrouterPath converts a plugin's declared absolute route path into the
// pattern the /api subrouter expects. Anything that is not an /api/<x>
// path is returned unchanged: chi panics on an empty pattern, so a
// malformed registration degrades to the old behaviour instead of
// bringing the host down at boot.
func subrouterPath(path string) string {
	if !strings.HasPrefix(path, "/api/") {
		return path
	}
	return strings.TrimPrefix(path, "/api")
}

func methodNeedsPluginAudit(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

type menuRegistry struct {
	runtime *Runtime
	origin  string
}

func (r *menuRegistry) Add(item plugin.MenuItem) error {
	if r.runtime.tracker == nil {
		return nil
	}
	// RecordMenu's signature is (ctx, origin, parent, labelNS, labelKey,
	// path, icon, permission). Passing the label key in the labelNS slot
	// stored an empty label_key on every row, and the sidebar then asked
	// i18next for "<key>:" — a key that can never resolve. A plugin that
	// does not name a namespace gets its own slug, which is the
	// convention every bundled plugin's i18n bundle already follows.
	labelNS := item.LabelNS
	if labelNS == "" {
		labelNS = r.origin
	}
	if recErr := r.runtime.tracker.RecordMenu(context.Background(), r.origin, item.Parent, labelNS, item.LabelKey, item.Path, item.Icon, item.Required, item.Priority); recErr != nil {
		r.runtime.logger.Warn("plugin runtime: recording menu entry failed", "origin", r.origin, "path", item.Path, "error", recErr)
	}
	return nil
}

func (r *menuRegistry) Remove(labelKey string) error { return nil }

type tabRegistry struct {
	runtime *Runtime
	origin  string
}

// Add records one tab on a host page. A plugin that names no namespace gets
// its own slug, which is the convention every bundled plugin's i18n bundle
// already follows.
func (r *tabRegistry) Add(item plugin.TabItem) error {
	if r.runtime.tracker == nil {
		return nil
	}
	labelNS := item.LabelNS
	if labelNS == "" {
		labelNS = r.origin
	}
	return r.runtime.tracker.RecordTab(context.Background(), r.origin, item.PageID, item.Key,
		labelNS, item.LabelKey, item.Icon, item.Required, item.Priority)
}

func (r *tabRegistry) Remove(pageID, key string) error { return nil }

type widgetRegistry struct {
	runtime *Runtime
	origin  string
}

// Add records one dashboard widget. The descriptor's Width/Height become the
// `WxH` default size the dashboard grid reads; anything else the plugin wants
// the picker to know (description, icon, min_width, min_height, category)
// rides in Settings and is stored as JSON, so the shared WidgetDescriptor
// struct — which every plugin .so is compiled against — never has to change
// shape.
func (r *widgetRegistry) Add(key string, descriptor plugin.WidgetDescriptor) error {
	if r.runtime.tracker == nil {
		return nil
	}
	size := "4x3"
	if descriptor.Width > 0 && descriptor.Height > 0 {
		size = fmt.Sprintf("%dx%d", descriptor.Width, descriptor.Height)
	}
	category := "plugin"
	if c, ok := descriptor.Settings["category"].(string); ok && c != "" {
		category = c
	}
	settings := "{}"
	if len(descriptor.Settings) > 0 {
		if raw, err := json.Marshal(descriptor.Settings); err == nil {
			settings = string(raw)
		} else {
			r.runtime.logger.Warn("plugin widget settings not serialisable", "plugin", r.origin, "widget", key, "error", err)
		}
	}
	if err := r.runtime.tracker.RecordWidget(context.Background(), r.origin, key, descriptor.Title, category, size, settings); err != nil {
		r.runtime.logger.Warn("recording plugin widget failed", "plugin", r.origin, "widget", key, "error", err)
	}
	return nil
}

func (r *widgetRegistry) Remove(key string) error { return nil }

type nodeRegistry struct {
	runtime *Runtime
	origin  string
	// dir is the plugin's install directory, read for its capabilities when a node
	// carries a plugin-select field: the registry only learns them after the load.
	dir  string
	caps []plugincaps.Capability
	read bool
}

func (r *nodeRegistry) Add(key string, descriptor plugin.NodeDescriptor) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("plugin %s: workflow node key is empty", r.origin)
	}
	r.stampPickerSources(key, descriptor.Fields)
	if r.runtime.tracker != nil {
		configJSON := marshalNodeConfig(descriptor)
		inputsJSON := marshalNodePorts(descriptor.Inputs)
		outputsJSON := marshalNodePorts(descriptor.Outputs)
		if recErr := r.runtime.tracker.RecordNode(context.Background(), r.origin, key, descriptor.Category, descriptor.DisplayName, descriptor.Icon, configJSON, inputsJSON, outputsJSON, descriptor.Requires); recErr != nil {
			r.runtime.logger.Warn("plugin runtime: recording node failed", "origin", r.origin, "key", key, "error", recErr)
		}
	}
	// The executor is what makes the node do anything at run time.
	// Bind it under the exact action key the SPA writes into the
	// canvas so the workflow engine's lookup is a plain map hit.
	action := workflowNodeActionKey(r.origin, key)
	exec := descriptor.Execute
	if exec == nil {
		// A node without an executor is a trigger (the plugin fires it
		// through HostContext.Workflows) or a pure pass-through. The
		// engine still has to know it, so register the identity: the
		// incoming msg — the event payload on a fired run, or an empty
		// msg on a manual run — flows out unchanged.
		exec = passThroughNode
	}
	r.runtime.mu.Lock()
	r.runtime.nodeExecutors[action] = exec
	r.runtime.mu.Unlock()
	return nil
}

// stampPickerSources pins every plugin-select field's source to the registering
// plugin, so a node can only ever list through its own plugin's declared pickers, and
// logs a source the manifest does not declare; the canvas then shows the field disabled.
func (r *nodeRegistry) stampPickerSources(key string, fields []map[string]any) {
	for _, field := range fields {
		if t, _ := field["type"].(string); t != plugincaps.FieldTypePluginSelect {
			continue
		}
		src := plugincaps.FieldSource{}
		if raw, ok := field["source"].(map[string]any); ok {
			src.Plugin, _ = raw["plugin"].(string)
			src.Target, _ = raw["target"].(string)
		}
		if err := plugincaps.CheckFieldSource(r.origin, r.capabilities(), src); err != nil {
			r.runtime.logger.Warn("plugin runtime: workflow node field names an undeclared picker source",
				"origin", r.origin, "node", key, "field", field["key"], "error", err)
		}
		field["source"] = map[string]any{"plugin": r.origin, "target": src.Target}
	}
}

// capabilities parses the plugin's manifest once per registration pass.
func (r *nodeRegistry) capabilities() []plugincaps.Capability {
	if r.read {
		return r.caps
	}
	r.read = true
	raw, err := os.ReadFile(filepath.Join(r.dir, "plugin.yaml"))
	if err != nil {
		return nil
	}
	if caps, err := plugincaps.Parse(r.origin, raw); err == nil {
		r.caps = caps
	}
	return r.caps
}

// passThroughNode is the executor bound to descriptors without one.
func passThroughNode(_ context.Context, in plugin.NodeExecInput) (plugin.NodeExecOutput, error) {
	return plugin.NodeExecOutput{Port: "output", Msg: in.Msg}, nil
}

// marshalNodeConfig serialises the per-node configSchema to the JSON
// array the SPA decodes into ConfigField[]. Fields wins; the legacy
// Config map is honoured only through a "fields" member holding a
// list. Anything else becomes `"[]"` so the SQL column always has a
// value the SPA can decode directly.
func marshalNodeConfig(d plugin.NodeDescriptor) string {
	var fields any
	switch {
	case len(d.Fields) > 0:
		fields = d.Fields
	case len(d.Config) > 0:
		if f, ok := d.Config["fields"]; ok {
			fields = f
		}
	}
	if fields == nil {
		return "[]"
	}
	out, err := json.Marshal(fields)
	if err != nil || len(out) == 0 || out[0] != '[' {
		return "[]"
	}
	return string(out)
}

// marshalNodePorts serialises the per-node input / output port list
// to JSON. An empty slice becomes `"[]"`.
func marshalNodePorts(p []string) string {
	if len(p) == 0 {
		return "[]"
	}
	out, err := json.Marshal(p)
	if err != nil {
		return "[]"
	}
	return string(out)
}

func (r *nodeRegistry) Remove(key string) error { return nil }

// accessRegistry translates the plugin's AccessRegistry calls into
// plugin_permissions / rbac_policies rows tagged with origin=<slug>.
type accessRegistry struct {
	runtime *Runtime
	origin  string
}

func (a *accessRegistry) AddPermission(p plugin.PermissionDecl) error {
	if _, err := a.runtime.host.DB.ExecContext(context.Background(), `
		INSERT INTO plugin_permissions (origin, permission_key, description, scope)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (origin, permission_key) DO NOTHING`,
		a.origin, p.Key, p.Description, orDefault(p.Scope, "global"),
	); err != nil {
		return fmt.Errorf("inserting plugin permission %s: %w", p.Key, err)
	}
	return nil
}

func (a *accessRegistry) RemovePermission(key string) error {
	_, err := a.runtime.host.DB.ExecContext(context.Background(), `DELETE FROM plugin_permissions WHERE origin = ? AND permission_key = ?`, a.origin, key)
	return err
}

func (a *accessRegistry) InstallABACRule(rule plugin.ABACRuleDecl) error {
	id := uuid.NewString()
	then := rule.Then
	if then != "allow" && then != "deny" {
		then = "deny"
	}
	_, err := a.runtime.host.DB.ExecContext(context.Background(), `
		INSERT INTO rbac_policies
		    (id, name, description, effect, resource_type, expression, priority, enabled, created_by, origin)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		id, "plugin:"+a.origin+":"+rule.ID, rule.Reason, then, "plugin",
		rule.When, 100, "plugin:"+a.origin, a.origin,
	)
	if err != nil {
		return fmt.Errorf("inserting plugin ABAC rule: %w", err)
	}
	return nil
}

func (a *accessRegistry) RemoveABACRule(id string) error {
	_, err := a.runtime.host.DB.ExecContext(context.Background(), `DELETE FROM rbac_policies WHERE id = ? AND origin = ?`, id, a.origin)
	return err
}

func (a *accessRegistry) RegisterSCIMSchema(schema plugin.SCIMSchema) error {
	// Phase A: SCIM schemas remain core. Plugins cannot register new ones.
	return plugin.ErrSchemaExists
}

func (a *accessRegistry) UnregisterSCIMSchema(uri string) error { return nil }

// --- sub-handle adapters -----------------------------------------

type slogAdapter struct {
	runtimeLogger *slog.Logger
	slug          string
}

func (s slogAdapter) Info(msg string, args ...any) {
	s.runtimeLogger.Info("[plugin:"+s.slug+"] "+msg, args...)
}
func (s slogAdapter) Warn(msg string, args ...any) {
	s.runtimeLogger.Warn("[plugin:"+s.slug+"] "+msg, args...)
}
func (s slogAdapter) Error(msg string, args ...any) {
	s.runtimeLogger.Error("[plugin:"+s.slug+"] "+msg, args...)
}
func (s slogAdapter) Debug(msg string, args ...any) {
	s.runtimeLogger.Debug("[plugin:"+s.slug+"] "+msg, args...)
}

type encryptionAdapter struct {
	service *encryption.Service
}

func (e encryptionAdapter) EncryptString(plain string) (string, error) {
	return e.service.Encrypt(plain)
}
func (e encryptionAdapter) DecryptString(cipher string) (string, error) {
	return e.service.Decrypt(cipher)
}

// pluginTablePrefix resolves the table prefix a plugin is allowed to
// touch, and is the single definition of that rule.
//
// Hyphens in the slug become underscores: slugs are kebab-case
// (`logging-siem`) and SQL identifiers are not, so every plugin's
// migration names its tables `plugin_logging_siem_*`. Comparing
// against the raw slug rejected every hyphenated plugin's own tables
// — only single-word slugs ever passed.
//
// A declared `table_prefix` overrides the derived value. That is what
// a plugin renamed after its migrations shipped uses: its tables keep
// the names they were created with, and deriving from the new slug
// would fail every one of its own statements closed.
func pluginTablePrefix(slug, declared string) string {
	if declared != "" {
		return declared
	}
	return "plugin_" + strings.ReplaceAll(slug, "-", "_") + "_"
}

type pluginDB struct {
	host *sql.DB
	slug string
	// prefix is the resolved table prefix, not the raw slug: see
	// pluginTablePrefix.
	prefix string
}

func (d *pluginDB) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	if err := pluginDBExec(d.prefix, query); err != nil {
		return 0, err
	}
	res, err := d.host.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected() //nolint:gocritic // plugin.DB.Exec must surface the driver error, which db.RowsAffected would only log
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (d *pluginDB) Query(ctx context.Context, query string, args ...any) (plugin.Rows, error) {
	if err := pluginDBExec(d.prefix, query); err != nil {
		return nil, err
	}
	rows, err := d.host.QueryContext(ctx, query, args...) //nolint:rowserrcheck // sqlRowsAdapter.Err forwards rows.Err to the plugin caller
	if err != nil {
		return nil, err
	}
	return &sqlRowsAdapter{rows: rows}, nil
}

func (d *pluginDB) KV() plugin.KVStore { return &kvAdapter{host: d.host, slug: d.slug} }

type sqlRowsAdapter struct {
	rows *sql.Rows
}

func (r *sqlRowsAdapter) Next() bool          { return r.rows.Next() }
func (r *sqlRowsAdapter) Scan(d ...any) error { return r.rows.Scan(d...) }
func (r *sqlRowsAdapter) Close() error        { return r.rows.Close() }
func (r *sqlRowsAdapter) Err() error          { return r.rows.Err() }

type kvAdapter struct {
	host *sql.DB
	slug string
}

func (k *kvAdapter) Get(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := k.host.QueryRowContext(ctx,
		`SELECT value_enc FROM plugin_config WHERE origin = ? AND key = ? AND is_secret = 0`,
		k.slug, key,
	).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

func (k *kvAdapter) Set(ctx context.Context, key, value string) error {
	_, err := k.host.ExecContext(ctx,
		`INSERT INTO plugin_config (origin, key, value_enc, is_secret) VALUES (?, ?, ?, 0)
		 ON CONFLICT (origin, key) DO UPDATE SET value_enc = excluded.value_enc, updated_at = datetime('now')`,
		k.slug, key, value,
	)
	return err
}

func (k *kvAdapter) Delete(ctx context.Context, key string) error {
	_, err := k.host.ExecContext(ctx,
		`DELETE FROM plugin_config WHERE origin = ? AND key = ?`, k.slug, key,
	)
	return err
}

type auditAdapter struct {
	log  *audit.Logger
	slug string
}

func (a *auditAdapter) Emit(event string, fields map[string]any) error {
	if a.log == nil {
		return nil
	}
	// Phase A: AuditLogger doesn't expose LogRaw yet. Best-effort:
	// serialize fields to a Details string until the audit package
	// adds a structured-emit method (tracked in Step 5).
	return a.log.EmitPlugin(a.slug, event, fields)
}

// EmitEvent satisfies plugin.AuditActorHandle, so an add-on's row names the
// account behind the request instead of leaving the Actor column blank.
func (a *auditAdapter) EmitEvent(ev plugin.AuditEvent) error {
	if a.log == nil {
		return nil
	}
	return a.log.EmitPluginEvent(a.slug, audit.PluginEvent{
		Action:   ev.Action,
		UserID:   ev.UserID,
		Severity: ev.Severity,
		Fields:   ev.Fields,
	})
}

// LifecycleRecorder is the host-side surface behind plugin.LifecycleHandle.
// Implemented by modules/lifecycle so the loader never writes the table
// itself — the same split StorageDispatchSource uses.
type LifecycleRecorder interface {
	Record(ctx context.Context, source string, ev LifecycleRecord) error
}

// LifecycleRecord is the loader-level shape of one timeline entry. It mirrors
// plugin.LifecycleEvent; the split keeps modules/lifecycle from importing the
// plugin package.
type LifecycleRecord struct {
	EnvironmentID string
	SubjectType   string
	SubjectID     string
	SubjectName   string
	EventType     string
	Action        string
	State         string
	Severity      string
	Metadata      map[string]any
}

// lifecycleHandleFor returns nil rather than a handle over a nil recorder, so
// a plugin's `if host.Lifecycle != nil` reads true only when a row would land.
func lifecycleHandleFor(source LifecycleRecorder, slug string) plugin.LifecycleHandle {
	if source == nil {
		return nil
	}
	return &lifecycleAdapter{source: source, slug: slug}
}

// lifecycleAdapter wraps LifecycleRecorder to satisfy plugin.LifecycleHandle,
// stamping every row with the emitting plugin's slug as its source.
type lifecycleAdapter struct {
	source LifecycleRecorder
	slug   string
}

func (a *lifecycleAdapter) Record(ctx context.Context, ev plugin.LifecycleEvent) error {
	if a == nil || a.source == nil {
		return nil
	}
	return a.source.Record(ctx, a.slug, LifecycleRecord{
		EnvironmentID: ev.EnvironmentID,
		SubjectType:   ev.SubjectType,
		SubjectID:     ev.SubjectID,
		SubjectName:   ev.SubjectName,
		EventType:     ev.EventType,
		Action:        ev.Action,
		State:         ev.State,
		Severity:      ev.Severity,
		Metadata:      ev.Metadata,
	})
}

// EnvironmentNamer is the host-side lookup behind plugin.EnvironmentHandle.
type EnvironmentNamer interface {
	EnvironmentName(ctx context.Context, environmentID string) string
}

// environmentHandleFor returns nil over a nil source, so a plugin's
// `if host.Environments != nil` reads true only when it would get an answer.
func environmentHandleFor(source EnvironmentNamer) plugin.EnvironmentHandle {
	if source == nil {
		return nil
	}
	return &environmentAdapter{source: source}
}

// environmentAdapter wraps EnvironmentNamer to satisfy plugin.EnvironmentHandle.
type environmentAdapter struct {
	source EnvironmentNamer
}

func (a *environmentAdapter) Name(ctx context.Context, environmentID string) string {
	if a == nil || a.source == nil {
		return environmentID
	}
	return a.source.EnvironmentName(ctx, environmentID)
}

// BackupLogRecorder is the host-side writer behind plugin.BackupLogHandle.
// The split keeps modules/backup_log from importing the plugin package.
type BackupLogRecorder interface {
	RecordProviderEntry(ctx context.Context, source string, entry BackupLogRecord) error
}

// BackupLogRecord is the loader-level shape of one backup log entry. It
// mirrors plugin.BackupLogEntry.
type BackupLogRecord struct {
	EnvironmentID string
	PlanID        string
	PlanName      string
	RunID         string
	SubjectID     string
	SubjectName   string
	SubjectKind   string
	Action        string
	Phase         string
	Severity      string
	Message       string
}

// backupLogHandleFor returns nil rather than a handle over a nil recorder, so
// a plugin's `if host.BackupLog != nil` reads true only when a row would land.
func backupLogHandleFor(source BackupLogRecorder, slug string) plugin.BackupLogHandle {
	if source == nil {
		return nil
	}
	return &backupLogAdapter{source: source, slug: slug}
}

// backupLogAdapter wraps BackupLogRecorder to satisfy plugin.BackupLogHandle,
// stamping every row with the emitting plugin's slug as its source.
type backupLogAdapter struct {
	source BackupLogRecorder
	slug   string
}

func (a *backupLogAdapter) Record(ctx context.Context, entry plugin.BackupLogEntry) error {
	if a == nil || a.source == nil {
		return nil
	}
	return a.source.RecordProviderEntry(ctx, a.slug, BackupLogRecord{
		EnvironmentID: entry.EnvironmentID,
		PlanID:        entry.PlanID,
		PlanName:      entry.PlanName,
		RunID:         entry.RunID,
		SubjectID:     entry.SubjectID,
		SubjectName:   entry.SubjectName,
		SubjectKind:   entry.SubjectKind,
		Action:        entry.Action,
		Phase:         entry.Phase,
		Severity:      entry.Severity,
		Message:       entry.Message,
	})
}

// aiAdapter wraps AICompleterSource to satisfy plugin.AIHandle,
// converting between the loader-local AICompletion* types and the
// SDK's plugin.AI* types (the same conversion idiom
// identityProvisionerAdapter uses). source is nil on a host that
// hasn't wired AI yet (main.go's bootPluginRuntime doesn't set
// HostServices.AI until Task 3 lands the real implementation in
// modules/ai_providers) — every method below degrades gracefully
// against a nil source instead of panicking.
//
// slug is the owning plugin's slug, captured once in buildHostContext
// (one aiAdapter is built per plugin Init call). It is forwarded to
// AICompleterSource.Complete as host-side plumbing only, so the
// implementation can key a per-plugin rate limit — it never appears
// in the plugin-facing plugin.AIHandle.Complete(ctx, req) signature,
// so a plugin cannot read or spoof it.
type aiAdapter struct {
	source AICompleterSource
	slug   string
}

func (a *aiAdapter) Complete(ctx context.Context, req plugin.AIRequest) (plugin.AIResponse, error) {
	if a.source == nil {
		return plugin.AIResponse{}, errors.New("ai.invoke: no AI bridge configured on this host")
	}
	messages := make([]AICompletionMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		messages = append(messages, AICompletionMessage{Role: m.Role, Content: m.Content, Name: m.Name})
	}
	out, err := a.source.Complete(ctx, a.slug, req.ConnectionID, AICompletionRequest{
		Model:        req.Model,
		Messages:     messages,
		Temperature:  req.Temperature,
		MaxTokens:    req.MaxTokens,
		Stop:         req.Stop,
		Metadata:     req.Metadata,
		ConnectionID: req.ConnectionID,
	})
	if err != nil {
		return plugin.AIResponse{}, fmt.Errorf("ai.invoke: %w", err)
	}
	return plugin.AIResponse{
		Content:  out.Content,
		Model:    out.Model,
		Provider: out.Provider,
		Usage: plugin.AIUsage{
			InputTokens:  out.Usage.InputTokens,
			OutputTokens: out.Usage.OutputTokens,
			TotalTokens:  out.Usage.TotalTokens,
		},
		Metadata: out.Metadata,
	}, nil
}

func (a *aiAdapter) Available() bool {
	return a.source != nil && a.source.Available()
}

// Connections returns an empty slice (never nil) when the source
// isn't wired, so plugin code can range over the result without a
// nil check.
func (a *aiAdapter) Connections() []plugin.AIConnection {
	if a.source == nil {
		return []plugin.AIConnection{}
	}
	src := a.source.Connections()
	out := make([]plugin.AIConnection, 0, len(src))
	for _, c := range src {
		out = append(out, plugin.AIConnection{
			ID:           c.ID,
			Name:         c.Name,
			ProviderType: c.ProviderType,
			IsDefault:    c.IsDefault,
		})
	}
	return out
}

// orDefault returns v if non-empty, otherwise def.
func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// enforceCapabilities is the Â§9 sandboxing gate. The host owns the
// network namespace and the data directory; plugins must not
// advertise net.bind or fs.write capabilities. Any such declaration
// is a hard error and short-circuits the load.
func enforceCapabilities(slug string, caps plugin.Capabilities) error {
	if caps.NetBind {
		return fmt.Errorf("%w: %s declared net.bind (host owns the network namespace)", plugin.ErrScopeViolation, slug)
	}
	// fs.write is always a contract violation. The plugin's
	// plugin.DBHandle, plugin.KVStore, and writes to its own data dir are already
	// mediated; any plugin claiming raw fs.write is trying to write
	// outside the allowed surface.
	if caps.FSWrite {
		return fmt.Errorf("%w: %s declared fs.write (plugins must use the host's plugin.DBHandle, plugin.KVStore, or contribution-table writers)", plugin.ErrScopeViolation, slug)
	}
	return nil
}

// --- storage + comms delivery adapters ---------------------------

// storageHandleFor returns a plugin.StorageHandle, or nil when the
// host wired no storage bridge. Returning a typed nil would give
// plugins a non-nil interface that panics on use, so the nil check
// happens here rather than inside every method.
func storageHandleFor(source StorageDispatchSource, slug string) plugin.StorageHandle {
	if source == nil {
		return nil
	}
	return &storageAdapter{source: source, slug: slug}
}

type storageAdapter struct {
	source StorageDispatchSource
	slug   string
}

func (a *storageAdapter) Upload(ctx context.Context, locationID, key string, data []byte) error {
	if a.source == nil {
		return errors.New("storage.upload: no storage bridge configured on this host")
	}
	if strings.TrimSpace(locationID) == "" {
		return errors.New("storage.upload: locationID is required")
	}
	if strings.TrimSpace(key) == "" {
		return errors.New("storage.upload: key is required")
	}
	// The slug is host-supplied, not plugin-supplied, so a plugin
	// cannot attribute an upload to another plugin in the audit trail.
	if err := a.source.Upload(ctx, a.slug, locationID, key, data); err != nil {
		return fmt.Errorf("storage.upload: %w", err)
	}
	return nil
}

func (a *storageAdapter) Locations() []plugin.StorageLocationRef {
	if a.source == nil {
		return nil
	}
	in := a.source.Locations(context.Background())
	out := make([]plugin.StorageLocationRef, 0, len(in))
	for _, l := range in {
		out = append(out, plugin.StorageLocationRef{
			ID: l.ID, Name: l.Name, LocationType: l.LocationType,
		})
	}
	return out
}

func (a *storageAdapter) Available() bool {
	return a.source != nil && len(a.source.Locations(context.Background())) > 0
}

// UploadStream satisfies plugin.StorageStreamHandle.
//
// It is present on the adapter unconditionally so a plugin's type
// assertion always succeeds, and refuses at call time when the host
// wired a source that cannot stream — a plugin that asserted
// successfully and then found nothing would have no way to tell the
// difference between "not supported" and "failed".
func (a *storageAdapter) UploadStream(
	ctx context.Context,
	req plugin.StorageStreamRequest,
) (plugin.StorageStreamResult, error) {
	var out plugin.StorageStreamResult
	streamer, err := a.streamSource("uploadStream")
	if err != nil {
		return out, err
	}
	if strings.TrimSpace(req.LocationID) == "" {
		return out, errors.New("storage.uploadStream: locationID is required")
	}
	if strings.TrimSpace(req.Key) == "" {
		return out, errors.New("storage.uploadStream: key is required")
	}
	if req.Body == nil {
		return out, errors.New("storage.uploadStream: body is required")
	}

	// The slug is host-supplied, exactly as it is for Upload, so a plugin
	// cannot attribute an upload to another plugin in the audit trail.
	res, err := streamer.UploadStream(ctx, a.slug, StorageStreamDispatch{
		LocationID: req.LocationID,
		Key:        req.Key,
		Body:       req.Body,
		SizeHint:   req.SizeHint,
		Encrypt:    req.Encrypt,
		Progress:   req.Progress,
	})
	if err != nil {
		return out, fmt.Errorf("storage.uploadStream: %w", err)
	}
	return plugin.StorageStreamResult{
		RemotePath:   res.RemotePath,
		BytesWritten: res.BytesWritten,
		KeyID:        res.KeyID,
	}, nil
}

// DownloadStream opens a stored object for a plugin.
//
// The reader is the adapter's, handed straight through: buffering it here
// would reintroduce exactly the memory ceiling this interface exists to
// avoid.
func (a *storageAdapter) DownloadStream(
	ctx context.Context,
	req plugin.StorageDownloadRequest,
) (io.ReadCloser, error) {
	streamer, err := a.streamSource("downloadStream")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.LocationID) == "" || strings.TrimSpace(req.Key) == "" {
		return nil, errors.New("storage.downloadStream: locationID and key are required")
	}

	body, err := streamer.DownloadStream(ctx, a.slug, StorageDownloadDispatch{
		LocationID: req.LocationID,
		Key:        req.Key,
		Decrypt:    req.Decrypt,
	})
	if err != nil {
		return nil, fmt.Errorf("storage.downloadStream: %w", err)
	}
	return body, nil
}

// DeleteObject removes a stored object on a plugin's behalf.
func (a *storageAdapter) DeleteObject(ctx context.Context, locationID, key string) error {
	streamer, err := a.streamSource("deleteObject")
	if err != nil {
		return err
	}
	if strings.TrimSpace(locationID) == "" || strings.TrimSpace(key) == "" {
		return errors.New("storage.deleteObject: locationID and key are required")
	}
	if err := streamer.DeleteObject(ctx, a.slug, locationID, key); err != nil {
		return fmt.Errorf("storage.deleteObject: %w", err)
	}
	return nil
}

// streamSource asserts the streaming half of the bridge.
//
// Shared by all three so a host wired with only the buffered bridge refuses
// each of them the same way, by name, rather than one panicking and the
// others returning a confusing nil.
func (a *storageAdapter) streamSource(op string) (StorageStreamDispatchSource, error) {
	if a.source == nil {
		return nil, fmt.Errorf("storage.%s: no storage bridge configured on this host", op)
	}
	streamer, ok := a.source.(StorageStreamDispatchSource)
	if !ok {
		return nil, fmt.Errorf("storage.%s: this host's storage bridge cannot stream", op)
	}
	return streamer, nil
}

// commsHandleFor returns a plugin.CommsHandle, or nil when the host
// wired no comms bridge.
// workflowHandleFor scopes the workflow trigger bridge to one plugin:
// every key the plugin passes is turned into its own
// `plugin:<slug>:<key>` action, so a plugin can only ever fire the
// trigger nodes it registered itself.
func workflowHandleFor(source WorkflowTriggerSource, slug string) plugin.WorkflowHandle {
	if source == nil {
		return nil
	}
	if scoped, ok := source.(PluginScopedWorkflowSource); ok {
		source = scoped.ForPlugin(slug)
	}
	return &workflowAdapter{source: source, slug: slug}
}

// PluginScopedWorkflowSource is a WorkflowTriggerSource that can bind itself to one
// plugin, so the host side too refuses a fire for any action key but that plugin's own.
type PluginScopedWorkflowSource interface {
	ForPlugin(slug string) WorkflowTriggerSource
}

type workflowAdapter struct {
	source WorkflowTriggerSource
	slug   string
}

func (a *workflowAdapter) action(key string) string {
	return workflowNodeActionKey(a.slug, strings.TrimSpace(key))
}

func (a *workflowAdapter) TriggerBindings(ctx context.Context, key string) ([]plugin.WorkflowTriggerBinding, error) {
	if strings.TrimSpace(key) == "" {
		return nil, errors.New("workflows.bindings: key is required")
	}
	infos, err := a.source.TriggerBindings(ctx, a.action(key))
	if err != nil {
		return nil, err
	}
	out := make([]plugin.WorkflowTriggerBinding, 0, len(infos))
	for _, b := range infos {
		out = append(out, plugin.WorkflowTriggerBinding{WorkflowID: b.WorkflowID, NodeID: b.NodeID, Config: b.Config})
	}
	return out, nil
}

func (a *workflowAdapter) Fire(ctx context.Context, key string, payload map[string]any, match func(plugin.WorkflowTriggerBinding) bool) (int, error) {
	if strings.TrimSpace(key) == "" {
		return 0, errors.New("workflows.fire: key is required")
	}
	var m func(WorkflowTriggerBindingInfo) bool
	if match != nil {
		m = func(b WorkflowTriggerBindingInfo) bool {
			return match(plugin.WorkflowTriggerBinding{WorkflowID: b.WorkflowID, NodeID: b.NodeID, Config: b.Config})
		}
	}
	return a.source.Fire(ctx, a.action(key), payload, m)
}

func (a *workflowAdapter) DockerEnvironmentIDs(ctx context.Context) ([]string, error) {
	return a.source.DockerEnvironmentIDs(ctx)
}

func commsHandleFor(source CommsDispatchSource, slug string) plugin.CommsHandle {
	if source == nil {
		return nil
	}
	return &commsAdapter{source: source, slug: slug}
}

type commsAdapter struct {
	source CommsDispatchSource
	slug   string
}

func (a *commsAdapter) Send(ctx context.Context, channelID string, msg plugin.CommsMessage) error {
	if a.source == nil {
		return errors.New("comms.send: no communications bridge configured on this host")
	}
	if strings.TrimSpace(channelID) == "" {
		return errors.New("comms.send: channelID is required")
	}
	if strings.TrimSpace(msg.Title) == "" && strings.TrimSpace(msg.Message) == "" {
		return errors.New("comms.send: title and message are both empty")
	}

	files := make([]CommsDispatchFile, 0, len(msg.Files))
	for _, f := range msg.Files {
		files = append(files, CommsDispatchFile{
			Filename: f.Filename, ContentType: f.ContentType, Data: f.Data,
		})
	}
	if err := a.source.Send(ctx, a.slug, channelID, CommsDispatchMessage{
		Title:     msg.Title,
		Message:   msg.Message,
		Level:     msg.Level,
		Files:     files,
		Recipient: strings.TrimSpace(msg.Recipient),
	}); err != nil {
		return fmt.Errorf("comms.send: %w", err)
	}
	return nil
}

func (a *commsAdapter) Channels() []plugin.CommsChannelRef {
	if a.source == nil {
		return nil
	}
	in := a.source.Channels(context.Background())
	out := make([]plugin.CommsChannelRef, 0, len(in))
	for _, c := range in {
		out = append(out, plugin.CommsChannelRef{
			ID:                      c.ID,
			Name:                    c.Name,
			ChannelType:             c.ChannelType,
			IsDefault:               c.IsDefault,
			SupportsFileAttachments: c.SupportsFileAttachments,
			Recipient: plugin.CommsRecipientOverride{
				Supported:   strings.TrimSpace(c.Recipient.Key) != "",
				Kind:        c.Recipient.Kind,
				Label:       c.Recipient.LabelEN,
				Placeholder: c.Recipient.PlaceholderEN,
				Multiple:    c.Recipient.Multiple,
			},
		})
	}
	return out
}

func (a *commsAdapter) Available() bool {
	return a.source != nil && len(a.source.Channels(context.Background())) > 0
}

// --- reporting handle ----------------------------------------------

// reportingHandleFor returns the consumer half of the report-provider
// family, handed to every plugin on HostContext.Reporting.
//
// Unlike Storage and Comms this is not a host-module bridge: both ends
// of this family are plugins, and the registry they meet in is the
// loader's own. The handle is therefore a thin read view over
// adapterRegistry — no credential passes through it, and a provider is
// only ever reached by the report type key it registered.
//
// It is handed to every plugin rather than only to Reporting because
// the loader has no notion of "the reporting consumer"; a plugin that
// is not a report consumer simply never touches the field. Nothing
// reachable through it is privileged: the specs are already public
// catalog data, and Build runs the provider's own code with the calling
// user's cookie, which is exactly the permission boundary a direct
// request would have hit.
func reportingHandleFor(reg *adapterRegistry) plugin.ReportingHandle {
	if reg == nil {
		return nil
	}
	return &reportingAdapter{reg: reg}
}

type reportingAdapter struct {
	reg *adapterRegistry
}

func (a *reportingAdapter) Providers() []plugin.ReportProviderSpec {
	return a.reg.reportingSpecList()
}

func (a *reportingAdapter) Available() bool {
	return a.reg.reportingCount() > 0
}

func (a *reportingAdapter) Build(ctx context.Context, typeKey string, req plugin.ReportRequest) (plugin.ReportResult, bool, error) {
	cli, ok := a.reg.reportingFor(typeKey)
	if !ok {
		return plugin.ReportResult{}, false, nil
	}
	// req.TypeKey is set from the dispatch key rather than trusted from
	// the caller, so a provider answering several types can switch on it
	// without re-checking that it matches what it was dispatched for.
	req.TypeKey = typeKey
	res, err := cli.Build(ctx, req)
	if err != nil {
		return plugin.ReportResult{}, true, fmt.Errorf("reporting provider %s: %w", typeKey, err)
	}
	return res, true, nil
}

func (a *reportingAdapter) Subjects(ctx context.Context, typeKey, kind string, req plugin.ReportRequest) ([]plugin.ReportSubjectOption, bool, error) {
	cli, ok := a.reg.reportingFor(typeKey)
	if !ok {
		return nil, false, nil
	}
	req.TypeKey = typeKey
	out, err := cli.Subjects(ctx, kind, req)
	if err != nil {
		return nil, true, fmt.Errorf("reporting provider %s subjects: %w", typeKey, err)
	}
	return out, true, nil
}

// --- RBAC handle ----------------------------------------------------

// rbacHandleFor returns the read-only permission surface handed to a
// plugin on HostContext.RBAC.
//
// Read-only by design: a plugin may ask whether an account holds a key,
// and may attach an i18n label to a key it declared. It may not grant,
// revoke or define anything at runtime — permission *definitions* go
// through AccessRegistry at install time, which is the one moment the
// host can audit them.
//
// The checker is RouterDeps.HasPermission, the same function the
// loader's own per-route middleware uses. That is deliberate: a plugin
// asking "may this account read my data" gets exactly the answer the
// plugin's own routes would have given, so a capability reached through
// a report and the same capability reached through a page can never
// diverge.
func rbacHandleFor(deps RouterDeps, labels *plugin.LabelRegistry, slug string) plugin.RBACHandle {
	if deps == nil {
		return nil
	}
	return &rbacAdapter{deps: deps, labels: labels, slug: slug}
}

type rbacAdapter struct {
	deps   RouterDeps
	labels *plugin.LabelRegistry
	slug   string
}

// HasPermission answers whether an account holds a key.
//
// An empty userID is denied without consulting the host: there is no
// "anonymous" account to check, and passing an empty id into the RBAC
// service would ask a question about no one and get an answer a caller
// might read as permission.
func (a *rbacAdapter) HasPermission(userID, envID, perm string) (bool, error) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(perm) == "" {
		return false, nil
	}
	return a.deps.HasPermission(userID, envID, perm)
}

func (a *rbacAdapter) RegisterLabel(key, ns, labelKey string) {
	if a.labels == nil {
		return
	}
	a.labels.Register(key, ns, labelKey)
}

func (a *rbacAdapter) UnregisterLabel(key string) {
	if a.labels == nil {
		return
	}
	a.labels.Unregister(key)
}

// agentRPCFor hands a plugin its view of the agent-rpc bridge, narrowed to its agent-rpc
// declaration, and only when its manifest also carries the agent.rpc token: the token is the
// grant an operator reviews at install, the declaration is what it grants. Either alone yields
// nil, and nil is returned untyped so a plugin's assertion fails rather than finding a nil view.
func agentRPCFor(b *agentrpc.Bridge, slug string, perms []string, decl *plugincaps.Capability) any {
	if b == nil || decl == nil || !slices.Contains(perms, agentRPCToken) {
		return nil
	}
	return b.ForPlugin(slug, decl.Send, decl.Observe)
}

// agentRPCDeclFor reads the plugin's agent-rpc declaration from its installed manifest,
// nil when it has none or its capabilities do not validate.
func agentRPCDeclFor(slug, dir string) *plugincaps.Capability {
	c, ok := plugincaps.AgentRPCDeclaration(slug, dir)
	if !ok {
		return nil
	}
	return &c
}

// agentRPCToken is the manifest permission that grants the agent-rpc view.
const agentRPCToken = "agent.rpc"

// closeRows releases a result set whose rows were already consumed; a close
// failure cannot undo the read, so it is logged instead of returned.
func closeRows(logger *slog.Logger, rows *sql.Rows, what string) {
	if err := rows.Close(); err != nil && logger != nil {
		logger.Warn("plugin runtime: closing rows failed", "rows", what, "error", err)
	}
}
