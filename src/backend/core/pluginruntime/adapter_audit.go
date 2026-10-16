// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Plugin work is audited here, in the host, rather than in each plugin.
//
// A plugin's own Init / Shutdown rows come from the plugin itself
// (host.Audit.Emit). Everything a plugin is actually *asked to do* —
// send a notification, upload an archive, complete a chat, take a
// snapshot, scan an image, finish an SSO login — arrives through one of
// the nine adapter families in the registry, and none of those calls
// used to leave a trace. Auditing at registration means a plugin cannot
// forget to audit, cannot attribute a call to a different plugin (the
// slug is host-supplied), and every future plugin inherits the coverage
// without writing a line of audit code.
//
// Not every call earns a row. Three operations run on a ticker or on
// every render, and a row per tick buries the rows an operator actually
// needs; one of them would feed itself, since the log forwarder's job is
// to ship audit events, so a success row per cycle becomes an event to
// ship on the next cycle, forever. Those are auditNever or
// auditOnFailure — see the mode passed at each call site below.

// auditMode says when an adapter operation earns an audit row.
type auditMode int

const (
	// auditAlways records both outcomes: this call is a discrete unit
	// of work an operator asked for.
	auditAlways auditMode = iota
	// auditOnFailure records only failures: the call is a read, a
	// probe, or a ticker cycle whose success is not news.
	auditOnFailure
	// auditNever records nothing: the call runs on every render and
	// touches no external system.
	auditNever
)

// Adapter audit actions. One action per operation, with the outcome
// carried by the row's severity rather than by a second action name —
// so a filter on "plugin.adapter.storage.upload" shows the failures
// next to the successes instead of hiding them under a sibling key.
const (
	actStorageTest            = "plugin.adapter.storage.test"
	actStorageUpload          = "plugin.adapter.storage.upload"
	actStorageDownload        = "plugin.adapter.storage.download"
	actStorageDelete          = "plugin.adapter.storage.delete"
	actStorageList            = "plugin.adapter.storage.list"
	actStorageQuota           = "plugin.adapter.storage.quota"
	actStorageConsentURL      = "plugin.adapter.storage.consent_url"
	actStorageConsentCallback = "plugin.adapter.storage.consent_callback"

	actNotificationTest = "plugin.adapter.notification.test"
	actNotificationSend = "plugin.adapter.notification.send"

	actIdentityTest        = "plugin.adapter.identity.test"
	actIdentityBeginAuth   = "plugin.adapter.identity.begin_auth"
	actIdentityCallback    = "plugin.adapter.identity.callback"
	actIdentityFetchGroups = "plugin.adapter.identity.fetch_groups"
	actIdentityMetadata    = "plugin.adapter.identity.metadata"

	actAITest     = "plugin.adapter.ai.test"
	actAIComplete = "plugin.adapter.ai.complete"
	actAIModels   = "plugin.adapter.ai.models"

	actSnapshotCapability = "plugin.adapter.snapshot.capability"
	actSnapshotCreate     = "plugin.adapter.snapshot.create"
	actSnapshotRemove     = "plugin.adapter.snapshot.remove"

	actLoggingTest    = "plugin.adapter.logging.test"
	actLoggingForward = "plugin.adapter.logging.forward"
	actLoggingHistory = "plugin.adapter.logging.delivery_history"

	actAlertingEvaluate = "plugin.adapter.alerting.evaluate"

	actReportingBuild    = "plugin.adapter.reporting.build"
	actReportingSubjects = "plugin.adapter.reporting.subjects"

	actScannerTest = "plugin.adapter.scanner.test"
	actScannerScan = "plugin.adapter.scanner.scan"
)

// maxAuditErrorLen caps the error text stored in the details column.
// Long enough to identify the failure, short enough that a chatty
// upstream cannot bloat every row.
const maxAuditErrorLen = 300

var (
	// reAuditBearer matches an Authorization-style credential that an
	// upstream client library echoed into its error string.
	reAuditBearer = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/=-]{8,}`)

	// reAuditSecretKV matches a secret carried as a query parameter or
	// a key=value pair. Deliberately conservative about which key names
	// count: over-redacting the error text costs an operator the one
	// clue they had.
	reAuditSecretKV = regexp.MustCompile(`(?i)\b(access[_-]?token|refresh[_-]?token|id[_-]?token|client[_-]?secret|api[_-]?key|apikey|private[_-]?key|token|secret|password|passwd|pwd|signature|assertion|credential)("?\s*[=:]\s*"?)[^\s&"',;)]{4,}`)
)

