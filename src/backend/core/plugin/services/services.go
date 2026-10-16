// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package services defines the host-side service handles and their
// reference interfaces that plugins read via field access on
// `contract.HostContext` (DB, KVStore, Encryption, DockerPool,
// I18n, Audit, RBAC, IdentityProvisioner, SessionIssuer, AI, and
// the Snapshot RPC handle).
//
// Why split: the previous monolithic `core/plugin` package kept
// every handle type alongside the lifecycle types. The host's
// `loader.go` references every handle (it constructs adapters that
// implement each one), so the host's reach set of `services`
// already covers the full set of handle types. By moving the handle
// types here, the plugin's reach set of `services` — forced by the
// `contract.HostContext` struct field types — lines up exactly.
//
// `services` depends on `specs/snapshot` for the snapshot capability
// / ref types used by `SnapshotRPCHandle` signatures. No other
// spec-family dependency: the agent-rpc bridge a plugin reaches the
// agent through is `any` here, asserted against builtin types only.
//
// Imported by:
//   - the host loader (`core/pluginruntime/loader.go`)
//   - host modules that consume plugin-contributed capabilities
//     (storage_locations, communications, container_backups,
//     ai_assistant, identity)
//   - every plugin (to type its `host` field)
package services

import (
	"context"
	"io"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/snapshot"
)

// DBHandle is the minimal DB surface a plugin may use (only its own
// tables — prefix `plugin_<slug>_`). Direct access to the host's
// tables is a contract violation caught at install time.
type DBHandle interface {
	// Exec runs a statement against the plugin's namespace. Statements
	// touching tables outside `plugin_<slug>_*` return ErrScopeViolation.
	Exec(ctx context.Context, query string, args ...any) (int64, error)
	// Query returns rows. Same scope check as Exec.
	Query(ctx context.Context, query string, args ...any) (Rows, error)
	// KV is the per-plugin key/value store: Get/Set/Delete.
	KV() KVStore
}

// Rows is the row cursor a plugin query returns, mirroring database/sql so plugins need no driver import.
type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

// KVStore is the per-plugin key-value store the host provides for small persistent state.
type KVStore interface {
	Get(ctx context.Context, key string) (string, bool, error)
	Set(ctx context.Context, key, value string) error
	Delete(ctx context.Context, key string) error
}

// EncryptionHandle is read-only access to the host's encryption
// service for plugin-owned secret fields.
type EncryptionHandle interface {
	EncryptString(plain string) (string, error)
	DecryptString(cipher string) (string, error)
}

// PoolHandle exposes the host's orchestrator client pools. Plugins
// must declare `docker.socket.{read,write}` in the manifest to use it.
type PoolHandle interface {
	ForKind(kind string) any // typed in the host; plugins type-assert
}

// DockerPoolAccessor is the typed slice of the host's docker client
// pool, returned by PoolHandle.ForKind("docker"). Plugins call
// Get / IsAgentEnv on it; the underlying *docker.ClientPool is
// resolved by the host at call time.
type DockerPoolAccessor struct {
	get   func(envID string) (any, error)
	agent func(envID string) bool
}

// Get returns the docker client for envID (any = *docker.Client).
// The plugin type-asserts: cli, _ := acc.Get(envID); cli.(*docker.Client).
func (a DockerPoolAccessor) Get(envID string) (any, error) { return a.get(envID) }

// IsAgentEnv reports whether envID is reached via the agent transport.
func (a DockerPoolAccessor) IsAgentEnv(envID string) bool { return a.agent(envID) }

// NewDockerPoolAccessor builds a DockerPoolAccessor from the two
// host-side callbacks. The loader uses this constructor when
// assembling each plugin's HostContext; plugins receive the
// fully-typed accessor via host.DockerPool.
//
// Defining the constructor here (rather than in the loader) keeps
// the unexported `get`/`agent` fields settable from a single
// package while exposing a typed value to plugins. The loader
// doesn't need to import the docker SDK — it just hands in the
// pool accessor callbacks it already owns.
func NewDockerPoolAccessor(
	get func(envID string) (any, error),
	agent func(envID string) bool,
) DockerPoolAccessor {
	return DockerPoolAccessor{get: get, agent: agent}
}

