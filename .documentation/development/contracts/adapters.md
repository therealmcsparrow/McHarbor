# Adapter and Provider Contracts

The contracts a plugin implements to become a runtime handler for part of the host. There are two
kinds:

- **Adapter families**: registered explicitly through `RegisterAdapters(AdapterRegistry)`. They
  live in `src/backend/core/plugin/specs/<family>/specs.go` and are re-exported by
  `core/plugin`.
- **Providers**: discovered by type assertion. The loader checks whether the `Plugin` value, or
  a registered client, also implements the interface. Import these straight from
  `core/plugin/specs/<name>`, because they are not in the re-export.

## How every adapter family works

Every family has the same three parts:

| Part | Is | Who fills it |
|---|---|---|
| `…Registry` | `Register(spec, client) error` | the host hands it to the plugin |
| `…Spec` | What the host renders and dispatches on: a type key, `Slug`, `DisplayName`, `Icon`, `Capabilities`, `ConfigSchema` | the plugin |
| `…Client` | The code that does the work, called with the decrypted `config map[string]string` of one configured instance | the plugin |

The host owns the admin surface: create, edit, test and delete screens, stored configuration with
secrets encrypted, permissions and audit. The plugin owns only the work. A configured instance's
settings reach the client as `config` on every call, and the client keeps no per-instance state
of its own. Write clients to be safe for concurrent use. The storage contract says outright that the host calls them from parallel goroutines.

```go
func (p *CommSlackPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Notification().Register(
		plugin.NotificationAdapterSpec{
			ChannelType:  "slack",
			Slug:         p.manifest.Slug,
			DisplayName:  "Slack",
			Icon:         "plugin.svg",
			Capabilities: plugin.NotificationAdapterCapabilities{EncryptedCredentials: true},
			ConfigSchema: []plugin.ConfigFieldDecl{
				{Key: "webhook_url", Type: "secret", Label: "Webhook URL", Required: true},
			},
		},
		NewSlackClient(), // implements plugin.NotificationAdapterClient
	)
}
```

## Adapter families

### Storage: `Storage()`

Backup destinations. The host keeps locations, plans and credentials.

| | |
|---|---|
| Spec | `StorageAdapterSpec{LocationType, DisplayName, Slug, Icon, Capabilities, ConfigSchema}` |
| Capabilities | `EncryptedCredentials`, `OAuth`, `SupportsListing`, `SupportsDelete` |
| Client | `Test(ctx, config) error` · `Upload(ctx, config, key, r io.Reader) error` · `Download(ctx, config, key) (io.ReadCloser, error)` · `Delete(ctx, config, key) error` · `List(ctx, config, prefix) ([]StorageObject, error)` · `ConsentURL(ctx, locationID, locationType, publicBaseURL, state, cfg) (string, error)` · `ConsentCallback(ctx, locationID, locationType, publicBaseURL, code, state, cfg) (map[string]string, error)` |
| Optional extensions | `RicherStorageTester.TestSteps(ctx, config) ([]TestStep, error)`, which gives a step-by-step Test. `StorageQuotaReporter.Quota(ctx, config) (StorageQuota, error)`, which gives used and free space. |
| Types | `StorageObject`, `StorageQuota`, `TestStep` |

### Notification: `Notification()`

Communication channels. The host keeps channels, routing and retries.