// redactSecrets strips credentials an adapter's error text may carry
// and truncates the result.
//
// The audit log is admin-only, but "admin-only" is not "safe to print a
// refresh token into": audit rows are exported to SIEM sinks by the
// logging family and kept for years by the retention policy. The error
// itself is still logged in full server-side by the caller.
func redactSecrets(s string) string {
	if s == "" {
		return ""
	}
	s = reAuditBearer.ReplaceAllString(s, "$1 [redacted]")
	s = reAuditSecretKV.ReplaceAllString(s, "$1$2[redacted]")
	if r := []rune(s); len(r) > maxAuditErrorLen {
		s = string(r[:maxAuditErrorLen]) + "…"
	}
	return strings.TrimSpace(s)
}

// RedactAuditError renders err for an audit row's details column, with
// credentials stripped and the text truncated.
//
// Exported for the host bridges (modules/storage_locations,
// modules/communications), which audit the calls a plugin makes *into*
// the host and need the same treatment the adapter wrappers apply to
// the calls the host makes into a plugin. A nil error returns "".
func RedactAuditError(err error) string {
	if err == nil {
		return ""
	}
	return redactSecrets(err.Error())
}

// adapterAuditor writes one audit row per plugin adapter call.
type adapterAuditor struct {
	log    *audit.Logger
	logger *slog.Logger
}

// newAdapterAuditor returns an auditor. Both dependencies may be nil —
// record degrades to a no-op rather than failing the adapter call it
// wraps, because losing an audit row must never lose the work.
func newAdapterAuditor(log *audit.Logger, logger *slog.Logger) *adapterAuditor {
	return &adapterAuditor{log: log, logger: logger}
}

// record writes the row for one completed adapter call.
//
// key is the family's dispatch key (location type, channel type,
// provider type, driver, …) so an operator can tell which of a plugin's
// adapters ran without opening the details blob.
func (a *adapterAuditor) record(ctx context.Context, slug, action, key string, start time.Time, err error, mode auditMode, fields map[string]any) {
	if a == nil || a.log == nil || mode == auditNever {
		return
	}
	// ErrUnsupported is a capability answer, not an outcome. Adapters
	// return it for the operations their family declares optional
	// (Download on a write-only destination, MetadataXML on OIDC), and
	// the host treats it as "ask someone else" — auditing it as an
	// error would teach operators to ignore the error severity.
	if errors.Is(err, plugin.ErrUnsupported) {
		return
	}
	if err == nil && mode == auditOnFailure {
		return
	}

	out := make(map[string]any, len(fields)+5)
	for k, v := range fields {
		out[k] = v
	}
	out["plugin_slug"] = slug
	out["adapter"] = key
	out["duration_ms"] = time.Since(start).Milliseconds()

	severity := "success"
	out["outcome"] = "success"
	if err != nil {
		severity = "error"
		out["outcome"] = "failure"
		out["error"] = redactSecrets(err.Error())
	}

	if emitErr := a.log.EmitPluginSeverityContext(ctx, slug, action, severity, out); emitErr != nil && a.logger != nil {
		a.logger.Error("plugin runtime: writing adapter audit row",
			"slug", slug, "action", action, "adapter", key, "error", emitErr)
	}
}

// --- storage -------------------------------------------------------

type auditedStorageClient struct {
	inner plugin.StorageAdapterClient
	aud   *adapterAuditor
	slug  string
	key   string
}

// auditedRicherStorageClient exists so wrapping does not erase the
// OPTIONAL RicherStorageTester contract. modules/storage_locations
// type-asserts the client to decide between the stepped test and the
// plain one; a wrapper that always implemented TestSteps would make
// every adapter claim the richer contract, and a wrapper that never did
// would silently downgrade the ones that have it.
type auditedRicherStorageClient struct {
	auditedStorageClient
	rich plugin.RicherStorageTester
}