// I18nHandle registers plugin-owned translations per language.
type I18nHandle interface {
	Register(lang, namespace string, bundle map[string]string) error
}

// AuditHandle emits read-only audit events tagged with origin=<slug>.
type AuditHandle interface {
	Emit(event string, fields map[string]any) error
}

// EnvironmentHandle answers what one of the host's environments is called.
//
// A plugin's DB handle is scoped to its own tables, so it cannot read the
// environments table -- and it should not want to. This is the host answering
// a question about its own data, the same shape as every other handle here.
type EnvironmentHandle interface {
	// Name returns the environment's display name, or the id when it has none
	// or no longer exists. It never returns an empty string, so a caller
	// building a path segment from it always has something to slug.
	Name(ctx context.Context, environmentID string) string
}

// BackupLogHandle lets a plugin that backs something up write to the host's
// backup log, so its lifecycle events land in the same feed as the host's own.
//
// The log is a host table because the Backups page filters and paginates it
// server-side; a plugin keeping its own and the host merging in memory would
// give wrong page boundaries. Writing through the host keeps one query.
type BackupLogHandle interface {
	Record(ctx context.Context, entry BackupLogEntry) error
}

// BackupLogEntry is one lifecycle event from a backup provider.
//
// SubjectID and SubjectName are what the row is about, whatever that is for
// the provider; PlanID and RunID are the provider's own ids and never reach
// the container foreign keys.
type BackupLogEntry struct {
	EnvironmentID string
	PlanID        string
	PlanName      string
	RunID         string
	SubjectID     string
	SubjectName   string
	// SubjectKind mirrors BackupProviderSpec.SubjectKind ("vm"), so the feed
	// labels and filters the row by the same value the Runs table shows.
	SubjectKind string
	// Action is the event ("backup.started", "backup.completed"), Phase the
	// step within it, both free text the feed renders as given.
	Action string
	Phase  string
	// Severity is "info", "success", "warning" or "error". Anything else is
	// recorded as "info" rather than rejected, because a dropped log line is
	// worse than an imprecise one.
	Severity string
	Message  string
}

// AuditEvent is an audit row that names the account that asked for it.
type AuditEvent struct {
	Action string
	// UserID is the authenticated account behind the request. Empty writes an
	// unattributed row, which is right for a scheduler or a reconciler.
	UserID string
	// Severity empty defers to the host's action catalog.
	Severity string
	Fields   map[string]any
}

// AuditActorHandle is the actor-attributing half of AuditHandle.
//
// Separate rather than folded into AuditHandle because a plugin's `.so` is
// built against the interface's method set: adding a method there would make
// every already-built plugin fail to load. A plugin reaches this with a type
// assertion on host.Audit and falls back to Emit when the host predates it.
type AuditActorHandle interface {
	EmitEvent(ev AuditEvent) error
}

// LifecycleEvent is one entry on the Logging timeline — what happened to which
// object, and what state it ended in.
type LifecycleEvent struct {
	EnvironmentID string
	// SubjectType is the object family the Logging page filters on: "vm",
	// "pool", "container", "image", "volume", "network", "stack". A type the
	// frontend does not know still lists, it just cannot be filtered to.
	SubjectType string
	SubjectID   string
	SubjectName string
	// EventType is the producer's own category, e.g. "guest" or "storage".
	EventType string
	Action    string
	// State is the state the subject settled in, when the action produced one.
	State string
	// Severity empty is recorded as "info".
	Severity string
	Metadata map[string]any
}

