// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Force-link every exported top-level type of every Go subpackage under
// github.com/therealmcsparrow/mcharbor/core/plugin/* so each subpackage's
// per-package ABI hash in the host binary matches the plugin .so's.
//
// Why this file exists: the Go plugin loader refuses a .so whose
// per-package ABI hash differs from the host's. The hash is computed
// from each side's kept-symbol set for the shared package. The host
// linker is happy to drop *transitive* references through field types
// (a `Manifest.Permissions []types.PermissionDecl` field reaches the
// type by signature, but the linker does not retain the type descriptor
// unless something *uses* the type at runtime). The plugin's
// `host_link_keep.go` does use the type — via `reflect.TypeOf((*T)(nil)).Elem()`
// — so the plugin's reach set is non-empty. The host's is empty. The
// hashes diverge.
//
// This file is the host's mirror of the plugin's
// `host_link_keep.go`. It lives in `package main` so its package-level
// var initializers survive dead-code elimination and contribute to
// the host binary's per-package reach set.
//
// This file is hand-maintained (not generated). When the set of
// exported symbols in any subpackage changes, update both this file
// and the extractor at `cmd/extract-core-plugin-symbols/main.go`.
// Until then, the two must agree exactly.
package main

import (
	"reflect"

	"github.com/therealmcsparrow/mcharbor/core/plugin/contract"
	hostplugin "github.com/therealmcsparrow/mcharbor/core/plugin/host"
	"github.com/therealmcsparrow/mcharbor/core/plugin/services"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/ai"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/identity"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/logging"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/notification"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/snapshot"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/storage"
	"github.com/therealmcsparrow/mcharbor/core/plugin/types"
)

// --- github.com/therealmcsparrow/mcharbor/core/plugin/types (types) ---
// The 7 leaf data shapes. Their reach set MUST match the plugin's
// — the multi-package force-link extractor emits the same slice.
var types_forceLinkTypes = []reflect.Type{
	reflect.TypeOf((*types.PermissionDecl)(nil)).Elem(),
	reflect.TypeOf((*types.ABACRuleDecl)(nil)).Elem(),
	reflect.TypeOf((*types.ConfigFieldDecl)(nil)).Elem(),
	reflect.TypeOf((*types.SCIMSchema)(nil)).Elem(),
	reflect.TypeOf((*types.SCIMAttributeDecl)(nil)).Elem(),
	reflect.TypeOf((*types.WidgetDescriptor)(nil)).Elem(),
	reflect.TypeOf((*types.NodeDescriptor)(nil)).Elem(),
}

// --- github.com/therealmcsparrow/mcharbor/core/plugin/contract (types) ---
// Lifecycle + registration interfaces. HostContext is a struct, not
// an interface — force-link its descriptor too so plugin code that
// reads `host.Audit` etc. via HostContext fields gets a matching
// type descriptor in both binaries.
var contract_forceLinkTypes = []reflect.Type{
	reflect.TypeOf((*contract.Plugin)(nil)).Elem(),
	reflect.TypeOf((*contract.Manifest)(nil)).Elem(),
	reflect.TypeOf((*contract.AdapterContributor)(nil)).Elem(),
	reflect.TypeOf((*contract.AdapterRegistry)(nil)).Elem(),
	reflect.TypeOf((*contract.Registry)(nil)).Elem(),
	reflect.TypeOf((*contract.MenuRegistry)(nil)).Elem(),
	reflect.TypeOf((*contract.WidgetRegistry)(nil)).Elem(),
	reflect.TypeOf((*contract.NodeRegistry)(nil)).Elem(),
	reflect.TypeOf((*contract.AccessRegistry)(nil)).Elem(),
	reflect.TypeOf((*contract.SCIMHandler)(nil)).Elem(),
	reflect.TypeOf((*contract.HostContext)(nil)).Elem(),
	reflect.TypeOf((*contract.FormDecl)(nil)).Elem(),
	reflect.TypeOf((*contract.RouteDecl)(nil)).Elem(),
	reflect.TypeOf((*contract.Contributes)(nil)).Elem(),
	reflect.TypeOf((*contract.Capabilities)(nil)).Elem(),
	reflect.TypeOf((*contract.MenuItem)(nil)).Elem(),
}