// auditedQuotaStorageClient / auditedRicherQuotaStorageClient keep
// the OPTIONAL StorageQuotaReporter contract visible through the
// audit wrapper, for the same reason auditedRicherStorageClient
// exists: modules/storage_locations type-asserts the client to decide
// whether a usage bar can be rendered.
type auditedQuotaStorageClient struct {
	auditedStorageClient
	quota plugin.StorageQuotaReporter
}

type auditedRicherQuotaStorageClient struct {
	auditedRicherStorageClient
	quota plugin.StorageQuotaReporter
}

func wrapStorageClient(aud *adapterAuditor, slug, key string, inner plugin.StorageAdapterClient) plugin.StorageAdapterClient {
	if aud == nil || aud.log == nil || inner == nil {
		return inner
	}
	base := auditedStorageClient{inner: inner, aud: aud, slug: slug, key: key}
	rich, hasRich := inner.(plugin.RicherStorageTester)
	quota, hasQuota := inner.(plugin.StorageQuotaReporter)
	switch {
	case hasRich && hasQuota:
		return &auditedRicherQuotaStorageClient{
			auditedRicherStorageClient: auditedRicherStorageClient{auditedStorageClient: base, rich: rich},
			quota:                      quota,
		}
	case hasRich:
		return &auditedRicherStorageClient{auditedStorageClient: base, rich: rich}
	case hasQuota:
		return &auditedQuotaStorageClient{auditedStorageClient: base, quota: quota}
	}
	return &base
}

func (c *auditedQuotaStorageClient) Quota(ctx context.Context, config map[string]string) (plugin.StorageQuota, error) {
	start := time.Now()
	q, err := c.quota.Quota(ctx, config)
	c.aud.record(ctx, c.slug, actStorageQuota, c.key, start, err, auditOnFailure, nil)
	return q, err
}

func (c *auditedRicherQuotaStorageClient) Quota(ctx context.Context, config map[string]string) (plugin.StorageQuota, error) {
	start := time.Now()
	q, err := c.quota.Quota(ctx, config)
	c.aud.record(ctx, c.slug, actStorageQuota, c.key, start, err, auditOnFailure, nil)
	return q, err
}

func (c *auditedStorageClient) Test(ctx context.Context, config map[string]string) error {
	start := time.Now()
	err := c.inner.Test(ctx, config)
	c.aud.record(ctx, c.slug, actStorageTest, c.key, start, err, auditAlways, nil)
	return err
}

func (c *auditedRicherStorageClient) TestSteps(ctx context.Context, config map[string]string) ([]plugin.TestStep, error) {
	start := time.Now()
	steps, err := c.rich.TestSteps(ctx, config)
	c.aud.record(ctx, c.slug, actStorageTest, c.key, start, err, auditAlways, map[string]any{"steps": len(steps)})
	return steps, err
}

// countingReader tallies what the adapter actually consumed so the
// audit row can say how large the upload was. The host hands Upload a
// reader, not a byte slice, so the size is not known until the adapter
// has drained it.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

func (c *auditedStorageClient) Upload(ctx context.Context, config map[string]string, key string, r io.Reader) error {
	start := time.Now()
	counter := &countingReader{r: r}
	err := c.inner.Upload(ctx, config, key, counter)
	c.aud.record(ctx, c.slug, actStorageUpload, c.key, start, err, auditAlways, map[string]any{
		"key":   key,
		"bytes": counter.n,
	})
	return err
}

func (c *auditedStorageClient) Download(ctx context.Context, config map[string]string, key string) (io.ReadCloser, error) {
	start := time.Now()
	rc, err := c.inner.Download(ctx, config, key)
	c.aud.record(ctx, c.slug, actStorageDownload, c.key, start, err, auditOnFailure, map[string]any{"key": key})
	return rc, err
}

func (c *auditedStorageClient) Delete(ctx context.Context, config map[string]string, key string) error {
	start := time.Now()
	err := c.inner.Delete(ctx, config, key)
	c.aud.record(ctx, c.slug, actStorageDelete, c.key, start, err, auditAlways, map[string]any{"key": key})
	return err
}

func (c *auditedStorageClient) List(ctx context.Context, config map[string]string, prefix string) ([]plugin.StorageObject, error) {
	start := time.Now()
	objects, err := c.inner.List(ctx, config, prefix)
	c.aud.record(ctx, c.slug, actStorageList, c.key, start, err, auditOnFailure, map[string]any{
		"prefix":  prefix,
		"objects": len(objects),
	})
	return objects, err
}