// LifecycleHandle records a subject's state change on the Logging timeline.
//
// Until this existed lifecycle_events was written only by the Docker event
// collector, so a plugin that manages objects of its own — virtual machines,
// storage pools — had nowhere to say that one started or was destroyed.
type LifecycleHandle interface {
	Record(ctx context.Context, ev LifecycleEvent) error
}

// ProvisionRequest is what an identity plugin passes to the host's
// IdentityProvisioner when a remote user has authenticated and needs
// to be linked to (or created as) a McHarbor user.
type ProvisionRequest struct {
	ProviderSlug  string   // slug of the identity plugin (e.g. "identity-oidc")
	ProviderID    string   // identity-provider row id within the plugin
	Subject       string   // external subject claim (sub, NameID, DN)
	Username      string   // preferred username; host may apply its own rules
	Email         string   // may be empty
	DisplayName   string   // may be empty
	Groups        []string // provider-side group names; host applies group mapping
	AutoProvision bool     // when false, returns ErrNotFound if no matching user
	DefaultRoleID string   // host applies if a new user is created
	// UsernamePrefix is prepended to generated usernames so accounts from
	// different providers cannot collide (e.g. "ldap_"). Empty means the
	// host's generic "idp_" prefix.
	UsernamePrefix string
}

// ProvisionedUser is the result of a successful provision call.
type ProvisionedUser struct {
	ID          string
	Username    string
	DisplayName string
	Email       string
}

// IdentityProvisioner is the host's stable callback for plugins that
// authenticate users from external providers (OIDC, LDAP, SAML, ...).
// Plugins MUST NOT touch core `users` / `groups` / `group_members` /
// `user_roles` tables directly — they go through this hook.
type IdentityProvisioner interface {
	ProvisionUser(ctx context.Context, req ProvisionRequest) (*ProvisionedUser, error)
}

// SessionIssuer creates a new authenticated session for the given
// user. The returned sessionID is opaque to the plugin; the host is
// the source of truth for cookie name, lifetime, and persistence.
type SessionIssuer interface {
	IssueSession(userID string) (sessionID string, expiresInSec int, err error)
	// SessionCookieName returns the name of the auth cookie the host
	// expects on protected routes. The plugin writes the cookie
	// with this name in its public callback handlers.
	SessionCookieName() string
	// SessionIsSecure reports whether the host considers cookies
	// Secure-required given the current request (X-Forwarded-Proto,
	// trusted-proxy chain, etc.).
	SessionIsSecure(r *http.Request) bool
}

// RBACHandle is the read-only permission check surface plus a
// plugin-owned label registry. Plugins that declare permission
// keys via RegisterPermissions call RegisterLabel at Init() so the
// host can surface the human-readable name in the admin UI without
// hardcoding it in the host's locale files (see dependency-host-plugin.md
// §6.4). The host persists labels in memory; /plugins/loader/permission-labels
// publishes them to the frontend, which mirrors the entry into the
// `permissions` i18next namespace so the existing
// `t('permissions.' + key)` calls pick the plugin-provided string.
type RBACHandle interface {
	HasPermission(userID, envID, perm string) (bool, error)
	// RegisterLabel associates a permission key with a label sourced
	// from the plugin's i18n namespace. ns is the plugin slug (or
	// any namespace the plugin's i18n bundles register under);
	// labelKey is the dotted path inside that namespace. Plugins
	// MUST call this for every AddsPerms entry they ship; missing
	// labels fall back to the raw permission key in the UI.
	RegisterLabel(key, ns, labelKey string)
	// UnregisterLabel removes a previously registered label. The
	// runtime calls it during plugin Shutdown so uninstalls wipe
	// the entry; plugins don't need to call it themselves.
	UnregisterLabel(key string)
}

