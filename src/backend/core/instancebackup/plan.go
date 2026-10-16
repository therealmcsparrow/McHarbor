// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package instancebackup

import (
	"context"
	"crypto/hmac"
	"database/sql"
	"fmt"
	"io"
	"sort"
	"strings"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

// Restore modes. A migration restore is a full restore onto a different
// database backend or host; the algorithm is the same and only the wording differs.
const (
	ModeFull      = "full"
	ModeSelective = "selective"
	ModeMigration = "migration"
)

// Conflict handling for a selective restore.
const (
	ConflictReplace = "replace"
	ConflictKeep    = "keep"
)

// Restore categories. A selective restore picks from these; audit is never
// implied by any other and needs its own permission.
const (
	CategoryAccess       = "access"
	CategoryEnvironments = "environments"
	CategoryAutomation   = "automation"
	CategoryStacks       = "stacks"
	CategorySecrets      = "secrets"
	CategorySettings     = "settings"
	CategoryPlugins      = "plugins"
	CategoryAudit        = "audit"
	CategoryOther        = "other"
)

// Categories lists every category in the order the plan screen shows them.
var Categories = []string{
	CategoryAccess, CategoryEnvironments, CategoryAutomation, CategoryStacks, CategorySecrets,
	CategorySettings, CategoryPlugins, CategoryAudit, CategoryOther,
}

// InstallerOwned tables are written by the plugin installer and the plugin
// runtime. They are never copied from an export: plugins are reinstalled
// through the installer after the restart, which rebuilds them.
var InstallerOwned = map[string]bool{
	"appstore_plugins": true, "plugin_permissions": true, "plugin_widgets": true, "plugin_themes": true,
	"plugin_tabs": true, "plugin_routes": true, "plugin_notification_adapters": true, "plugin_nodes": true,
	"plugin_menu": true, "plugin_topbar": true, "plugin_context_menus": true,
}

var categoryTables = map[string]string{
	"audit_logs": CategoryAudit, "audit_chain_tombstones": CategoryAudit,
	"users": CategoryAccess, "user_roles": CategoryAccess, "user_permissions": CategoryAccess,
	"user_labels": CategoryAccess, "user_mfa": CategoryAccess, "password_history": CategoryAccess,
	"roles": CategoryAccess, "role_presets": CategoryAccess, "groups": CategoryAccess,
	"group_members": CategoryAccess, "group_roles": CategoryAccess, "permission_catalog": CategoryAccess,
	"identity_providers": CategoryAccess, "auth_settings": CategoryAccess, "elevation_requests": CategoryAccess, "permission_elevation": CategoryAccess,
	"elevation_history": CategoryAccess,
	"environments":      CategoryEnvironments, "environment_notifications": CategoryEnvironments,
	"registries": CategoryEnvironments, "storage_locations": CategoryEnvironments,
	"workflows": CategoryAutomation, "workflow_kv": CategoryAutomation, "workflow_link_messages": CategoryAutomation,
	"workflow_runs": CategoryAutomation, "schedules": CategoryAutomation, "schedule_executions": CategoryAutomation,
	"webhooks": CategoryAutomation, "webhook_deliveries": CategoryAutomation, "alerts": CategoryAutomation,
	"alert_destinations": CategoryAutomation, "alert_events": CategoryAutomation, "alert_states": CategoryAutomation,
	"desired_states": CategoryAutomation, "reconciliation_logs": CategoryAutomation, "blueprints": CategoryAutomation,
	"reconcile_proposals": CategoryAutomation, "reconciler_settings": CategoryAutomation, "desired_state_sources": CategoryAutomation,
	"incidents": CategoryAutomation, "incident_settings": CategoryAutomation, "behaviour_findings": CategoryAutomation,
	"incident_links": CategoryAutomation, "incident_outbox": CategoryAutomation,
	"git_repos": CategoryAutomation, "git_deployments": CategoryAutomation, "update_policies": CategoryAutomation,
	"stacks": CategoryStacks, "stack_sources": CategoryStacks, "stack_environment_variables": CategoryStacks,
	"stack_webhooks": CategoryStacks, "container_stack_links": CategoryStacks,
	"cluster_state": CategorySecrets, "secrets": CategorySecrets, "secret_versions": CategorySecrets, "secret_references": CategorySecrets,
	"settings": CategorySettings, "settings_catalogs": CategorySettings, "table_views": CategorySettings,
	"communication_channels": CategorySettings, "notification_channels": CategorySettings,
	"email_servers": CategorySettings, "audit_log_forwarders": CategorySettings,
	"audit_retention_presets": CategorySettings, "instance_backup_schedules": CategorySettings,
	"plugin_config": CategoryPlugins, "plugin_migrations": CategoryPlugins, "plugin_catalogs": CategoryPlugins,
}

var categoryPrefixes = []struct{ prefix, category string }{
	{"api_key", CategoryAccess}, {"rbac_", CategoryAccess}, {"scim_", CategoryAccess},
	{"gitops_", CategoryAutomation}, {"container_backup_", CategoryStacks}, {"ai_", CategorySettings},
	{"license", CategorySettings}, {"plugin_", CategoryPlugins},
}

// CategoryOf names the category a table is restored with.
func CategoryOf(table string) string {
	t := strings.ToLower(table)
	if c, ok := categoryTables[t]; ok {
		return c
	}
	for _, p := range categoryPrefixes {
		if strings.HasPrefix(t, p.prefix) {
			return p.category
		}
	}
	return CategoryOther
}

// Target is the instance a restore writes to.
type Target struct {
	DB            *sql.DB
	Driver        coredb.Driver
	DataDir       string
	EncryptionKey []byte
	Secret        string
	// KeyPinned and SecretPinned are true when the environment supplies the
	// key or secret. A restore cannot replace a value the environment re-applies
	// at every start, so it is refused rather than left to fail after the restart.
	KeyPinned    bool
	SecretPinned bool
}

// RestoreOptions are the operator's choices for one restore.
type RestoreOptions struct {
	Secret       Secret
	Mode         string
	Categories   []string
	IncludeAudit bool
	Conflict     string
	Actor        string
	Via          string
	Progress     func(Progress)
	// Emit records the intent outside the database before anything is
	// written. A restore that replaces audit history refuses to run without it.
	Emit func(ctx context.Context, in Intent) error
}

// TablePlan is one table's before and after.
type TablePlan struct {
	Name         string `json:"name"`
	TargetRows   int64  `json:"targetRows"`
	ArtifactRows int64  `json:"artifactRows"`
	Exists       bool   `json:"exists"`
}

// CategoryPlan groups the tables one category overwrites.
type CategoryPlan struct {
	Name     string      `json:"name"`
	Selected bool        `json:"selected"`
	Tables   []TablePlan `json:"tables"`
}

// Plan is what a restore will do, computed before anything is written.
type Plan struct {
	Manifest       Manifest       `json:"manifest"`
	Mode           string         `json:"mode"`
	Categories     []CategoryPlan `json:"categories"`
	RestoresKeys   bool           `json:"restoresKeys"`
	RestoresSecret bool           `json:"restoresSecret"`
	RestoresFiles  bool           `json:"restoresFiles"`
	SameKey        bool           `json:"sameKey"`
	Plugins        []PluginRecord `json:"plugins"`
	Refusals       []string       `json:"refusals"`
	Warnings       []string       `json:"warnings"`
}

// Refusal and warning reasons, stable strings the interface translates.
const (
	RefuseNewerSchema     = "newer_schema"
	RefuseKeyPinned       = "key_pinned"
	RefuseSecretPinned    = "secret_pinned"
	RefuseNothingSelected = "nothing_selected"
	RefuseBadMode         = "bad_mode"
	RefusePostgresCascade = "postgres_cascade"
	WarnDifferentKey      = "different_key"
	WarnMetricsExcluded   = "metrics_excluded"
	WarnSessionsEnd       = "sessions_end"
)

// selected reports whether a table is restored under opts.
func selected(table string, opts RestoreOptions) bool {
	lower := strings.ToLower(table)
	if InstallerOwned[lower] || NeverExported[lower] {
		return false
	}
	c := CategoryOf(table)
	if c == CategoryAudit {
		return opts.IncludeAudit
	}
	if opts.Mode != ModeSelective {
		return true
	}
	for _, want := range opts.Categories {
		if want == c {
			return true
		}
	}
	return false
}

// PlanRestore reads the whole artifact, which proves it complete, and states
// per category what the target holds and what the artifact would put there.
func PlanRestore(ctx context.Context, t Target, r io.Reader, opts RestoreOptions) (Plan, Blobs, error) {
	blobs := Blobs{}
	a, err := Open(r, opts.Secret)
	if err != nil {
		return Plan{}, blobs, &ArtifactError{Err: err}
	}
	defer func() { logCleanup("a", a.Close()) }()
	for {
		e, err := a.Next()
		if err != nil {
			return Plan{}, blobs, &ArtifactError{Err: err}
		}
		if e.Kind == EntryEnd {
			break
		}
		if e.Kind == EntryBlob && e.Blob == BlobKey {
			b, err := io.ReadAll(a.Blob())
			if err != nil {
				return Plan{}, blobs, &ArtifactError{Err: fmt.Errorf("reading %s: %w", e.Name, err)}
			}
			blobs[e.Name] = b
		}
	}
	p, err := buildPlan(ctx, t, a.Manifest, blobs, opts)
	return p, blobs, err
}

// ArtifactError is a problem with the file or the key the operator supplied:
// wrong key, damaged, incomplete or altered. Its message is written for the
// operator and is safe to show them, unlike a database failure.
type ArtifactError struct{ Err error }

func (e *ArtifactError) Error() string { return e.Err.Error() }

// Unwrap exposes the cause, so errors.Is(err, ErrWrongKey) still holds.
func (e *ArtifactError) Unwrap() error { return e.Err }

// Blobs holds the key material read from an artifact.
type Blobs map[string][]byte

func buildPlan(ctx context.Context, t Target, m Manifest, blobs Blobs, opts RestoreOptions) (Plan, error) {
	p := Plan{Manifest: m, Mode: opts.Mode, Plugins: m.Plugins, Refusals: []string{}, Warnings: []string{}}
	switch opts.Mode {
	case ModeFull, ModeMigration, ModeSelective:
	default:
		p.Refusals = append(p.Refusals, RefuseBadMode)
		return p, nil
	}
	known, err := coredb.MigrationNames()
	if err != nil {
		return p, fmt.Errorf("listing migrations: %w", err)
	}
	have := make(map[string]bool, len(known))
	for _, n := range known {
		have[n] = true
	}
	for _, n := range m.Schema {
		if !have[n] {
			p.Refusals = append(p.Refusals, RefuseNewerSchema)
			break
		}
	}

	full := opts.Mode != ModeSelective
	p.SameKey = hmac.Equal(blobs[KeyEncryption], t.EncryptionKey)
	p.RestoresKeys = full && !p.SameKey
	p.RestoresSecret = opts.IncludeAudit && string(blobs[KeySecret]) != t.Secret
	p.RestoresFiles = full
	if p.RestoresKeys && t.KeyPinned {
		p.Refusals = append(p.Refusals, RefuseKeyPinned)
	}
	if p.RestoresSecret && t.SecretPinned {
		p.Refusals = append(p.Refusals, RefuseSecretPinned)
	}
	if !full && !p.SameKey {
		p.Warnings = append(p.Warnings, WarnDifferentKey)
	}
	if !m.IncludesMetrics {
		p.Warnings = append(p.Warnings, WarnMetricsExcluded)
	}
	if full {
		p.Warnings = append(p.Warnings, WarnSessionsEnd)
	}

	targetTables, err := coredb.ListTables(ctx, t.DB, t.Driver)
	if err != nil {
		return p, fmt.Errorf("listing target tables: %w", err)
	}
	exists := make(map[string]bool, len(targetTables))
	for _, name := range targetTables {
		exists[strings.ToLower(name)] = true
	}
	byCat := map[string]*CategoryPlan{}
	anySelected := false
	// bounded by the tables listed in the artifact manifest.
	for _, tbl := range m.Tables {
		lower := strings.ToLower(tbl.Name)
		if InstallerOwned[lower] {
			continue
		}
		c := CategoryOf(tbl.Name)
		cp, ok := byCat[c]
		if !ok {
			cp = &CategoryPlan{Name: c, Selected: selected(tbl.Name, opts)}
			byCat[c] = cp
		}
		tp := TablePlan{Name: tbl.Name, ArtifactRows: tbl.Rows, Exists: exists[lower]}
		if tp.Exists {
			if err := t.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+coredb.QuoteIdent(t.Driver, tbl.Name)).Scan(&tp.TargetRows); err != nil {
				return p, fmt.Errorf("counting %s: %w", tbl.Name, err)
			}
		}
		cp.Tables = append(cp.Tables, tp)
		anySelected = anySelected || cp.Selected
	}
	if !anySelected {
		p.Refusals = append(p.Refusals, RefuseNothingSelected)
	}
	for _, c := range Categories {
		if cp, ok := byCat[c]; ok {
			sort.Slice(cp.Tables, func(i, j int) bool { return cp.Tables[i].Name < cp.Tables[j].Name })
			p.Categories = append(p.Categories, *cp)
		}
	}
	return p, nil
}