func (c *auditedStorageClient) ConsentURL(ctx context.Context, locationID, locationType, publicBaseURL, state string, cfg map[string]string) (string, error) {
	start := time.Now()
	url, err := c.inner.ConsentURL(ctx, locationID, locationType, publicBaseURL, state, cfg)
	// The URL itself is not recorded: it carries the state token and,
	// for some providers, a client assertion.
	c.aud.record(ctx, c.slug, actStorageConsentURL, c.key, start, err, auditAlways, map[string]any{
		"location_id":   locationID,
		"location_type": locationType,
	})
	return url, err
}

func (c *auditedStorageClient) ConsentCallback(ctx context.Context, locationID, locationType, publicBaseURL, code, state string, cfg map[string]string) (map[string]string, error) {
	start := time.Now()
	out, err := c.inner.ConsentCallback(ctx, locationID, locationType, publicBaseURL, code, state, cfg)
	// Neither the authorization code nor the returned config map is
	// recorded — the map is where the refresh token lands.
	c.aud.record(ctx, c.slug, actStorageConsentCallback, c.key, start, err, auditAlways, map[string]any{
		"location_id":   locationID,
		"location_type": locationType,
	})
	return out, err
}

// --- notification --------------------------------------------------

type auditedNotificationClient struct {
	inner plugin.NotificationAdapterClient
	aud   *adapterAuditor
	slug  string
	key   string
}

func wrapNotificationClient(aud *adapterAuditor, slug, key string, inner plugin.NotificationAdapterClient) plugin.NotificationAdapterClient {
	if aud == nil || aud.log == nil || inner == nil {
		return inner
	}
	return wrapIncidentCapabilities(&auditedNotificationClient{inner: inner, aud: aud, slug: slug, key: key}, inner)
}

func (c *auditedNotificationClient) Test(ctx context.Context, config map[string]string) error {
	start := time.Now()
	err := c.inner.Test(ctx, config)
	c.aud.record(ctx, c.slug, actNotificationTest, c.key, start, err, auditAlways, nil)
	return err
}

func (c *auditedNotificationClient) Send(ctx context.Context, config map[string]string, msg plugin.NotificationMessage) error {
	start := time.Now()
	err := c.inner.Send(ctx, config, msg)
	// Shape only, never content: a notification body routinely carries
	// container names, alert text, and whatever a workflow templated
	// into it, and the audit log is not the place to duplicate it.
	c.aud.record(ctx, c.slug, actNotificationSend, c.key, start, err, auditAlways, map[string]any{
		"files":        len(msg.Files),
		"attachments":  len(msg.Attachments),
		"message_size": len(msg.Message),
	})
	return err
}

// --- identity ------------------------------------------------------

type auditedIdentityClient struct {
	inner plugin.IdentityAdapterClient
	aud   *adapterAuditor
	slug  string
	key   string
}

func wrapIdentityClient(aud *adapterAuditor, slug, key string, inner plugin.IdentityAdapterClient) plugin.IdentityAdapterClient {
	if aud == nil || aud.log == nil || inner == nil {
		return inner
	}
	return &auditedIdentityClient{inner: inner, aud: aud, slug: slug, key: key}
}

func (c *auditedIdentityClient) Test(ctx context.Context, config map[string]string) error {
	start := time.Now()
	err := c.inner.Test(ctx, config)
	c.aud.record(ctx, c.slug, actIdentityTest, c.key, start, err, auditAlways, nil)
	return err
}

func (c *auditedIdentityClient) BeginAuth(ctx context.Context, config map[string]string, req plugin.IdentityAuthRequest) (*plugin.IdentityAuthResult, error) {
	start := time.Now()
	res, err := c.inner.BeginAuth(ctx, config, req)
	c.aud.record(ctx, c.slug, actIdentityBeginAuth, c.key, start, err, auditAlways, map[string]any{
		"provider_id": req.ProviderID,
	})
	return res, err
}