| | |
|---|---|
| Spec | `NotificationAdapterSpec{ChannelType, DisplayName, Slug, Icon, Description, Capabilities, ConfigSchema}` |
| Capabilities | `EncryptedCredentials`, `SupportsRichAttachments`, `SupportsFileAttachments`, `RecipientOverride` |
| Client | `Test(ctx, config) error` · `Send(ctx, config, msg NotificationMessage) error` |
| Optional extensions | The two-way ITSM contracts in [Incident lifecycle](#incident-lifecycle-specsincident), implemented on the same client |
| Types | `NotificationMessage`, `NotificationFile`, `NotificationAttachment`, `NotificationRecipientOverride` |

### Identity: `Identity()`

Identity providers. The host keeps users, sessions and group mapping.

| | |
|---|---|
| Spec | `IdentityAdapterSpec{ProviderType, DisplayName, Slug, Icon, Capabilities, ConfigSchema}` |
| Capabilities | `OIDCRedirect`, `SAMLRedirect`, `SupportsGroupFetch`, `SupportsMetadata` |
| Client | `Test(ctx, config) error` · `BeginAuth(ctx, config, IdentityAuthRequest) (*IdentityAuthResult, error)` · `HandleCallback(ctx, config, IdentityCallbackRequest) (*IdentityCallbackResult, error)` · `FetchGroups(ctx, config) ([]IdentityGroup, error)` · `MetadataXML(ctx, config, baseURL) ([]byte, error)` |
| Note | OIDC and SAML redirect through the host's own dispatcher. Only the `ldap` provider type checks a credential itself, through the external-login surface in `pluginruntime/external_login.go`. Any other caller gets `ErrExternalLoginNotOwner`. |

### Snapshot: `Snapshot()`

Snapshot drivers, which give a backup a consistent point in time.

| | |
|---|---|
| Spec | `SnapshotAdapterSpec{Driver, DisplayName, Slug, Icon, Description, Capabilities, ConfigSchema}` |
| Capabilities | `SupportsMount`, `SupportsRemoteTarget`, `RequiresAgent` |
| Client | `Capability(ctx, config) (SnapshotCapability, error)` · `Create(ctx, config, SnapshotCreateRequest) (SnapshotRef, error)` · `Remove(ctx, config, SnapshotRef) error` |
| Types | `SnapshotCreateRequest`, `SnapshotRef`, `SnapshotCapability`, `SnapshotTarget` |

A driver that wraps a CLI tool on the agent host (zfs, btrfs, lvm) calls
`HostContext.SnapshotRPC`. One that talks to a NAS's REST API (Synology, TrueNAS) calls it
directly from the host process.

### AI: `AI()`

AI providers. The host keeps the router, consent and prompts.

| | |
|---|---|
| Spec | `AIAdapterSpec{ProviderType, Slug, Icon, DisplayName, Description, DefaultModel, Models, Capabilities, ConfigSchema, OAuthDefaults}` |
| Capabilities | `EncryptedCredentials`, `SupportsStreaming`, `SupportsTools`, `SupportsVision`, `ListModelsDynamic`, `SupportsOAuth` |
| Client | `Test(ctx, config) (*AITestResult, error)` · `Complete(ctx, config, LLMRequest) (*LLMResponse, error)` · `Models(ctx, config) ([]ModelDecl, error)` |
| Types | `LLMRequest`, `LLMMessage`, `LLMTool`, `LLMToolCall`, `LLMResponse`, `LLMUsage`, `ModelDecl`, `AITestResult`, and `OAuthDefaults` for a zero-config device-flow sign-in |

### Logging: `Logging()`

Audit log forwarders.

| | |
|---|---|
| Spec | `LogForwarderAdapterSpec{ForwarderType, Slug, DisplayName, Icon, Description, Capabilities, ConfigSchema}` |
| Capabilities | `EncryptedCredentials`, `SupportsDeliveryHistory`, `RequiresLeaderElection` |
| Client | `Test(ctx, config) error` · `ForwardPending(ctx, config) (ForwardResult, error)` · `DeliveryHistory(ctx, config, limit) ([]LogDelivery, error)` |

### Alerting: `Alerting()`

Alert condition sources, for a resource the plugin owns. The host's alert engine schedules,
thresholds, deduplicates and delivers them the same way it handles its built-in conditions.

| | |
|---|---|
| Spec | `AlertSourceAdapterSpec{SourceType, DisplayName, Slug, Icon, Description, Conditions}` |
| Condition | `AlertConditionDecl{Key, DisplayName, Description, Kind, Unit, Operators, DefaultOperator, DefaultThreshold, SupportsFor, TargetKind, PollInterval, Fields}` |
| Client | `Evaluate(ctx, EvaluateRequest) ([]AlertObservation, error)` |

The Reporting add-on registers one, so that a schedule which stops producing reports raises an
alert:

```go
func (p *ReportingPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	if err := reg.Alerting().Register(reportingAlertSpec(), &alertSource{plugin: p}); err != nil {
		return fmt.Errorf("reporting: register alert source: %w", err)
	}
	return nil
}
```

### Reporting: `Reporting()`

Report providers. This is the one family whose consumer is another plugin, the Reporting add-on,
rather than the host.

| | |
|---|---|
| Spec | `ReportProviderSpec{Slug, DisplayName, Types}` |
| Report type | `ReportTypeDecl{Key, Category, Levels, Options, Subjects, DefaultDays, PointInTime, EnvironmentScoped, TitleEN, SubtitleEN, LabelKey, DescriptionKey, CategoryLabelKey, CategoryDescriptionKey}`. The key is prefixed with the provider's slug, and the labels resolve in the provider's own i18n namespace. |
| Client | `Build(ctx, ReportRequest) (ReportResult, error)` · `Subjects(ctx, kind, ReportRequest) ([]SubjectOption, error)` |
| Consumer side | `ReportingHandle` on `HostContext.Reporting`: `Providers()`, `Build(ctx, typeKey, req)`, `Subjects(ctx, typeKey, kind, req)`, `Available()` |
| Document model | `Section`, `Block`, `KPI`, `Column`, `Cell`, `Pair`, `Finding`, `Chart`, `ChartSeries`, `ChartPoint` |

### Scanner: `Scanner()`

Image vulnerability scanners. The host keeps scan rows, findings, retention and history.

| | |
|---|---|
| Spec | `ImageScannerAdapterSpec{ScannerType, Slug, DisplayName, Icon, Description, Capabilities, ConfigSchema}` |
| Capabilities | `EncryptedCredentials`, `RequiresLocalBinary`, `SupportsTest` |
| Client | `Available(ctx, config) bool` · `Test(ctx, config) error` · `Scan(ctx, config, imageRef) (ScanReport, error)` |
| Optional extension | `RicherScannerTester.TestSteps(ctx, config) ([]TestStep, error)` |
| Types | `ScanReport`, `ScanFinding`, `TestStep` |

### Backup: `Backup()`

Backup sources beside the host's own container backups, such as virtual machines and system
containers, shown on the same Backups page.

| | |
|---|---|
| Spec | `BackupProviderSpec{Slug, SubjectKind, DisplayName, Icon, Capabilities}` |
| Capabilities | `Restore`, `Delete`, `Plans` |
| Client | `Runs(ctx, RunQuery) ([]RunSummary, error)` · `Plans(ctx, environmentID) ([]PlanSummary, error)` |
| Related | Write lifecycle events to `HostContext.BackupLog` so they appear in the page's log |

## Providers found by type assertion

Implement the interface on the `Plugin` value, or on the adapter client where noted. There is
nothing to register: the loader finds it after `Init`, and drops it on a reload that removed it.

### ABAC resources: `specs/abac`

```go
type Provider interface {
	ABACResourceTypes() []ResourceType
	ResolveABACResource(ctx context.Context, resourceType, envID, id string) (*Resource, error)
}
```

Publishes resource types that ABAC policies can target, and resolves one instance when a policy
is evaluated. Types: `ResourceType`, `Resource`, `Attribute`, `Route`. Wired in
`pluginruntime/abac_resources.go`.

### Desired state: `specs/desiredstate`

```go
type Provider interface {
	DesiredKinds() []Kind
	DesiredSubjects(ctx context.Context, kind, environmentID string) ([]Option, error)
	DesiredOptions(ctx context.Context, kind, field, environmentID string) ([]Option, error)
	CurrentSpec(ctx context.Context, kind, environmentID, subjectID string) (map[string]string, error)
	DetectDrift(ctx context.Context, kind, environmentID, subjectID string, spec map[string]string) ([]Difference, error)
	PlanDrift(ctx context.Context, kind, environmentID, subjectID string, spec map[string]string, diffs []Difference) ([]Step, error)
	ApplyStep(ctx context.Context, kind, environmentID, subjectID string, spec map[string]string, step Step) error
}

type ScopeProvider interface {
	DesiredScopes(ctx context.Context, kind string) ([]Option, error)
}
```

Lets the reconciler watch what the plugin manages: declare kinds, read the current spec, detect
drift, plan it as steps and apply them one at a time. `ScopeProvider` is optional. Types: `Kind`,
`Field`, `Option`, `Difference`, `Step`. Wired in `pluginruntime/desired_state_sources.go`.

### Incident sources: `specs/incidentsource`

```go
type Provider interface {
	IncidentSubjectTypes() []SubjectType
	IncidentSignals(ctx context.Context, since time.Time) ([]Signal, error)
	IncidentEvidence(ctx context.Context, subjectType, environmentID, subjectID string, from, to time.Time) (*Evidence, error)
	IncidentSettled(ctx context.Context, subjectType, environmentID, subjectID string) (bool, error)
}
```

Lets the plugin raise incidents about what it manages and supply their evidence. Types:
`SubjectType`, `SignalDecl`, `Signal`, `Section`, `Evidence`. Wired in
`pluginruntime/incident_sources.go`.

### Incident lifecycle: `specs/incident`

Implemented on a **notification client**, beside `NotificationAdapterClient`, by an ITSM channel
that keeps a ticket in step with an alert. The host finds each part through
`core/notify/incident.go`.

```go
type LifecycleClient interface {
	Open(ctx context.Context, config map[string]string, ev Event) (Ref, error)
	Update(ctx context.Context, config map[string]string, ref Ref, ev Event) error
	Resolve(ctx context.Context, config map[string]string, ref Ref, ev Event) error
}

type WebhookParser interface { // optional: the vendor pushes ticket changes
	VerifyWebhook(config map[string]string, header http.Header, body []byte) error
	ParseWebhook(ctx context.Context, config map[string]string, header http.Header, body []byte) ([]RemoteStatus, error)
}

type Poller interface { // optional: the host pulls ticket changes
	Poll(ctx context.Context, config map[string]string, refs []Ref) ([]RemoteStatus, error)
}
```

`Event.DedupKey` is `alertID:environmentID:subjectID`, stable for the life of a condition.
Severity is McHarbor's three levels, which each plugin maps to its vendor's scale in one table.
A plugin reports the remote status as one of five normalised states. `RetryAfterError` carries a
vendor's Retry-After, so the outbox waits exactly that long instead of using its own backoff. An
error wrapped in `ErrOperatorMessage` is safe to show an operator, and anything else is shown as a
generic message. The host audits each call with its shape only, never the ticket content.

## Compute types

The `specs/compute` package no longer exists. A plugin that talks to an agent declares its own wire
structs and sends JSON through `HostContext.AgentRPC`; see [Agent RPC](./agent-rpc/README.md).
