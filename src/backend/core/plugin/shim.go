// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package plugin is the backward-compatibility shim for the
// pre-shim-refactor `core/plugin` package.
//
// After the 2026-08-14 reach-set refactor, every type and interface
// previously declared here moved into one of:
//
//   - core/plugin/contract       (Plugin, Manifest, Registry,
//     MenuRegistry, WidgetRegistry,
//     NodeRegistry, AccessRegistry,
//     AdapterContributor, AdapterRegistry,
//     HostContext)
//   - core/plugin/services       (DBHandle, EncryptionHandle,
//     PoolHandle, DockerPoolAccessor,
//     NewDockerPoolAccessor, I18nHandle,
//     AuditHandle, RBACHandle,
//     IdentityProvisioner, SessionIssuer,
//     AIHandle, SnapshotRPCHandle and their
//     request/result shapes)
//   - core/plugin/types          (PermissionDecl, ABACRuleDecl,
//     ConfigFieldDecl, SCIMSchema,
//     SCIMAttributeDecl, WidgetDescriptor,
//     NodeDescriptor)
//   - core/plugin/specs/storage          (storage family)
//   - core/plugin/specs/notification     (notification family)
//   - core/plugin/specs/identity         (identity family)
//   - core/plugin/specs/ai               (ai family)
//   - core/plugin/specs/snapshot         (snapshot family)
//   - core/plugin/specs/logging          (logging family)
//   - core/plugin/specs/scanner          (scanner family)
//   - core/plugin/host           (ValidateManifestSchema,
//     NewLabelRegistry, LabelRegistry,
//     PermissionLabel, TrustedKeys,
//     *AdapterSource, etc.)
//
// This file re-exports every previously-public symbol as a type
// alias or a re-assigned variable so existing callers that import
// `github.com/therealmcsparrow/mcharbor/core/plugin` keep compiling
// without changes. New code SHOULD import from the new packages
// directly (this shim will be removed in a future release).
//
// Why the shim exists: 18 plugins, the host's plugin_runtime loader,
// and a long tail of host modules all reference symbols like
// `plugin.Plugin`, `plugin.StorageAdapterSpec`, `plugin.NewLabelRegistry`.
// Updating every site in one sweep is a large mechanical change; the
// shim lets the refactor ship as an atomic "move, then sweep" — first
// move the symbols (this file), then incrementally remove the shim
// references in follow-up PRs.
package plugin