func (c *auditedIdentityClient) HandleCallback(ctx context.Context, config map[string]string, req plugin.IdentityCallbackRequest) (*plugin.IdentityCallbackResult, error) {
	start := time.Now()
	res, err := c.inner.HandleCallback(ctx, config, req)
	fields := map[string]any{"provider_id": req.ProviderID}
	if res != nil {
		// The upstream subject is the whole point of an SSO audit row:
		// without it the trail says a login happened but not whose.
		fields["subject"] = res.Subject
		fields["groups"] = len(res.Groups)
	}
	c.aud.record(ctx, c.slug, actIdentityCallback, c.key, start, err, auditAlways, fields)
	return res, err
}

func (c *auditedIdentityClient) FetchGroups(ctx context.Context, config map[string]string) ([]plugin.IdentityGroup, error) {
	start := time.Now()
	groups, err := c.inner.FetchGroups(ctx, config)
	c.aud.record(ctx, c.slug, actIdentityFetchGroups, c.key, start, err, auditOnFailure, map[string]any{"groups": len(groups)})
	return groups, err
}

func (c *auditedIdentityClient) MetadataXML(ctx context.Context, config map[string]string, baseURL string) ([]byte, error) {
	start := time.Now()
	doc, err := c.inner.MetadataXML(ctx, config, baseURL)
	c.aud.record(ctx, c.slug, actIdentityMetadata, c.key, start, err, auditOnFailure, map[string]any{"bytes": len(doc)})
	return doc, err
}

// --- ai ------------------------------------------------------------

type auditedAIClient struct {
	inner plugin.AIAdapterClient
	aud   *adapterAuditor
	slug  string
	key   string
}

func wrapAIClient(aud *adapterAuditor, slug, key string, inner plugin.AIAdapterClient) plugin.AIAdapterClient {
	if aud == nil || aud.log == nil || inner == nil {
		return inner
	}
	return &auditedAIClient{inner: inner, aud: aud, slug: slug, key: key}
}

func (c *auditedAIClient) Test(ctx context.Context, config map[string]string) (*plugin.AITestResult, error) {
	start := time.Now()
	res, err := c.inner.Test(ctx, config)
	fields := map[string]any{}
	if res != nil {
		fields["reachable"] = res.Success
		fields["latency_ms"] = res.LatencyMs
	}
	c.aud.record(ctx, c.slug, actAITest, c.key, start, err, auditAlways, fields)
	return res, err
}

func (c *auditedAIClient) Complete(ctx context.Context, config map[string]string, req plugin.LLMRequest) (*plugin.LLMResponse, error) {
	start := time.Now()
	res, err := c.inner.Complete(ctx, config, req)
	// A completion ships whatever the operator typed to a third party.
	// The row records that it happened and what it cost — never the
	// prompt, the system message, or the reply.
	fields := map[string]any{
		"model":    req.Model,
		"messages": len(req.Messages),
		"tools":    len(req.Tools),
	}
	if res != nil {
		if res.Model != "" {
			fields["model"] = res.Model
		}
		fields["input_tokens"] = res.Usage.InputTokens
		fields["output_tokens"] = res.Usage.OutputTokens
		fields["finish_reason"] = res.FinishReason
		fields["tool_calls"] = len(res.ToolCalls)
	}
	c.aud.record(ctx, c.slug, actAIComplete, c.key, start, err, auditAlways, fields)
	return res, err
}

func (c *auditedAIClient) Models(ctx context.Context, config map[string]string) ([]plugin.ModelDecl, error) {
	start := time.Now()
	models, err := c.inner.Models(ctx, config)
	c.aud.record(ctx, c.slug, actAIModels, c.key, start, err, auditOnFailure, map[string]any{"models": len(models)})
	return models, err
}

// --- snapshot ------------------------------------------------------

type auditedSnapshotClient struct {
	inner plugin.SnapshotAdapterClient
	aud   *adapterAuditor
	slug  string
	key   string
}

func wrapSnapshotClient(aud *adapterAuditor, slug, key string, inner plugin.SnapshotAdapterClient) plugin.SnapshotAdapterClient {
	if aud == nil || aud.log == nil || inner == nil {
		return inner
	}
	return &auditedSnapshotClient{inner: inner, aud: aud, slug: slug, key: key}
}

func (c *auditedSnapshotClient) Capability(ctx context.Context, config map[string]string) (plugin.SnapshotCapability, error) {
	start := time.Now()
	capability, err := c.inner.Capability(ctx, config)
	c.aud.record(ctx, c.slug, actSnapshotCapability, c.key, start, err, auditOnFailure, map[string]any{
		"available": capability.Available,
		"targets":   len(capability.Targets),
	})
	return capability, err
}