// --- github.com/therealmcsparrow/mcharbor/core/plugin/services (types) ---
// Host services + handle types exposed to plugins via HostContext
// fields. AIRequest / AIMessage / AIResponse / AIUsage live here (not
// in specs/ai — those are LLM* in specs/ai).
var services_forceLinkTypes = []reflect.Type{
	reflect.TypeOf((*services.DBHandle)(nil)).Elem(),
	reflect.TypeOf((*services.Rows)(nil)).Elem(),
	reflect.TypeOf((*services.KVStore)(nil)).Elem(),
	reflect.TypeOf((*services.EncryptionHandle)(nil)).Elem(),
	reflect.TypeOf((*services.DockerPoolAccessor)(nil)).Elem(),
	reflect.TypeOf((*services.PoolHandle)(nil)).Elem(),
	reflect.TypeOf((*services.IdentityProvisioner)(nil)).Elem(),
	reflect.TypeOf((*services.SessionIssuer)(nil)).Elem(),
	reflect.TypeOf((*services.AIHandle)(nil)).Elem(),
	reflect.TypeOf((*services.AuditHandle)(nil)).Elem(),
	reflect.TypeOf((*services.RBACHandle)(nil)).Elem(),
	reflect.TypeOf((*services.SnapshotRPCHandle)(nil)).Elem(),
	reflect.TypeOf((*services.StorageHandle)(nil)).Elem(),
	reflect.TypeOf((*services.StorageLocationRef)(nil)).Elem(),
	reflect.TypeOf((*services.CommsHandle)(nil)).Elem(),
	reflect.TypeOf((*services.CommsChannelRef)(nil)).Elem(),
	reflect.TypeOf((*services.CommsMessage)(nil)).Elem(),
	reflect.TypeOf((*services.CommsFile)(nil)).Elem(),
	reflect.TypeOf((*services.ProvisionRequest)(nil)).Elem(),
	reflect.TypeOf((*services.ProvisionedUser)(nil)).Elem(),
	reflect.TypeOf((*services.AIRequest)(nil)).Elem(),
	reflect.TypeOf((*services.AIMessage)(nil)).Elem(),
	reflect.TypeOf((*services.AIResponse)(nil)).Elem(),
	reflect.TypeOf((*services.AIUsage)(nil)).Elem(),
	reflect.TypeOf((*services.AIConnection)(nil)).Elem(),
}

// --- github.com/therealmcsparrow/mcharbor/core/plugin/specs/storage ---
var storage_forceLinkTypes = []reflect.Type{
	reflect.TypeOf((*storage.StorageAdapterRegistry)(nil)).Elem(),
	reflect.TypeOf((*storage.StorageAdapterSpec)(nil)).Elem(),
	reflect.TypeOf((*storage.StorageAdapterClient)(nil)).Elem(),
	reflect.TypeOf((*storage.StorageAdapterCapabilities)(nil)).Elem(),
	reflect.TypeOf((*storage.StorageObject)(nil)).Elem(),
	reflect.TypeOf((*storage.TestStep)(nil)).Elem(),
	reflect.TypeOf((*storage.RicherStorageTester)(nil)).Elem(),
	reflect.TypeOf((*storage.StorageQuota)(nil)).Elem(),
	reflect.TypeOf((*storage.StorageQuotaReporter)(nil)).Elem(),
}

// --- github.com/therealmcsparrow/mcharbor/core/plugin/specs/notification ---
var notification_forceLinkTypes = []reflect.Type{
	reflect.TypeOf((*notification.NotificationAdapterRegistry)(nil)).Elem(),
	reflect.TypeOf((*notification.NotificationAdapterSpec)(nil)).Elem(),
	reflect.TypeOf((*notification.NotificationAdapterClient)(nil)).Elem(),
	reflect.TypeOf((*notification.NotificationAdapterCapabilities)(nil)).Elem(),
	reflect.TypeOf((*notification.NotificationMessage)(nil)).Elem(),
	reflect.TypeOf((*notification.NotificationAttachment)(nil)).Elem(),
}

// --- github.com/therealmcsparrow/mcharbor/core/plugin/specs/identity ---
var identity_forceLinkTypes = []reflect.Type{
	reflect.TypeOf((*identity.IdentityAdapterRegistry)(nil)).Elem(),
	reflect.TypeOf((*identity.IdentityAdapterSpec)(nil)).Elem(),
	reflect.TypeOf((*identity.IdentityAdapterCapabilities)(nil)).Elem(),
	reflect.TypeOf((*identity.IdentityAdapterClient)(nil)).Elem(),
	reflect.TypeOf((*identity.IdentityAuthRequest)(nil)).Elem(),
	reflect.TypeOf((*identity.IdentityAuthResult)(nil)).Elem(),
	reflect.TypeOf((*identity.IdentityCallbackRequest)(nil)).Elem(),
	reflect.TypeOf((*identity.IdentityCallbackResult)(nil)).Elem(),
	reflect.TypeOf((*identity.IdentityGroup)(nil)).Elem(),
}