// AIHandle is the read-only entry point into McHarbor's core AI
// router. Plugins that want to summarize logs, draft incident reports,
// generate commit messages, etc. invoke it through this handle.
// Requires the `ai.invoke` scope token in the manifest's
// `permissions:` list.
//
// Plugins MAY pin a specific configured connection via
// AIRequest.ConnectionID instead of letting the host pick one at
// call time. This deliberately reverses the router's original
// "plugins do not pin a provider" behavior: a report handed to a
// third party must be reproducible on the named model it was
// generated with, not silently re-routed to a different provider on
// a later replay. Leaving ConnectionID empty preserves the original
// behavior — the host resolves its default/favourite connection.
//
// All calls are synchronous; the host enforces per-plugin rate limits
// (default 60/min) and a per-call timeout (default 30s). The plugin
// receives the assembled provider response, never the API key.
type AIHandle interface {
	// Complete sends a chat-style completion request. The plugin may
	// pass any number of prior messages; the host prepends the active
	// system prompt if one is configured globally.
	Complete(ctx context.Context, req AIRequest) (AIResponse, error)
	// Available reports whether the AI router has at least one
	// configured provider plugin. Plugins can use this to skip
	// gracefully when AI is not provisioned.
	Available() bool
	// Connections lists the connections a plugin may pin
	// AIRequest.ConnectionID to. Each entry carries id, name,
	// provider_type, and whether it is the host's default — never a
	// credential. Plugins use this to build a selector UI (e.g. "which
	// AI should write this report") without ever seeing an API key.
	Connections() []AIConnection
}

// AIConnection is the plugin-visible summary of one configured AI
// connection. It deliberately carries NO credential: no api_key, no
// base URL (which could embed one), no secret of any kind — only
// enough to label a connection and let a plugin pin AIRequest.ConnectionID
// to it.
type AIConnection struct {
	ID           string // stable id; pass back as AIRequest.ConnectionID
	Name         string // human-readable label
	ProviderType string // dispatch key, matches ai_provider_connections.provider_type
	IsDefault    bool   // the host's current default/favourite connection
}

// --- Storage delivery -------------------------------------------

// StorageHandle lets a plugin write a file to one of the host's
// configured storage locations.
//
// The plugin names a location by **id** and the host resolves the row,
// decrypts its config, and dispatches to the storage adapter that owns
// that location type. A plugin therefore never holds an access key, a
// bearer token, or an SFTP password — the same discipline AIHandle
// follows for provider credentials.
type StorageHandle interface {
	// Upload writes data to key under the given location. key is
	// relative to the location's configured base path; intermediate
	// directories are created by the adapter.
	//
	// data is []byte rather than io.Reader on purpose: the storage
	// adapter contract wants a fresh reader per call and does not own
	// it past return, so passing a reader across the plugin boundary
	// would make its lifetime a shared problem. Callers are expected
	// to keep uploads to a size that fits comfortably in memory — a
	// report or an export, not a disk image.
	Upload(ctx context.Context, locationID, key string, data []byte) error

	// Locations lists what a plugin may upload to. Each entry carries
	// id, name and type — never a credential — so a plugin can build a
	// destination picker.
	Locations() []StorageLocationRef

	// Available reports whether the host has at least one usable
	// storage location, so a plugin can degrade rather than fail.
	Available() bool
}

// StorageLocationRef is one selectable storage destination.
//
// There is deliberately no IsDefault: unlike communication channels,
// storage locations have no default in the schema, and a field that is
// always false invites a caller to build a picker around it.
type StorageLocationRef struct {
	ID           string // stable id; pass back as Upload's locationID
	Name         string // human-readable label
	LocationType string // dispatch key, matches storage_locations.location_type
}

