// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"context"
	"errors"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/pluginruntime"
)

// Source is the installable input. Exactly one of URL or Tarball
// is set per call. Checksum is optional client-supplied validation.
//
// Signature is the base64 ed25519 signature over the tarball, taken
// from the catalog entry that named it. It is optional: a plugin with
// no signature installs once AllowUnsigned confirms it, is recorded as
// unsigned, and is labelled that way everywhere it appears. When it is absent the installer still
// looks for a <tarball>.sig sidecar before concluding as much.
type Source struct {
	URL       string
	Tarball   []byte
	Checksum  string
	Signature string
	// Build is the artifact's own yyyyMMddHHmm compile stamp, recorded on the
	// installed row so a later catalog can tell whether a rebuild is newer.
	Build string
	// BuiltFor names the McHarbor release the artifact was compiled for,
	// taken from the catalog entry. Empty skips the compatibility check.
	BuiltFor string
	// AllowUnsigned is the operator's explicit consent to install a tarball no
	// trust key verifies. Without it an unsigned install is refused with
	// ErrUnsignedNotConfirmed, so a dialog skipped in devtools cannot bypass it.
	AllowUnsigned bool
}

// SignatureProber is implemented by installers that can resolve a source and
// report its signature state without installing it. Update uses it to refuse an
// unconfirmed unsigned release before the current install is torn down.
type SignatureProber interface {
	ProbeSignature(ctx context.Context, src Source) (Source, bool, error)
}

// Installed is the result of a successful Install.
type Installed struct {
	Slug        string
	Version     string
	Path        string
	NeedsEnable bool
}

// ProgressFunc receives human-readable progress messages during
// Install / Uninstall. Used by the SSE streaming endpoints.
type ProgressFunc func(stage string, pct int, message string)

// Installer is the contract the host uses to manage plugin
// lifecycle. The default implementation lives in installer_default.go.
type Installer interface {
	Install(ctx context.Context, src Source, progress ProgressFunc) (*Installed, error)
	Uninstall(ctx context.Context, slug string, opts UninstallOptions, progress ProgressFunc) (*Uninstalled, error)
}

// Contributor is implemented by the runtime so the installer can
// persist route / menu / widget / node contributions as plugins
// register them. Same shape as plugin.ContributionTracker but lives
// here to avoid a circular import.
type Contributor interface {
	RecordRoute(ctx context.Context, origin, method, path, requiresPermission string, isPublic bool) error
	RecordMenu(ctx context.Context, origin, parent, labelNS, labelKey, path, icon, permission string, priority int) error
	RecordTab(ctx context.Context, origin, pageID, key, labelNS, labelKey, icon, permission string, sortOrder int) error
	RecordWidget(ctx context.Context, origin, key, title, category, defaultSize, settingsJSON string) error
	RecordNode(ctx context.Context, origin, key, category, displayName, icon, configSchemaJSON, inputPortsJSON, outputPortsJSON, requiresPermission string) error
	RecordTheme(ctx context.Context, origin, id, name, description, cssPath string, swatchHue int, defaultVariant string, hasDark, hasLight bool) error
	RecordCatalog(ctx context.Context, origin, catalogID, kind, name, url string) error
	RecordNotificationAdapter(ctx context.Context, origin, channelType, displayName string, supportsFiles, supportsRich, encryptedCredentials bool, recipient pluginruntime.NotificationRecipientDecl) error
	RecordCLIRoot(ctx context.Context, origin, name, short, aliasesJSON string) error
	RecordCLICommand(ctx context.Context, origin string, cmd pluginruntime.CLICommandRow) error
	DropCLI(ctx context.Context, origin string) error
	ClearAll(ctx context.Context, origin string) error
}