// --- github.com/therealmcsparrow/mcharbor/core/plugin/specs/ai ---
var ai_forceLinkTypes = []reflect.Type{
	reflect.TypeOf((*ai.AIAdapterRegistry)(nil)).Elem(),
	reflect.TypeOf((*ai.AIAdapterSpec)(nil)).Elem(),
	reflect.TypeOf((*ai.AIAdapterCapabilities)(nil)).Elem(),
	reflect.TypeOf((*ai.AIAdapterClient)(nil)).Elem(),
	reflect.TypeOf((*ai.AITestResult)(nil)).Elem(),
	reflect.TypeOf((*ai.LLMRequest)(nil)).Elem(),
	reflect.TypeOf((*ai.LLMMessage)(nil)).Elem(),
	reflect.TypeOf((*ai.LLMResponse)(nil)).Elem(),
	reflect.TypeOf((*ai.LLMTool)(nil)).Elem(),
	reflect.TypeOf((*ai.LLMUsage)(nil)).Elem(),
	reflect.TypeOf((*ai.ModelDecl)(nil)).Elem(),
}

// --- github.com/therealmcsparrow/mcharbor/core/plugin/specs/snapshot ---
var snapshot_forceLinkTypes = []reflect.Type{
	reflect.TypeOf((*snapshot.SnapshotAdapterRegistry)(nil)).Elem(),
	reflect.TypeOf((*snapshot.SnapshotAdapterSpec)(nil)).Elem(),
	reflect.TypeOf((*snapshot.SnapshotAdapterCapabilities)(nil)).Elem(),
	reflect.TypeOf((*snapshot.SnapshotAdapterClient)(nil)).Elem(),
	reflect.TypeOf((*snapshot.SnapshotCreateRequest)(nil)).Elem(),
	reflect.TypeOf((*snapshot.SnapshotRef)(nil)).Elem(),
	reflect.TypeOf((*snapshot.SnapshotCapability)(nil)).Elem(),
	reflect.TypeOf((*snapshot.SnapshotTarget)(nil)).Elem(),
}

// --- github.com/therealmcsparrow/mcharbor/core/plugin/specs/logging ---
var logging_forceLinkTypes = []reflect.Type{
	reflect.TypeOf((*logging.LogForwarderAdapterRegistry)(nil)).Elem(),
	reflect.TypeOf((*logging.LogForwarderAdapterSpec)(nil)).Elem(),
	reflect.TypeOf((*logging.LogForwarderAdapterCapabilities)(nil)).Elem(),
	reflect.TypeOf((*logging.LogForwarderAdapterClient)(nil)).Elem(),
	reflect.TypeOf((*logging.ForwardResult)(nil)).Elem(),
	reflect.TypeOf((*logging.LogDelivery)(nil)).Elem(),
}

// --- github.com/therealmcsparrow/mcharbor/core/plugin/host (types) ---
// Host-only types. Plugins don't import this package, so its ABI hash
// is host-only and doesn't affect plugin.Open. We still force-link so
// the host binary's reach set is stable across rebuilds.
var host_forceLinkTypes = []reflect.Type{
	reflect.TypeOf((*hostplugin.StorageAdapterSource)(nil)).Elem(),
	reflect.TypeOf((*hostplugin.NotificationAdapterSource)(nil)).Elem(),
	reflect.TypeOf((*hostplugin.SnapshotAdapterSource)(nil)).Elem(),
	reflect.TypeOf((*hostplugin.LogForwarderAdapterSource)(nil)).Elem(),
	reflect.TypeOf((*hostplugin.LabelRegistry)(nil)).Elem(),
	reflect.TypeOf((*hostplugin.PermissionLabel)(nil)).Elem(),
	reflect.TypeOf((*hostplugin.SchemaReportT)(nil)).Elem(),
	reflect.TypeOf((*hostplugin.ManifestError)(nil)).Elem(),
	reflect.TypeOf((*hostplugin.ManifestSchemaError)(nil)).Elem(),
}

// --- cross-reference ---
// forceLinkAll references every per-package types slice so the linker
// has an observable read of each entry. The slice is never read at
// runtime; only the linker sees it.
var forceLinkAll = [][]reflect.Type{
	types_forceLinkTypes,
	contract_forceLinkTypes,
	services_forceLinkTypes,
	storage_forceLinkTypes,
	notification_forceLinkTypes,
	identity_forceLinkTypes,
	ai_forceLinkTypes,
	snapshot_forceLinkTypes,
	logging_forceLinkTypes,
	host_forceLinkTypes,
}

func init() {
	// Touch forceLinkAll so the linker has an observable read of the
	// cross-reference. The slice itself is never iterated at runtime
	// — this is purely a linkage marker.
	_ = forceLinkAll
}