func (c *auditedSnapshotClient) Create(ctx context.Context, config map[string]string, req plugin.SnapshotCreateRequest) (plugin.SnapshotRef, error) {
	start := time.Now()
	ref, err := c.inner.Create(ctx, config, req)
	c.aud.record(ctx, c.slug, actSnapshotCreate, c.key, start, err, auditAlways, map[string]any{
		"snapshot_id": req.SnapshotID,
		"targets":     len(req.Targets),
		"snapshot":    ref.SnapshotName,
	})
	return ref, err
}

func (c *auditedSnapshotClient) Remove(ctx context.Context, config map[string]string, ref plugin.SnapshotRef) error {
	start := time.Now()
	err := c.inner.Remove(ctx, config, ref)
	c.aud.record(ctx, c.slug, actSnapshotRemove, c.key, start, err, auditAlways, map[string]any{
		"snapshot": ref.SnapshotName,
		"targets":  len(ref.Targets),
	})
	return err
}

// --- logging -------------------------------------------------------

type auditedLoggingClient struct {
	inner plugin.LogForwarderAdapterClient
	aud   *adapterAuditor
	slug  string
	key   string
}

func wrapLoggingClient(aud *adapterAuditor, slug, key string, inner plugin.LogForwarderAdapterClient) plugin.LogForwarderAdapterClient {
	if aud == nil || aud.log == nil || inner == nil {
		return inner
	}
	return &auditedLoggingClient{inner: inner, aud: aud, slug: slug, key: key}
}

func (c *auditedLoggingClient) Test(ctx context.Context, config map[string]string) error {
	start := time.Now()
	err := c.inner.Test(ctx, config)
	c.aud.record(ctx, c.slug, actLoggingTest, c.key, start, err, auditAlways, nil)
	return err
}

func (c *auditedLoggingClient) ForwardPending(ctx context.Context, config map[string]string) (plugin.LogForwardResult, error) {
	start := time.Now()
	res, err := c.inner.ForwardPending(ctx, config)
	// Failure only, and this one is not a preference. This adapter's
	// job is to ship audit events to a sink; a success row per cycle
	// becomes an event to ship on the next cycle, which writes another
	// row, which becomes another event — a log that grows on its own
	// with the sink working perfectly. A failure row is self-limiting:
	// it stops the moment the sink comes back, and while it is down it
	// is exactly what the operator needs to see.
	c.aud.record(ctx, c.slug, actLoggingForward, c.key, start, err, auditOnFailure, map[string]any{
		"delivered": res.Delivered,
		"failed":    res.Failed,
		"skipped":   res.Skipped,
	})
	return res, err
}

func (c *auditedLoggingClient) DeliveryHistory(ctx context.Context, config map[string]string, limit int) ([]plugin.LogDelivery, error) {
	start := time.Now()
	rows, err := c.inner.DeliveryHistory(ctx, config, limit)
	c.aud.record(ctx, c.slug, actLoggingHistory, c.key, start, err, auditOnFailure, map[string]any{"rows": len(rows)})
	return rows, err
}

// --- alerting ------------------------------------------------------

type auditedAlertingClient struct {
	inner plugin.AlertSourceAdapterClient
	aud   *adapterAuditor
	slug  string
	key   string
}

func wrapAlertingClient(aud *adapterAuditor, slug, key string, inner plugin.AlertSourceAdapterClient) plugin.AlertSourceAdapterClient {
	if aud == nil || aud.log == nil || inner == nil {
		return inner
	}
	return &auditedAlertingClient{inner: inner, aud: aud, slug: slug, key: key}
}

func (c *auditedAlertingClient) Evaluate(ctx context.Context, req plugin.AlertEvaluateRequest) ([]plugin.AlertObservation, error) {
	start := time.Now()
	obs, err := c.inner.Evaluate(ctx, req)
	// Failure only: the alert engine calls this on a ticker, once per
	// (condition, environment) pair. Auditing the successful ticks
	// would outnumber every other row in the log within a day.
	c.aud.record(ctx, c.slug, actAlertingEvaluate, c.key, start, err, auditOnFailure, map[string]any{
		"condition":      req.ConditionKey,
		"environment_id": req.EnvironmentID,
		"observations":   len(obs),
	})
	return obs, err
}