// UninstallOptions carries the caller's intent into an uninstall.
//
// It is a struct rather than a second method so every call site states what it
// wants: the update path reinstalls immediately afterwards and must not purge,
// and that has to be a visible decision rather than a default inherited by
// accident.
type UninstallOptions struct {
	// PurgeData also removes what an ordinary uninstall keeps: the plugin's
	// config rows, encrypted secrets included, and any catalog source left
	// behind for a kind no remaining plugin declares.
	PurgeData bool
	// KeepTables leaves the plugin's own tables, and the rows in them, where
	// they are. Set by the two paths that take a plugin out only to put it
	// back — a repair after a host rebuild, and an update to a newer version —
	// where dropping them costs the operator every guest, pool and network
	// they defined, for a reason that has nothing to do with any of them.
	KeepTables bool
	// SetAside moves the install directory to the rollback area instead of
	// deleting it, keeps the stored package and skips the dependency prune, so
	// an upgrade whose new install fails can put the previous version back.
	SetAside bool
}

// Staged is an artifact that passed every check Install makes before it touches
// the host: resolved, verified, extracted and validated, but not yet placed.
// Installing it installs exactly these bytes, with no second download.
type Staged struct {
	Source   Source
	Dir      string
	Slug     string
	Version  string
	Signed   bool
	manifest *plugin.Manifest
	tarball  []byte
}

// PreviousInstall describes the version an upgrade replaces, as persisted in
// the update snapshot, so a rollback can reinstate its row exactly.
type PreviousInstall struct {
	Slug     string
	Version  string
	Build    string
	BuiltFor string
	Signed   bool
}

// Upgrader is implemented by installers that can validate an artifact before
// the installed copy is touched and reinstate the previous one afterwards. The
// update and repair paths use it so a refused artifact never costs a plugin.
type Upgrader interface {
	Preflight(ctx context.Context, src Source, progress ProgressFunc) (*Staged, error)
	InstallStaged(ctx context.Context, st *Staged, progress ProgressFunc) (*Installed, error)
	DiscardStaged(st *Staged)
	RestorePrevious(ctx context.Context, prev PreviousInstall) error
	HasPrevious(slug, version string) bool
	FinishUpgrade(ctx context.Context, slug, prevVersion, newVersion string)
}

// Public errors returned from the installer. Handlers translate
// these into i18n error codes.
var (
	ErrAlreadyInstalled    = errors.New("plugin already installed")
	ErrSignatureInvalid    = errors.New("plugin signature invalid")
	ErrManifestInvalid     = errors.New("plugin manifest invalid")
	ErrPermissionInUse     = errors.New("permission still referenced by a role")
	ErrPluginNotFound      = errors.New("plugin not found")
	ErrPlatformUnsupported = errors.New("plugin runtime unsupported on this platform")
	ErrBuiltForMismatch    = errors.New("plugin built for a different McHarbor release")
	// ErrUnsignedNotConfirmed is returned when a tarball is unsigned and the
	// request did not carry the operator's explicit AllowUnsigned consent.
	ErrUnsignedNotConfirmed = errors.New("plugin is unsigned and the install was not confirmed")
)

// Uninstalled is the result of a successful Uninstall.
//
// RolesStripped names every role (system presets and custom roles
// alike) that still granted one of the plugin's permission keys and
// had those keys removed as part of the uninstall. The list exists so
// the admin is told what changed instead of finding a quieter role
// later — a role grant is not a reason to refuse an uninstall, but it
// is a reason to say so.
type Uninstalled struct {
	Slug          string   `json:"slug"`
	Version       string   `json:"version"`
	RolesStripped []string `json:"roles_stripped"`
	// ConfigPurged and CatalogsPurged report what PurgeData removed, so the
	// caller can say what went rather than only that something did. Zero on
	// an ordinary uninstall.
	ConfigPurged   int `json:"config_purged"`
	CatalogsPurged int `json:"catalogs_purged"`
}

// ManifestError wraps a manifest validation failure with the field
// path that failed.
type ManifestError struct {
	Field  string
	Reason string
}

func (e *ManifestError) Error() string {
	return fmt.Sprintf("manifest %s: %s", e.Field, e.Reason)
}

// Compile-time check: ensure Installer satisfies what the runtime
// expects at the package boundary.
var _ Installer = (*DefaultInstaller)(nil)

// Compile-time check: the production installer supports the safe upgrade path.
var _ Upgrader = (*DefaultInstaller)(nil)