import (
	"context"
	"github.com/therealmcsparrow/mcharbor/core/plugin/contract"
	"github.com/therealmcsparrow/mcharbor/core/plugin/host"
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

// --- Lifecycle interfaces (contract) ----------------------------

// Plugin re-exports contract.Plugin so plugin code depends on this package alone.
type Plugin = contract.Plugin

// Registry re-exports contract.Registry so plugin code depends on this package alone.
type Registry = contract.Registry

// MenuRegistry re-exports contract.MenuRegistry so plugin code depends on this package alone.
type MenuRegistry = contract.MenuRegistry

// WidgetRegistry re-exports contract.WidgetRegistry so plugin code depends on this package alone.
type WidgetRegistry = contract.WidgetRegistry

// NodeRegistry re-exports contract.NodeRegistry so plugin code depends on this package alone.
type NodeRegistry = contract.NodeRegistry

// AccessRegistry re-exports contract.AccessRegistry so plugin code depends on this package alone.
type AccessRegistry = contract.AccessRegistry

// AdapterContributor re-exports contract.AdapterContributor so plugin code depends on this package alone.
type AdapterContributor = contract.AdapterContributor

// AdapterRegistry re-exports contract.AdapterRegistry so plugin code depends on this package alone.
type AdapterRegistry = contract.AdapterRegistry

// SCIMHandler re-exports contract.SCIMHandler so plugin code depends on this package alone.
type SCIMHandler = contract.SCIMHandler

// --- Manifest + data shapes (contract, types) --------------------

// Manifest re-exports contract.Manifest so plugin code depends on this package alone.
type Manifest = contract.Manifest

// Capabilities re-exports contract.Capabilities so plugin code depends on this package alone.
type Capabilities = contract.Capabilities

// Contributes re-exports contract.Contributes so plugin code depends on this package alone.
type Contributes = contract.Contributes

// FormDecl re-exports contract.FormDecl so plugin code depends on this package alone.
type FormDecl = contract.FormDecl

// RouteDecl re-exports contract.RouteDecl so plugin code depends on this package alone.
type RouteDecl = contract.RouteDecl

// MenuItem re-exports contract.MenuItem so plugin code depends on this package alone.
type MenuItem = contract.MenuItem

// TabItem re-exports contract.TabItem so plugin code depends on this package alone.
type TabItem = contract.TabItem

// TabRegistry re-exports contract.TabRegistry so plugin code depends on this package alone.
type TabRegistry = contract.TabRegistry

// TabProvider re-exports contract.TabProvider so plugin code depends on this package alone.
type TabProvider = contract.TabProvider

// CLIProvider re-exports contract.CLIProvider; the whole CLI contract is aliased here so a
// plugin keeps writing `plugin.CLICommand` rather than importing the contract package.
type CLIProvider = contract.CLIProvider

// CLIRegistry re-exports contract.CLIRegistry so plugin code depends on this package alone.
type CLIRegistry = contract.CLIRegistry

// CLIRoot re-exports contract.CLIRoot so plugin code depends on this package alone.
type CLIRoot = contract.CLIRoot

// CLICommand re-exports contract.CLICommand so plugin code depends on this package alone.
type CLICommand = contract.CLICommand

// CLIFlag re-exports contract.CLIFlag so plugin code depends on this package alone.
type CLIFlag = contract.CLIFlag

// CLIColumn re-exports contract.CLIColumn so plugin code depends on this package alone.
type CLIColumn = contract.CLIColumn

// CLIKind re-exports contract.CLIKind so plugin code depends on this package alone.
type CLIKind = contract.CLIKind

// CLIBody re-exports contract.CLIBody so plugin code depends on this package alone.
type CLIBody = contract.CLIBody

// CLIRisk re-exports contract.CLIRisk so plugin code depends on this package alone.
type CLIRisk = contract.CLIRisk

// CLIFlagType re-exports contract.CLIFlagType so plugin code depends on this package alone.
type CLIFlagType = contract.CLIFlagType

// CLITarget re-exports contract.CLITarget so plugin code depends on this package alone.
type CLITarget = contract.CLITarget

// The CLI contract's enumerations, re-exported so a command table reads as
// `plugin.CLIList` beside the types it uses.
const (
	CLIList     = contract.CLIList
	CLIGet      = contract.CLIGet
	CLIAction   = contract.CLIAction
	CLIText     = contract.CLIText
	CLIUpload   = contract.CLIUpload
	CLIDownload = contract.CLIDownload
	CLIAttach   = contract.CLIAttach

	BodyNone  = contract.BodyNone
	BodyFlags = contract.BodyFlags
	BodyFile  = contract.BodyFile
	BodyBoth  = contract.BodyBoth

	RiskSafe         = contract.RiskSafe
	RiskDisruptive   = contract.RiskDisruptive
	RiskIrreversible = contract.RiskIrreversible

	FlagString      = contract.FlagString
	FlagInt         = contract.FlagInt
	FlagBool        = contract.FlagBool
	FlagDuration    = contract.FlagDuration
	FlagStringSlice = contract.FlagStringSlice
	FlagFile        = contract.FlagFile

	TargetQuery = contract.TargetQuery
	TargetBody  = contract.TargetBody
	TargetPath  = contract.TargetPath

	CLISpecVersion = contract.CLISpecVersion
)

// HostContext is the slice of McHarbor the host hands to a plugin
// during Init(). The shim re-exports it as a type alias so existing
// callers can keep referencing plugin.HostContext.
type HostContext = contract.HostContext

// --- Leaf data shapes (types) ------------------------------------

// PermissionDecl re-exports types.PermissionDecl so plugin code depends on this package alone.
type PermissionDecl = types.PermissionDecl

// ABACRuleDecl re-exports types.ABACRuleDecl so plugin code depends on this package alone.
type ABACRuleDecl = types.ABACRuleDecl

// ConfigFieldDecl re-exports types.ConfigFieldDecl so plugin code depends on this package alone.
type ConfigFieldDecl = types.ConfigFieldDecl

// OAuthDefaults re-exports types.OAuthDefaults so plugin code depends on this package alone.
type OAuthDefaults = types.OAuthDefaults

// SCIMSchema re-exports types.SCIMSchema so plugin code depends on this package alone.
type SCIMSchema = types.SCIMSchema

// SCIMAttributeDecl re-exports types.SCIMAttributeDecl so plugin code depends on this package alone.
type SCIMAttributeDecl = types.SCIMAttributeDecl

// WidgetDescriptor re-exports types.WidgetDescriptor so plugin code depends on this package alone.
type WidgetDescriptor = types.WidgetDescriptor

// NodeDescriptor re-exports types.NodeDescriptor so plugin code depends on this package alone.
type NodeDescriptor = types.NodeDescriptor

// NodeExecInput re-exports types.NodeExecInput so plugin code depends on this package alone.
type NodeExecInput = types.NodeExecInput

// NodeExecOutput re-exports types.NodeExecOutput so plugin code depends on this package alone.
type NodeExecOutput = types.NodeExecOutput

// NodeExecFunc re-exports types.NodeExecFunc so plugin code depends on this package alone.
type NodeExecFunc = types.NodeExecFunc

// WorkflowHandle re-exports services.WorkflowHandle so plugin code depends on this package alone.
type WorkflowHandle = services.WorkflowHandle

// WorkflowTriggerBinding re-exports services.WorkflowTriggerBinding so plugin code depends on this package alone.
type WorkflowTriggerBinding = services.WorkflowTriggerBinding

// WorkflowNodeExecutorSource is the slice of the plugin runtime the
// workflow engine narrows `router.AppDeps.PluginAdapters` to when a
// canvas node's action is `plugin:<slug>:<key>`. Implemented by
// core/pluginruntime.Runtime; consumed through
// internal/bootstrap so modules/workflows never imports the runtime.
type WorkflowNodeExecutorSource interface {
	// HasWorkflowNode reports whether a loaded plugin registered an
	// executor under the full `plugin:<slug>:<key>` action key.
	HasWorkflowNode(actionKey string) bool
	// ExecuteWorkflowNode runs that executor. It returns an error
	// when no plugin owns the key or the executor itself fails.
	ExecuteWorkflowNode(ctx context.Context, actionKey string, in NodeExecInput) (NodeExecOutput, error)
}

// --- Host service handles (services) -----------------------------

// DBHandle re-exports services.DBHandle so plugin code depends on this package alone.
type DBHandle = services.DBHandle

// Rows re-exports services.Rows so plugin code depends on this package alone.
type Rows = services.Rows

// KVStore re-exports services.KVStore so plugin code depends on this package alone.
type KVStore = services.KVStore

// EncryptionHandle re-exports services.EncryptionHandle so plugin code depends on this package alone.
type EncryptionHandle = services.EncryptionHandle

// PoolHandle re-exports services.PoolHandle so plugin code depends on this package alone.
type PoolHandle = services.PoolHandle

// DockerPoolAccessor re-exports services.DockerPoolAccessor so plugin code depends on this package alone.
type DockerPoolAccessor = services.DockerPoolAccessor

// I18nHandle re-exports services.I18nHandle so plugin code depends on this package alone.
type I18nHandle = services.I18nHandle

// AuditHandle re-exports services.AuditHandle so plugin code depends on this package alone.
type AuditHandle = services.AuditHandle

// AuditActorHandle re-exports services.AuditActorHandle so plugin code depends on this package alone.
type AuditActorHandle = services.AuditActorHandle

// AuditEvent re-exports services.AuditEvent so plugin code depends on this package alone.
type AuditEvent = services.AuditEvent

// EnvironmentHandle re-exports services.EnvironmentHandle so plugin code depends on this package alone.
type EnvironmentHandle = services.EnvironmentHandle

// Slugify and SlugifyOr turn a name into one path segment every storage
// destination accepts. Re-exported so a plugin naming an object after a
// machine produces the same segment the host produces for a container.
var (
	Slugify   = services.Slugify
	SlugifyOr = services.SlugifyOr
)

// BackupLogHandle re-exports services.BackupLogHandle so plugin code depends on this package alone.
type BackupLogHandle = services.BackupLogHandle

// BackupLogEntry re-exports services.BackupLogEntry so plugin code depends on this package alone.
type BackupLogEntry = services.BackupLogEntry

// LifecycleHandle re-exports services.LifecycleHandle so plugin code depends on this package alone.
type LifecycleHandle = services.LifecycleHandle

// LifecycleEvent re-exports services.LifecycleEvent so plugin code depends on this package alone.
type LifecycleEvent = services.LifecycleEvent

// RBACHandle re-exports services.RBACHandle so plugin code depends on this package alone.
type RBACHandle = services.RBACHandle

// IdentityProvisioner re-exports services.IdentityProvisioner so plugin code depends on this package alone.
type IdentityProvisioner = services.IdentityProvisioner

// ProvisionRequest re-exports services.ProvisionRequest so plugin code depends on this package alone.
type ProvisionRequest = services.ProvisionRequest

// ProvisionedUser re-exports services.ProvisionedUser so plugin code depends on this package alone.
type ProvisionedUser = services.ProvisionedUser

// SessionIssuer re-exports services.SessionIssuer so plugin code depends on this package alone.
type SessionIssuer = services.SessionIssuer

// AIHandle re-exports services.AIHandle so plugin code depends on this package alone.
type AIHandle = services.AIHandle

// AIRequest re-exports services.AIRequest so plugin code depends on this package alone.
type AIRequest = services.AIRequest

// AIMessage re-exports services.AIMessage so plugin code depends on this package alone.
type AIMessage = services.AIMessage

// AIResponse re-exports services.AIResponse so plugin code depends on this package alone.
type AIResponse = services.AIResponse

// AIUsage re-exports services.AIUsage so plugin code depends on this package alone.
type AIUsage = services.AIUsage

// AIConnection re-exports services.AIConnection so plugin code depends on this package alone.
type AIConnection = services.AIConnection

// SnapshotRPCHandle re-exports services.SnapshotRPCHandle so plugin code depends on this package alone.
type SnapshotRPCHandle = services.SnapshotRPCHandle

// StorageHandle re-exports services.StorageHandle so plugin code depends on this package alone.
type StorageHandle = services.StorageHandle

// StorageLocationRef re-exports services.StorageLocationRef so plugin code depends on this package alone.
type StorageLocationRef = services.StorageLocationRef

// StorageStreamHandle re-exports services.StorageStreamHandle so plugin code depends on this package alone.
type StorageStreamHandle = services.StorageStreamHandle

// StorageStreamRequest re-exports services.StorageStreamRequest so plugin code depends on this package alone.
type StorageStreamRequest = services.StorageStreamRequest

// StorageStreamResult re-exports services.StorageStreamResult so plugin code depends on this package alone.
type StorageStreamResult = services.StorageStreamResult

// StorageDownloadRequest re-exports services.StorageDownloadRequest so plugin code depends on this package alone.
type StorageDownloadRequest = services.StorageDownloadRequest

// CommsHandle re-exports services.CommsHandle so plugin code depends on this package alone.
type CommsHandle = services.CommsHandle

// CommsChannelRef re-exports services.CommsChannelRef so plugin code depends on this package alone.
type CommsChannelRef = services.CommsChannelRef

// CommsMessage re-exports services.CommsMessage so plugin code depends on this package alone.
type CommsMessage = services.CommsMessage

// CommsRecipientOverride re-exports services.CommsRecipientOverride so plugin code depends on this package alone.
type CommsRecipientOverride = services.CommsRecipientOverride

// CommsFile re-exports services.CommsFile so plugin code depends on this package alone.
type CommsFile = services.CommsFile

// CatalogHandle re-exports services.CatalogHandle so plugin code depends on this package alone.
type CatalogHandle = services.CatalogHandle

// CatalogSourceRef re-exports services.CatalogSourceRef so plugin code depends on this package alone.
type CatalogSourceRef = services.CatalogSourceRef

// NewDockerPoolAccessor is the constructor the host's loader calls
// when assembling each plugin's HostContext. It lives in the
// `services` package (alongside DockerPoolAccessor) so the
// unexported `get`/`agent` fields can be set from inside the same
// package. The shim re-exports it as a value so existing callers
// can keep referencing plugin.NewDockerPoolAccessor.
var NewDockerPoolAccessor = services.NewDockerPoolAccessor

// --- Storage family (specs/storage) ------------------------------

// BackupAdapterRegistry re-exports backup.BackupAdapterRegistry so plugin code depends on this package alone.
type BackupAdapterRegistry = backup.BackupAdapterRegistry

// BackupProviderSpec re-exports backup.BackupProviderSpec so plugin code depends on this package alone.
type BackupProviderSpec = backup.BackupProviderSpec

// BackupProviderClient re-exports backup.BackupProviderClient so plugin code depends on this package alone.
type BackupProviderClient = backup.BackupProviderClient

// BackupProviderCapabilities re-exports backup.BackupProviderCapabilities so plugin code depends on this package alone.
type BackupProviderCapabilities = backup.BackupProviderCapabilities

// BackupRunQuery re-exports backup.RunQuery so plugin code depends on this package alone.
type BackupRunQuery = backup.RunQuery

// BackupRunSummary re-exports backup.RunSummary so plugin code depends on this package alone.
type BackupRunSummary = backup.RunSummary

// BackupPlanSummary re-exports backup.PlanSummary so plugin code depends on this package alone.
type BackupPlanSummary = backup.PlanSummary

// ErrBackupUnsupported is what a provider returns when asked for something
// it does not do — a provider with no schedules is not a broken one.
var ErrBackupUnsupported = backup.ErrUnsupported

// StorageAdapterRegistry re-exports storage.StorageAdapterRegistry so plugin code depends on this package alone.
type StorageAdapterRegistry = storage.StorageAdapterRegistry

// StorageAdapterSpec re-exports storage.StorageAdapterSpec so plugin code depends on this package alone.
type StorageAdapterSpec = storage.StorageAdapterSpec

// StorageAdapterClient re-exports storage.StorageAdapterClient so plugin code depends on this package alone.
type StorageAdapterClient = storage.StorageAdapterClient

// StorageAdapterCapabilities re-exports storage.StorageAdapterCapabilities so plugin code depends on this package alone.
type StorageAdapterCapabilities = storage.StorageAdapterCapabilities

// StorageObject re-exports storage.StorageObject so plugin code depends on this package alone.
type StorageObject = storage.StorageObject

// TestStep re-exports storage.TestStep so plugin code depends on this package alone.
type TestStep = storage.TestStep

// RicherStorageTester re-exports storage.RicherStorageTester so plugin code depends on this package alone.
type RicherStorageTester = storage.RicherStorageTester

// StorageQuota re-exports storage.StorageQuota so plugin code depends on this package alone.
type StorageQuota = storage.StorageQuota

// StorageQuotaReporter re-exports storage.StorageQuotaReporter so plugin code depends on this package alone.
type StorageQuotaReporter = storage.StorageQuotaReporter

// --- Notification family (specs/notification) -------------------

// NotificationAdapterRegistry re-exports notification.NotificationAdapterRegistry so plugin code depends on this package alone.
type NotificationAdapterRegistry = notification.NotificationAdapterRegistry

// NotificationAdapterSpec re-exports notification.NotificationAdapterSpec so plugin code depends on this package alone.
type NotificationAdapterSpec = notification.NotificationAdapterSpec

// NotificationAdapterClient re-exports notification.NotificationAdapterClient so plugin code depends on this package alone.
type NotificationAdapterClient = notification.NotificationAdapterClient

// NotificationAdapterCapabilities re-exports notification.NotificationAdapterCapabilities so plugin code depends on this package alone.
type NotificationAdapterCapabilities = notification.NotificationAdapterCapabilities

// NotificationRecipientOverride re-exports notification.NotificationRecipientOverride so plugin code depends on this package alone.
type NotificationRecipientOverride = notification.NotificationRecipientOverride

// NotificationMessage re-exports notification.NotificationMessage so plugin code depends on this package alone.
type NotificationMessage = notification.NotificationMessage

// NotificationAttachment re-exports notification.NotificationAttachment so plugin code depends on this package alone.
type NotificationAttachment = notification.NotificationAttachment

// NotificationFile re-exports notification.NotificationFile so plugin code depends on this package alone.
type NotificationFile = notification.NotificationFile

// --- Identity family (specs/identity) ---------------------------

// IdentityAdapterRegistry re-exports identity.IdentityAdapterRegistry so plugin code depends on this package alone.
type IdentityAdapterRegistry = identity.IdentityAdapterRegistry

// IdentityAdapterSpec re-exports identity.IdentityAdapterSpec so plugin code depends on this package alone.
type IdentityAdapterSpec = identity.IdentityAdapterSpec

// IdentityAdapterCapabilities re-exports identity.IdentityAdapterCapabilities so plugin code depends on this package alone.
type IdentityAdapterCapabilities = identity.IdentityAdapterCapabilities

// IdentityAdapterClient re-exports identity.IdentityAdapterClient so plugin code depends on this package alone.
type IdentityAdapterClient = identity.IdentityAdapterClient

// IdentityAuthRequest re-exports identity.IdentityAuthRequest so plugin code depends on this package alone.
type IdentityAuthRequest = identity.IdentityAuthRequest

// IdentityAuthResult re-exports identity.IdentityAuthResult so plugin code depends on this package alone.
type IdentityAuthResult = identity.IdentityAuthResult

// IdentityCallbackRequest re-exports identity.IdentityCallbackRequest so plugin code depends on this package alone.
type IdentityCallbackRequest = identity.IdentityCallbackRequest

// IdentityCallbackResult re-exports identity.IdentityCallbackResult so plugin code depends on this package alone.
type IdentityCallbackResult = identity.IdentityCallbackResult

// IdentityGroup re-exports identity.IdentityGroup so plugin code depends on this package alone.
type IdentityGroup = identity.IdentityGroup

// --- AI family (specs/ai) ---------------------------------------
//
// The AI family has TWO request/response shapes that previously shared
// the same `plugin.AIRequest` / `plugin.AIMessage` / `plugin.AIResponse`
// / `plugin.AIUsage` symbol names but pointed at DIFFERENT types:
//   - Consumer side (services.AIHandle.Complete): the AIRequest /
//     AIMessage / AIResponse / AIUsage shapes a plugin sends to the
//     host's AI router.
//   - Provider side (ai.AIAdapterClient.Complete): the LLMRequest /
//     LLMMessage / LLMResponse / LLMUsage / LLMTool shapes the host
//     sends to an AI provider plugin.
// Plugins rarely need both — a plugin is either an AI consumer (uses
// host.AI) or an AI provider (registers via AdapterContributor), not
// both. The shim aliases the consumer-side names (matching the legacy
// plugin.AI* identifiers) and the provider-side names (matching the
// legacy plugin.LLM* identifiers).

// AIAdapterRegistry re-exports ai.AIAdapterRegistry so plugin code depends on this package alone.
type AIAdapterRegistry = ai.AIAdapterRegistry

// AIAdapterSpec re-exports ai.AIAdapterSpec so plugin code depends on this package alone.
type AIAdapterSpec = ai.AIAdapterSpec

// AIAdapterCapabilities re-exports ai.AIAdapterCapabilities so plugin code depends on this package alone.
type AIAdapterCapabilities = ai.AIAdapterCapabilities

// AITestResult re-exports ai.AITestResult so plugin code depends on this package alone.
type AITestResult = ai.AITestResult

// AIAdapterClient re-exports ai.AIAdapterClient so plugin code depends on this package alone.
type AIAdapterClient = ai.AIAdapterClient

// ModelDecl re-exports ai.ModelDecl so plugin code depends on this package alone.
type ModelDecl = ai.ModelDecl

// LLMRequest re-exports ai.LLMRequest, a provider-side shape used by ai.AIAdapterClient methods.
type LLMRequest = ai.LLMRequest

// LLMMessage re-exports ai.LLMMessage so plugin code depends on this package alone.
type LLMMessage = ai.LLMMessage

// LLMTool re-exports ai.LLMTool so plugin code depends on this package alone.
type LLMTool = ai.LLMTool

// LLMToolCall re-exports ai.LLMToolCall so plugin code depends on this package alone.
type LLMToolCall = ai.LLMToolCall

// LLMResponse re-exports ai.LLMResponse so plugin code depends on this package alone.
type LLMResponse = ai.LLMResponse

// LLMUsage re-exports ai.LLMUsage so plugin code depends on this package alone.
type LLMUsage = ai.LLMUsage

// --- Snapshot family (specs/snapshot) ---------------------------

// SnapshotAdapterRegistry re-exports snapshot.SnapshotAdapterRegistry so plugin code depends on this package alone.
type SnapshotAdapterRegistry = snapshot.SnapshotAdapterRegistry

// SnapshotAdapterSpec re-exports snapshot.SnapshotAdapterSpec so plugin code depends on this package alone.
type SnapshotAdapterSpec = snapshot.SnapshotAdapterSpec

// SnapshotAdapterCapabilities re-exports snapshot.SnapshotAdapterCapabilities so plugin code depends on this package alone.
type SnapshotAdapterCapabilities = snapshot.SnapshotAdapterCapabilities

// SnapshotAdapterClient re-exports snapshot.SnapshotAdapterClient so plugin code depends on this package alone.
type SnapshotAdapterClient = snapshot.SnapshotAdapterClient

// SnapshotCreateRequest re-exports snapshot.SnapshotCreateRequest so plugin code depends on this package alone.
type SnapshotCreateRequest = snapshot.SnapshotCreateRequest

// SnapshotRef re-exports snapshot.SnapshotRef so plugin code depends on this package alone.
type SnapshotRef = snapshot.SnapshotRef

// SnapshotCapability re-exports snapshot.SnapshotCapability so plugin code depends on this package alone.
type SnapshotCapability = snapshot.SnapshotCapability

// SnapshotTarget re-exports snapshot.SnapshotTarget so plugin code depends on this package alone.
type SnapshotTarget = snapshot.SnapshotTarget

// --- Logging family (specs/logging) -----------------------------
//
// Audit log forwarders: HTTP SIEM collectors and RFC5424 syslog
// daemons. Distinct from the notification family — notification
// adapters carry human-readable Title/Message pairs, logging
// adapters carry machine-readable structured audit records.

// LogForwarderAdapterRegistry re-exports logging.LogForwarderAdapterRegistry so plugin code depends on this package alone.
type LogForwarderAdapterRegistry = logging.LogForwarderAdapterRegistry

// LogForwarderAdapterSpec re-exports logging.LogForwarderAdapterSpec so plugin code depends on this package alone.
type LogForwarderAdapterSpec = logging.LogForwarderAdapterSpec

// LogForwarderAdapterClient re-exports logging.LogForwarderAdapterClient so plugin code depends on this package alone.
type LogForwarderAdapterClient = logging.LogForwarderAdapterClient

// LogForwarderAdapterCapabilities re-exports logging.LogForwarderAdapterCapabilities so plugin code depends on this package alone.
type LogForwarderAdapterCapabilities = logging.LogForwarderAdapterCapabilities

// LogForwardResult re-exports logging.ForwardResult so plugin code depends on this package alone.
type LogForwardResult = logging.ForwardResult

// LogDelivery re-exports logging.LogDelivery so plugin code depends on this package alone.
type LogDelivery = logging.LogDelivery

// --- Scanner family (specs/scanner) -----------------------------
//
// Image vulnerability scanners: Trivy and Grype wrap a local CLI,
// Clair calls a v4 server. The adapter turns an image reference into
// findings and keeps no state; the host owns the scan rows, the
// vulnerability rows, retention and the history page.

// ImageScannerAdapterRegistry re-exports scanner.ImageScannerAdapterRegistry so plugin code depends on this package alone.
type ImageScannerAdapterRegistry = scanner.ImageScannerAdapterRegistry

// ImageScannerAdapterSpec re-exports scanner.ImageScannerAdapterSpec so plugin code depends on this package alone.
type ImageScannerAdapterSpec = scanner.ImageScannerAdapterSpec

// ImageScannerAdapterClient re-exports scanner.ImageScannerAdapterClient so plugin code depends on this package alone.
type ImageScannerAdapterClient = scanner.ImageScannerAdapterClient

// ImageScannerAdapterCapabilities re-exports scanner.ImageScannerAdapterCapabilities so plugin code depends on this package alone.
type ImageScannerAdapterCapabilities = scanner.ImageScannerAdapterCapabilities

// ScanReport re-exports scanner.ScanReport so plugin code depends on this package alone.
type ScanReport = scanner.ScanReport

// ScanFinding re-exports scanner.ScanFinding so plugin code depends on this package alone.
type ScanFinding = scanner.ScanFinding

// ScannerTestStep is the scanner family's own copy of the four-field
// step shape. Named distinctly from the storage family's TestStep
// (aliased above as plugin.TestStep) because the two are separate
// types in separate spec packages — see the note on scanner.TestStep.
type ScannerTestStep = scanner.TestStep

// RicherScannerTester re-exports scanner.RicherScannerTester so plugin code depends on this package alone.
type RicherScannerTester = scanner.RicherScannerTester

// --- Alerting family (specs/alerting) ---------------------------
//
// Alert condition sources. A source answers "is this condition true,
// and for which subjects?"; the host owns thresholds, scheduling,
// the firing state machine and delivery. The host's own container /
// host / platform conditions register through the same surface.

// AlertSourceAdapterRegistry re-exports alerting.AlertSourceAdapterRegistry so plugin code depends on this package alone.
type AlertSourceAdapterRegistry = alerting.AlertSourceAdapterRegistry

// AlertSourceAdapterSpec re-exports alerting.AlertSourceAdapterSpec so plugin code depends on this package alone.
type AlertSourceAdapterSpec = alerting.AlertSourceAdapterSpec

// AlertSourceAdapterClient re-exports alerting.AlertSourceAdapterClient so plugin code depends on this package alone.
type AlertSourceAdapterClient = alerting.AlertSourceAdapterClient

// AlertConditionDecl re-exports alerting.AlertConditionDecl so plugin code depends on this package alone.
type AlertConditionDecl = alerting.AlertConditionDecl

// AlertConditionKind re-exports alerting.AlertConditionKind so plugin code depends on this package alone.
type AlertConditionKind = alerting.AlertConditionKind

// AlertTargetKind re-exports alerting.AlertTargetKind so plugin code depends on this package alone.
type AlertTargetKind = alerting.AlertTargetKind

// AlertEvaluateRequest re-exports alerting.EvaluateRequest so plugin code depends on this package alone.
type AlertEvaluateRequest = alerting.EvaluateRequest

// AlertObservation re-exports alerting.AlertObservation so plugin code depends on this package alone.
type AlertObservation = alerting.AlertObservation

// The kind and target vocabularies are constants rather than types, so
// they need re-exporting by value for a plugin to declare a condition
// without importing specs/alerting directly.
const (
	AlertKindThreshold     = alerting.KindThreshold
	AlertKindDuration      = alerting.KindDuration
	AlertKindEvent         = alerting.KindEvent
	AlertKindCountInWindow = alerting.KindCountInWindow

	AlertTargetContainer   = alerting.TargetContainer
	AlertTargetEnvironment = alerting.TargetEnvironment
	AlertTargetNone        = alerting.TargetNone
)

// --- Reporting family (specs/reporting) -------------------------
//
// Report providers. The one family whose consumer is another plugin
// (Reporting) rather than the host: a provider contributes report
// types and builds their sections, the Reporting plugin owns the
// cover, the annex, the persistence and the three renderers.

// ReportAdapterRegistry re-exports reporting.ReportAdapterRegistry so plugin code depends on this package alone.
type ReportAdapterRegistry = reporting.ReportAdapterRegistry

// ReportProviderSpec re-exports reporting.ReportProviderSpec so plugin code depends on this package alone.
type ReportProviderSpec = reporting.ReportProviderSpec

// ReportProviderClient re-exports reporting.ReportProviderClient so plugin code depends on this package alone.
type ReportProviderClient = reporting.ReportProviderClient

// ReportTypeDecl re-exports reporting.ReportTypeDecl so plugin code depends on this package alone.
type ReportTypeDecl = reporting.ReportTypeDecl

// ReportOptionDecl re-exports reporting.ReportOptionDecl so plugin code depends on this package alone.
type ReportOptionDecl = reporting.ReportOptionDecl

// ReportRequest re-exports reporting.ReportRequest so plugin code depends on this package alone.
type ReportRequest = reporting.ReportRequest

// ReportResult re-exports reporting.ReportResult so plugin code depends on this package alone.
type ReportResult = reporting.ReportResult

// ReportingHandle re-exports reporting.ReportingHandle so plugin code depends on this package alone.
type ReportingHandle = reporting.ReportingHandle

// ReportSection re-exports reporting.Section so plugin code depends on this package alone.
type ReportSection = reporting.Section

// ReportBlock re-exports reporting.Block so plugin code depends on this package alone.
type ReportBlock = reporting.Block

// ReportKPI re-exports reporting.KPI so plugin code depends on this package alone.
type ReportKPI = reporting.KPI

// ReportColumn re-exports reporting.Column so plugin code depends on this package alone.
type ReportColumn = reporting.Column

// ReportCell re-exports reporting.Cell so plugin code depends on this package alone.
type ReportCell = reporting.Cell

// ReportPair re-exports reporting.Pair so plugin code depends on this package alone.
type ReportPair = reporting.Pair

// ReportFinding re-exports reporting.Finding so plugin code depends on this package alone.
type ReportFinding = reporting.Finding

// ReportChart re-exports reporting.Chart so plugin code depends on this package alone.
type ReportChart = reporting.Chart

// ReportChartSeries re-exports reporting.ChartSeries so plugin code depends on this package alone.
type ReportChartSeries = reporting.ChartSeries

// ReportChartPoint re-exports reporting.ChartPoint so plugin code depends on this package alone.
type ReportChartPoint = reporting.ChartPoint

// ReportFacetDecl re-exports reporting.FacetDecl so plugin code depends on this package alone.
type ReportFacetDecl = reporting.FacetDecl

// ReportFacetRequest re-exports reporting.FacetRequest so plugin code depends on this package alone.
type ReportFacetRequest = reporting.FacetRequest

// ReportFacetIdentity re-exports reporting.FacetIdentity so plugin code depends on this package alone.
type ReportFacetIdentity = reporting.FacetIdentity

// ReportInventoryRow re-exports reporting.InventoryRow so plugin code depends on this package alone.
type ReportInventoryRow = reporting.InventoryRow

// ReportCapacityRow re-exports reporting.CapacityRow so plugin code depends on this package alone.
type ReportCapacityRow = reporting.CapacityRow

// ReportProtectionRow re-exports reporting.ProtectionRow so plugin code depends on this package alone.
type ReportProtectionRow = reporting.ProtectionRow

// ReportHealthRow re-exports reporting.HealthRow so plugin code depends on this package alone.
type ReportHealthRow = reporting.HealthRow

// ReportExposureRow re-exports reporting.ExposureRow so plugin code depends on this package alone.
type ReportExposureRow = reporting.ExposureRow

// ReportCostRow re-exports reporting.CostRow so plugin code depends on this package alone.
type ReportCostRow = reporting.CostRow

// ReportFacetResult re-exports reporting.FacetResult so plugin code depends on this package alone.
type ReportFacetResult = reporting.FacetResult

// ReportFacetContributor re-exports reporting.ReportFacetContributor so plugin code depends on this package alone.
type ReportFacetContributor = reporting.ReportFacetContributor

// ReportFacetProvider re-exports reporting.FacetProvider so plugin code depends on this package alone.
type ReportFacetProvider = reporting.FacetProvider

// ReportFacetContribution re-exports reporting.FacetContribution so plugin code depends on this package alone.
type ReportFacetContribution = reporting.FacetContribution

// ReportFacetHandle re-exports reporting.ReportFacetHandle so plugin code depends on this package alone.
type ReportFacetHandle = reporting.ReportFacetHandle

// The report facets, re-exported so a provider names them without importing specs/reporting.
const (
	ReportFacetInventory  = reporting.FacetInventory
	ReportFacetCapacity   = reporting.FacetCapacity
	ReportFacetProtection = reporting.FacetProtection
	ReportFacetHealth     = reporting.FacetHealth
	ReportFacetExposure   = reporting.FacetExposure
	ReportFacetCost       = reporting.FacetCost
)

// ReportSourceStatus re-exports reporting.SourceStatus so plugin code depends on this package alone.
type ReportSourceStatus = reporting.SourceStatus

// ReportSubjectOption re-exports reporting.SubjectOption so plugin code depends on this package alone.
type ReportSubjectOption = reporting.SubjectOption

// --- Host-only types (host) -------------------------------------

// PermissionLabel + LabelRegistry + NewLabelRegistry + the schema
// helpers + the trusted-keys helpers live in `host` (the plugin
// never imports `host`). The shim re-exports them so existing host
// callers that still use `plugin.PermissionLabel`,
// `plugin.NewLabelRegistry`, `plugin.ValidateManifestSchema`,
// `plugin.LoadTrustedKeysFromEnv`, etc. keep compiling.

// PermissionLabel re-exports host.PermissionLabel so plugin code depends on this package alone.
type PermissionLabel = host.PermissionLabel

// LabelRegistry re-exports host.LabelRegistry so plugin code depends on this package alone.
type LabelRegistry = host.LabelRegistry

// ManifestSchemaError re-exports host.ManifestSchemaError so plugin code depends on this package alone.
type ManifestSchemaError = host.ManifestSchemaError

// ManifestError re-exports host.ManifestError so plugin code depends on this package alone.
type ManifestError = host.ManifestError

// SchemaReportT re-exports host.SchemaReportT so plugin code depends on this package alone.
type SchemaReportT = host.SchemaReportT

// NewLabelRegistry, ValidateManifestSchema, ManifestSchemaJSON,
// SchemaReport, DefaultPinnedKeys, TrustedKeys,
// LoadTrustedKeysFromEnv, ParsePEMPublicKeys are re-assigned as
// package-level vars so callers can call them as functions.
var (
	NewLabelRegistry       = host.NewLabelRegistry
	ValidateManifestSchema = host.ValidateManifestSchema
	KnownScopeToken        = host.KnownScopeToken
	ManifestSchemaJSON     = host.ManifestSchemaJSON
	SchemaReport           = host.SchemaReport
	SchemaLenient          = host.SchemaLenient
	DefaultPinnedKeys      = host.DefaultPinnedKeys
	TrustedKeys            = host.TrustedKeys
	LoadTrustedKeysFromEnv = host.LoadTrustedKeysFromEnv
	ParsePEMPublicKeys     = host.ParsePEMPublicKeys
)

// --- *AdapterSource interfaces (host) ----------------------------

// These three interfaces are implemented by the host's plugin
// runtime; consumers (storage_locations, communications,
// container_backups) type-assert against them via
// `_, ok := runtime.(host.StorageAdapterSource)`. The shim
// re-exports them so existing callers that still use
// `plugin.StorageAdapterSource` keep compiling.

// StorageAdapterSource re-exports host.StorageAdapterSource so plugin code depends on this package alone.
type StorageAdapterSource = host.StorageAdapterSource

// NotificationAdapterSource re-exports host.NotificationAdapterSource so plugin code depends on this package alone.
type NotificationAdapterSource = host.NotificationAdapterSource

// SnapshotAdapterSource re-exports host.SnapshotAdapterSource so plugin code depends on this package alone.
type SnapshotAdapterSource = host.SnapshotAdapterSource

// AlertSourceAdapterSource re-exports host.AlertSourceAdapterSource so plugin code depends on this package alone.
type AlertSourceAdapterSource = host.AlertSourceAdapterSource

// --- Variable re-exports ----------------------------------------

// ErrScopeViolation, ErrUnknownHostAction, ErrPlatformUnsupported
// live in `contract` (they're surfaced through the Plugin interface).
var (
	ErrScopeViolation      = contract.ErrScopeViolation
	ErrUnknownHostAction   = contract.ErrUnknownHostAction
	ErrPlatformUnsupported = contract.ErrPlatformUnsupported
)

// ErrPermissionExists, ErrPermissionInUse, ErrSchemaExists,
// ErrSchemaInUse live in `host` (the AccessRegistry's install-time
// conflicts surface from there).
var (
	ErrPermissionExists = host.ErrPermissionExists
	ErrPermissionInUse  = host.ErrPermissionInUse
	ErrSchemaExists     = host.ErrSchemaExists
	ErrSchemaInUse      = host.ErrSchemaInUse
)

// ErrUnsupported lives in `specs/storage`. Re-exported here so
// storage_locations callers that previously did
// `if errors.Is(err, plugin.ErrUnsupported)` keep working.
var ErrUnsupported = storage.ErrUnsupported