// --- reporting -----------------------------------------------------

type auditedReportingClient struct {
	inner plugin.ReportProviderClient
	aud   *adapterAuditor
	slug  string
	key   string
}

func wrapReportingClient(aud *adapterAuditor, slug, key string, inner plugin.ReportProviderClient) plugin.ReportProviderClient {
	if aud == nil || aud.log == nil || inner == nil {
		return inner
	}
	return &auditedReportingClient{inner: inner, aud: aud, slug: slug, key: key}
}

func (c *auditedReportingClient) Build(ctx context.Context, req plugin.ReportRequest) (plugin.ReportResult, error) {
	start := time.Now()
	res, err := c.inner.Build(ctx, req)
	fields := map[string]any{
		"report_type":    req.TypeKey,
		"environment_id": req.EnvironmentID,
		"requested_by":   req.User,
		"sections":       len(res.Sections),
	}
	// A provider may succeed with sources that did not answer. That is
	// a partial document, and the row says so rather than reporting a
	// clean success the reader would trust more than the PDF deserves.
	degraded := 0
	for _, s := range res.Sources {
		if !s.OK {
			degraded++
		}
	}
	fields["sources_failed"] = degraded
	c.aud.record(ctx, c.slug, actReportingBuild, c.key, start, err, auditAlways, fields)
	return res, err
}

func (c *auditedReportingClient) Subjects(ctx context.Context, kind string, req plugin.ReportRequest) ([]plugin.ReportSubjectOption, error) {
	start := time.Now()
	subjects, err := c.inner.Subjects(ctx, kind, req)
	c.aud.record(ctx, c.slug, actReportingSubjects, c.key, start, err, auditOnFailure, map[string]any{
		"kind":     kind,
		"subjects": len(subjects),
	})
	return subjects, err
}

// --- scanner -------------------------------------------------------

type auditedScannerClient struct {
	inner plugin.ImageScannerAdapterClient
	aud   *adapterAuditor
	slug  string
	key   string
}

// auditedRicherScannerClient preserves the OPTIONAL RicherScannerTester
// contract across wrapping, for the reason auditedRicherStorageClient
// exists: modules/scans type-asserts the client to pick the stepped test.
type auditedRicherScannerClient struct {
	auditedScannerClient
	rich plugin.RicherScannerTester
}

func wrapScannerClient(aud *adapterAuditor, slug, key string, inner plugin.ImageScannerAdapterClient) plugin.ImageScannerAdapterClient {
	if aud == nil || aud.log == nil || inner == nil {
		return inner
	}
	base := auditedScannerClient{inner: inner, aud: aud, slug: slug, key: key}
	if rich, ok := inner.(plugin.RicherScannerTester); ok {
		return &auditedRicherScannerClient{auditedScannerClient: base, rich: rich}
	}
	return &base
}

// Available is deliberately not audited: the Scanners tab calls it on
// every render and the contract forbids it from touching the network,
// so there is no outcome to record.
func (c *auditedScannerClient) Available(ctx context.Context, config map[string]string) bool {
	return c.inner.Available(ctx, config)
}

func (c *auditedScannerClient) Test(ctx context.Context, config map[string]string) error {
	start := time.Now()
	err := c.inner.Test(ctx, config)
	c.aud.record(ctx, c.slug, actScannerTest, c.key, start, err, auditAlways, nil)
	return err
}

func (c *auditedRicherScannerClient) TestSteps(ctx context.Context, config map[string]string) ([]plugin.ScannerTestStep, error) {
	start := time.Now()
	steps, err := c.rich.TestSteps(ctx, config)
	c.aud.record(ctx, c.slug, actScannerTest, c.key, start, err, auditAlways, map[string]any{"steps": len(steps)})
	return steps, err
}

func (c *auditedScannerClient) Scan(ctx context.Context, config map[string]string, imageRef string) (plugin.ScanReport, error) {
	start := time.Now()
	report, err := c.inner.Scan(ctx, config, imageRef)
	c.aud.record(ctx, c.slug, actScannerScan, c.key, start, err, auditAlways, map[string]any{
		"image":    imageRef,
		"findings": len(report.Findings),
	})
	return report, err
}