// Compile-time check: Contributor must satisfy plugin.ContributionTracker.
var _ pluginruntime.ContributionTracker = (*contributorAdapter)(nil)

// Compile-time check: *Service implements Contributor. The five
// Record* + ClearAll methods on *Service match the Contributor
// interface signature-for-signature, but no compile-time check
// existed before — adding one prevents the runtime from breaking
// silently if a future refactor drops or renames a method.
var _ Contributor = (*Service)(nil)

// contributorAdapter wraps a Contributor so the loader can call it
// via its plugin.ContributionTracker interface.
type contributorAdapter struct {
	c Contributor
}

func (a contributorAdapter) RecordRoute(ctx context.Context, origin, method, path, requiresPermission string, isPublic bool) error {
	return a.c.RecordRoute(ctx, origin, method, path, requiresPermission, isPublic)
}

func (a contributorAdapter) RecordMenu(ctx context.Context, origin, parent, labelNS, labelKey, path, icon, permission string, priority int) error {
	return a.c.RecordMenu(ctx, origin, parent, labelNS, labelKey, path, icon, permission, priority)
}

func (a contributorAdapter) RecordTab(ctx context.Context, origin, pageID, key, labelNS, labelKey, icon, permission string, sortOrder int) error {
	return a.c.RecordTab(ctx, origin, pageID, key, labelNS, labelKey, icon, permission, sortOrder)
}

func (a contributorAdapter) RecordWidget(ctx context.Context, origin, key, title, category, defaultSize, settingsJSON string) error {
	return a.c.RecordWidget(ctx, origin, key, title, category, defaultSize, settingsJSON)
}

func (a contributorAdapter) RecordNotificationAdapter(ctx context.Context, origin, channelType, displayName string, supportsFiles, supportsRich, encryptedCredentials bool, recipient pluginruntime.NotificationRecipientDecl) error {
	return a.c.RecordNotificationAdapter(ctx, origin, channelType, displayName, supportsFiles, supportsRich, encryptedCredentials, recipient)
}

func (a contributorAdapter) RecordNode(ctx context.Context, origin, key, category, displayName, icon, configSchemaJSON, inputPortsJSON, outputPortsJSON, requiresPermission string) error {
	return a.c.RecordNode(ctx, origin, key, category, displayName, icon, configSchemaJSON, inputPortsJSON, outputPortsJSON, requiresPermission)
}

func (a contributorAdapter) RecordTheme(ctx context.Context, origin, id, name, description, cssPath string, swatchHue int, defaultVariant string, hasDark, hasLight bool) error {
	return a.c.RecordTheme(ctx, origin, id, name, description, cssPath, swatchHue, defaultVariant, hasDark, hasLight)
}

func (a contributorAdapter) RecordCatalog(ctx context.Context, origin, catalogID, kind, name, url string) error {
	return a.c.RecordCatalog(ctx, origin, catalogID, kind, name, url)
}

func (a contributorAdapter) RecordCLIRoot(ctx context.Context, origin, name, short, aliasesJSON string) error {
	return a.c.RecordCLIRoot(ctx, origin, name, short, aliasesJSON)
}

func (a contributorAdapter) RecordCLICommand(ctx context.Context, origin string, cmd pluginruntime.CLICommandRow) error {
	return a.c.RecordCLICommand(ctx, origin, cmd)
}

func (a contributorAdapter) DropCLI(ctx context.Context, origin string) error {
	return a.c.DropCLI(ctx, origin)
}

func (a contributorAdapter) ClearAll(ctx context.Context, origin string) error {
	return a.c.ClearAll(ctx, origin)
}

// NewContributorAdapter wraps a Contributor so it satisfies the plugin
// runtime's ContributionTracker interface. Exported so callers outside
// this package (specifically main.go) can construct one and pass it
// to pluginruntime.Boot. The wrapper is value-receiver — callers may
// pass either a pointer or a value.
func NewContributorAdapter(c Contributor) pluginruntime.ContributionTracker {
	return contributorAdapter{c: c}
}