// StorageStreamHandle moves an artifact far too large to hold in memory to
// and from a storage destination, and removes one.
//
// Asserted by the caller rather than added to StorageHandle on purpose:
// adding a method to an existing handle changes its ABI, and every plugin
// already built against the old shape then stops loading.
//
// The three belong together because they are the lifecycle of one object: a
// backup that can be written but not read back is not a backup, and one that
// cannot be removed makes a retention policy a fiction.
type StorageStreamHandle interface {
	UploadStream(ctx context.Context, req StorageStreamRequest) (StorageStreamResult, error)
	// DownloadStream opens a stored object. The caller closes the reader.
	//
	// Decrypt undoes what UploadStream's Encrypt did, in flight, so the
	// master key stays on the host at both ends.
	DownloadStream(ctx context.Context, req StorageDownloadRequest) (io.ReadCloser, error)
	// DeleteObject removes a stored object. Absent is not an error: a
	// retention pass that already ran, or an object an operator removed by
	// hand, must not fail the sweep behind it.
	DeleteObject(ctx context.Context, locationID, key string) error
}

// StorageDownloadRequest is one streamed read from a destination.
type StorageDownloadRequest struct {
	LocationID string
	Key        string
	// Decrypt unwraps the backup envelope on the way out. It must match how
	// the object was written: reading an encrypted object without it yields
	// the envelope rather than the artifact, which fails later and further
	// away than it should.
	Decrypt bool
}

// StorageStreamRequest is one streamed upload to one location.
//
// Body is consumed exactly once and is never retried by the host: a stream
// has no second read, so a failed upload is reported back and the caller —
// which owns the source — decides whether to produce the bytes again.
type StorageStreamRequest struct {
	LocationID string
	// Key is relative to the location's configured base path, as
	// StorageHandle.Upload's key is.
	Key  string
	Body io.Reader
	// SizeHint is the artifact's size when it is known, or -1. Adapters
	// that need a content length ahead of the body use it; the upload is
	// refused rather than guessed at when one needs it and it is absent.
	SizeHint int64
	// Encrypt wraps the stream in McHarbor's backup envelope on its way to
	// the destination. The master key stays on the host: this is the reason
	// the handle exists rather than the plugin encrypting for itself.
	Encrypt bool
	// Progress is called as bytes reach the adapter. It must not block —
	// it runs on the upload path.
	Progress func(written int64)
}

// StorageStreamResult is where the artifact landed and what it cost.
//
// KeyID identifies the master key the envelope was sealed with, so a later
// restore can say "this needs a different key" instead of failing to
// decrypt with no explanation.
type StorageStreamResult struct {
	RemotePath   string
	BytesWritten int64
	KeyID        string
}

// --- Communication delivery --------------------------------------

// CommsHandle lets a plugin send a message, optionally carrying files,
// to one of the host's configured communication channels.
//
// As with StorageHandle the plugin names a channel by id and the host
// resolves and decrypts it. Files reach the provider only when its
// adapter declared it can carry them; otherwise the host drops them and
// the message text stands, so a caller that wants the recipient to get
// *something* should put a link in Message.
type CommsHandle interface {
	// Send delivers msg to the given channel.
	Send(ctx context.Context, channelID string, msg CommsMessage) error

	// Channels lists what a plugin may send to. SupportsFileAttachments
	// tells a plugin whether a document will actually arrive, so a
	// target picker can say "link only" rather than promising an
	// attachment the provider cannot carry.
	Channels() []CommsChannelRef

	// Available reports whether the host has at least one configured
	// channel.
	Available() bool
}

// CommsChannelRef is one selectable communication channel.
type CommsChannelRef struct {
	ID          string // stable id; pass back as Send's channelID
	Name        string // human-readable label
	ChannelType string // dispatch key, matches communication_channels.channel_type
	IsDefault   bool

	// SupportsFileAttachments mirrors the adapter's declared
	// capability. False means Files will be dropped before the
	// provider sees them.
	SupportsFileAttachments bool

	// Recipient describes the channel's overridable destination, or
	// is zero when the adapter behind it has none. A plugin reads it
	// to decide whether to offer an "send this one somewhere else"
	// field, and what to call it.
	Recipient CommsRecipientOverride
}

// CommsRecipientOverride is what a channel will accept in
// CommsMessage.Recipient.
//
// Mirrors the adapter's own declaration (see
// specs/notification.NotificationRecipientOverride). Supported is the
// field to branch on: everything else is presentation.
type CommsRecipientOverride struct {
	// Supported is false for a webhook- or token-bound provider,
	// where the destination is the credential and cannot be varied
	// per message. Setting CommsMessage.Recipient on such a channel
	// is an error rather than a silent no-op, so a caller never
	// believes it retargeted a message that in fact went to the
	// original destination.
	Supported bool

	// Kind is "email", "phone", "chat_id", "topic", or empty for
	// free text. A picker uses it to validate and to label.
	Kind string

	// Label and Placeholder are the adapter's English source
	// strings, for a caller with no bundle of its own.
	Label       string
	Placeholder string

	// Multiple reports whether a comma-separated list is accepted.
	Multiple bool
}

// CommsMessage is what a plugin sends through a channel.
type CommsMessage struct {
	Title   string // one-line subject; required
	Message string // body; may contain line breaks
	Level   string // "info", "warning", "error"; optional

	// Files are optional attachments, delivered only to channels whose
	// adapter declared SupportsFileAttachments.
	Files []CommsFile

	// Recipient retargets this one message, leaving the stored
	// channel untouched: a channel configured to mail the ops list
	// carries a single report to a board member, and the next send
	// through the same channel goes to the ops list again.
	//
	// Empty means "the channel's own destination", which is what
	// every existing caller does. Non-empty is refused unless the
	// channel's CommsChannelRef reported Recipient.Supported, so a
	// silent delivery to the wrong place is not a possible outcome.
	//
	// The host writes the value into the single config key the
	// adapter declared and no other: a plugin cannot reach
	// `webhook_url`, `password` or `bot_token` through this field.
	Recipient string
}

// CommsFile is one file to transmit through a channel.
type CommsFile struct {
	Filename    string // what the recipient sees, including extension
	ContentType string // MIME type, e.g. "application/pdf"
	Data        []byte
}

// AIRequest is the shape plugins send to the AI router. Field names
// mirror the OpenAI chat completions schema so provider plugins can
// translate without renaming.
type AIRequest struct {
	Model       string            // optional override; empty = host default
	Messages    []AIMessage       // ordered, oldest first
	Temperature *float32          // nil = host default
	MaxTokens   *int              // nil = host default
	Stop        []string          // optional stop sequences
	Metadata    map[string]string // forwarded to providers for tracing
	// ConnectionID optionally pins this request to one configured AI
	// connection (see AIConnection / AIHandle.Connections). Empty
	// means "the host's default/favourite connection" — this is the
	// pre-existing behavior and every caller that leaves it unset
	// keeps working unchanged.
	ConnectionID string
}

// AIMessage is one chat turn.
type AIMessage struct {
	Role    string // "system" | "user" | "assistant" | "tool"
	Content string
	Name    string // optional, for tool/function messages
}

// AIResponse is the assembled provider response.
type AIResponse struct {
	Content  string
	Model    string
	Provider string            // slug of the provider plugin that answered
	Usage    AIUsage           // token accounting
	Metadata map[string]string // echoed request metadata + provider extras
}

// AIUsage is best-effort token accounting from the provider.
type AIUsage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

// SnapshotRPCHandle is the host's bridge for plugins that need to
// call snapshot operations through the remote-agent transport. Used
// by local-host plugins (zfs, btrfs, lvm) that wrap CLI tools on the
// agent host. HTTP-based NAS plugins (synology, truenas) don't need
// this — they call their provider's REST API directly from the host
// process.
//
// The host guarantees a non-nil SnapshotRPC for every plugin whose
// manifest declares `snapshot.adapter.register`; plugins that don't
// declare that scope can still receive a nil value and should ignore
// it. The agent transport is per-environment: envID is the row id of
// the target environment (the same value host code passes to the
// docker client pool's Get / IsAgentEnv).
//
// All methods honor the caller's context for cancellation and
// deadlines. The host enforces a 60-second timeout on the underlying
// agent RPC; plugins SHOULD apply their own shorter deadline via
// context.WithTimeout when the snapshot operation has its own
// narrower SLA.
type SnapshotRPCHandle interface {
	// Capability probes the agent for the available snapshot targets
	// (datasets, subvolumes, LVs). Returns Available=false with an
	// Error string when no snapshot-capable tool is present or the
	// agent is unreachable.
	Capability(ctx context.Context, envID string) (snapshot.SnapshotCapability, error)
	// Create asks the agent to take a snapshot of the given targets.
	// Returns a SnapshotRef the plugin then forwards to the host's
	// backup pipeline (the host persists it on the backup run row).
	Create(ctx context.Context, envID string, req snapshot.SnapshotCreateRequest) (snapshot.SnapshotRef, error)
	// Remove asks the agent to remove a previously-created snapshot.
	// Best-effort: a non-nil error is logged but does not fail the
	// surrounding operation, mirroring the plugin's own Remove
	// semantics.
	Remove(ctx context.Context, envID string, ref snapshot.SnapshotRef) error
}

// WorkflowTriggerBinding is one trigger node of the calling plugin,
// placed on an active workflow's canvas: the node's saved config is
// what the plugin's watcher matches an event against.
type WorkflowTriggerBinding struct {
	WorkflowID string
	NodeID     string
	Config     map[string]any
}

// WorkflowHandle lets a plugin start workflows from its own trigger
// nodes. Keys are plugin-local (the key passed to NodeRegistry.Add);
// the host scopes them to the plugin, so one plugin can never fire
// another's triggers. nil when the host wired no workflow bridge.
type WorkflowHandle interface {
	// TriggerBindings lists the active placements of the plugin's
	// trigger node `key`, so a watcher can decide what to observe
	// (which environments, which clusters) instead of watching
	// everything.
	TriggerBindings(ctx context.Context, key string) ([]WorkflowTriggerBinding, error)
	// Fire starts every active workflow whose trigger node `key`
	// accepts the event: match decides per placement from the node's
	// config (nil = every placement). payload becomes msg.payload;
	// the run's environment is the node's "environment" config when
	// set. Returns how many workflow runs were started.
	Fire(ctx context.Context, key string, payload map[string]any, match func(WorkflowTriggerBinding) bool) (int, error)
	// DockerEnvironmentIDs lists the host's active Docker
	// environments, for watchers that subscribe per environment.
	DockerEnvironmentIDs(ctx context.Context) ([]string, error)
}

// CatalogSourceRef is one enabled catalog row as a plugin sees it. The row's
// id, origin and enabled flag stay host-side: a plugin reads sources to load
// from, it does not manage the registry.
type CatalogSourceRef struct {
	Name string
	URL  string
	// ReadOnly marks a built-in or plugin-contributed row, whose URL is not
	// operator-reachable state. A plugin uses it the way the app store does:
	// to decide whether the payload was vouched for or merely added.
	ReadOnly bool
}

// CatalogHandle lets a plugin read the enabled catalog sources of a kind it
// owns, so an operator manages every source in one place — Settings →
// Catalogs — instead of the plugin growing a second registry beside it.
//
// The host guarantees a non-nil CatalogHandle for every plugin whose manifest
// declares `catalog.source.register`; anything else may receive nil and must
// treat that as "no sources configured" rather than dereferencing it.
//
// kind is a host-known catalog kind ("vm_image", "docker_app", "plugin",
// "theme"). A plugin may read any kind, but in practice reads the one it
// contributes to: the host does not stop addon-compute from listing plugin
// catalogs, it simply has no reason to.
type CatalogHandle interface {
	// Sources returns the enabled rows of kind, built-in first then by name,
	// which is the order the app and plugin stores already read them in.
	Sources(ctx context.Context, kind string) ([]CatalogSourceRef, error)
	// Available reports whether the host wired a catalog registry at all.
	Available() bool
}
