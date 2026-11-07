// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updates

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/xid"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/redact"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/updatechannel"
	appversion "github.com/therealmcsparrow/mcharbor/core/version"
	"github.com/therealmcsparrow/mcharbor/core/versionorder"
	"github.com/therealmcsparrow/mcharbor/modules/agentreleases"
)

// The pin rows. modules/agent reads the same two keys to decide who may fetch a
// version-specific binary, and cannot import this package, so the names must
// match its release_source.go. Settings keys are unique across categories.
const (
	pinsCategory = updatechannel.PinsCategory
	pinListKey   = updatechannel.PinListKey
	pinStateKey  = updatechannel.PinStateKey

	// returnReasonTimeout bounds the write that records why a move failed, which
	// runs on a context of its own so a gone caller cannot lose the reason.
	returnReasonTimeout = 10 * time.Second
)

// envIDPattern bounds the {envId} path segment before it reaches a query.
var envIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

var (
	errReturnNotConnected  = errors.New("the agent is not connected")
	errReturnUnsupported   = errors.New("the agent is too old to update itself")
	errReturnNoStable      = errors.New("no stable release holds a build for this agent")
	errReturnNotPrerelease = errors.New("the agent is not running a prerelease")
	errReturnUnknownKind   = errors.New("the agent has not reported whether it runs natively or in a container")
)

// pinRecord is what the state row keeps per pinned agent: the stable version it
// was told to return to, the redacted reason its last move failed, and whether
// the pin added the exclusion (so removal never drops an operator's own).
type pinRecord struct {
	Version       string `json:"version"`
	Reason        string `json:"reason,omitempty"`
	ExcludedByPin bool   `json:"excludedByPin,omitempty"`
}

// settingsExec is the part of *sql.DB and *sql.Tx the pin rows use, so one
// helper serves both a read and a transaction.
type settingsExec interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// agentPins keeps the pin rows. A pin and its exclusion always change in one
// transaction, so a crash never leaves an agent pinned but swept.
type agentPins struct{ db *sql.DB }

func readSetting(ctx context.Context, q settingsExec, key string) (string, error) {
	var value string
	err := q.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading setting %s: %w", key, err)
	}
	return value, nil
}

func writeSetting(ctx context.Context, q settingsExec, category, key, value string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := q.ExecContext(ctx,
		`INSERT INTO settings (id, category, key, value, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, category = excluded.category`,
		xid.New().String(), category, key, value, now, now); err != nil {
		return fmt.Errorf("writing setting %s: %w", key, err)
	}
	return nil
}

func readPinState(ctx context.Context, q settingsExec) (ids []string, state map[string]pinRecord, err error) {
	rawList, err := readSetting(ctx, q, pinListKey)
	if err != nil {
		return nil, nil, err
	}
	rawState, err := readSetting(ctx, q, pinStateKey)
	if err != nil {
		return nil, nil, err
	}
	ids = normalizeIDList(decodeStringList(rawList))
	state = map[string]pinRecord{}
	if rawState != "" {
		if err := json.Unmarshal([]byte(rawState), &state); err != nil {
			return nil, nil, fmt.Errorf("decoding agent return state: %w", err)
		}
	}
	return ids, state, nil
}

func writePinState(ctx context.Context, q settingsExec, ids []string, state map[string]pinRecord) error {
	if err := writeSetting(ctx, q, pinsCategory, pinListKey, encodeStringList(ids)); err != nil {
		return err
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encoding agent return state: %w", err)
	}
	return writeSetting(ctx, q, pinsCategory, pinStateKey, string(raw))
}

// IDs lists the pinned environments.
func (p agentPins) IDs(ctx context.Context) ([]string, error) {
	ids, _, err := readPinState(ctx, p.db)
	return ids, err
}

// Records returns each pinned environment's record.
func (p agentPins) Records(ctx context.Context) (map[string]pinRecord, error) {
	ids, state, err := readPinState(ctx, p.db)
	if err != nil {
		return nil, err
	}
	out := make(map[string]pinRecord, len(ids))
	for _, id := range ids {
		out[id] = state[id]
	}
	return out, nil
}

func (p agentPins) tx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	return coredb.RetryOnBusy(ctx, 5, func() error {
		tx, err := p.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("beginning pin change: %w", err)
		}
		defer func() { _ = tx.Rollback() }() //nolint:errcheck // rollback after a commit returns ErrTxDone by design
		if err := fn(tx); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing pin change: %w", err)
		}
		return nil
	})
}

// pinAdd is what Add reports for the audit row: whether it added the exclusion,
// and the pin that was already there, if any.
type pinAdd struct {
	AddedExclusion bool
	WasPinned      bool
	PrevVersion    string
}

// Add pins envID to version and excludes it from sweeps in one transaction, and
// clears any earlier failure reason. It reports what it changed, which is what
// the audit row records.
func (p agentPins) Add(ctx context.Context, envID, version string) (out pinAdd, err error) {
	err = p.tx(ctx, func(tx *sql.Tx) error {
		ids, state, err := readPinState(ctx, tx)
		if err != nil {
			return err
		}
		rawExcl, err := readSetting(ctx, tx, selfUpdateAgentExclKey)
		if err != nil {
			return err
		}
		excl := normalizeIDList(decodeStringList(rawExcl))
		rec, hadRecord := state[envID]
		out = pinAdd{WasPinned: containsID(ids, envID)}
		if out.WasPinned {
			out.PrevVersion = rec.Version
		}
		if !hadRecord || !out.WasPinned {
			rec = pinRecord{ExcludedByPin: !containsID(excl, envID)}
		}
		out.AddedExclusion = !containsID(excl, envID)
		rec.Version, rec.Reason = version, ""
		state[envID] = rec
		if !containsID(ids, envID) {
			ids = append(ids, envID)
		}
		if out.AddedExclusion {
			excl = append(excl, envID)
		}
		if err := writePinState(ctx, tx, ids, state); err != nil {
			return err
		}
		return writeSetting(ctx, tx, selfUpdateCategory, selfUpdateAgentExclKey, encodeStringList(excl))
	})
	return out, err
}

// SetReason records why the last move failed, for a pinned agent only.
func (p agentPins) SetReason(ctx context.Context, envID, reason string) error {
	return p.tx(ctx, func(tx *sql.Tx) error {
		ids, state, err := readPinState(ctx, tx)
		if err != nil {
			return err
		}
		rec, ok := state[envID]
		if !ok || !containsID(ids, envID) {
			return nil
		}
		rec.Reason = redact.ErrorDetail(reason)
		state[envID] = rec
		return writePinState(ctx, tx, ids, state)
	})
}

// Remove releases the pin and, when the pin added it, the exclusion, in one
// transaction. It returns the record it removed and whether there was one.
func (p agentPins) Remove(ctx context.Context, envID string) (old pinRecord, wasPinned bool, err error) {
	err = p.tx(ctx, func(tx *sql.Tx) error {
		ids, state, err := readPinState(ctx, tx)
		if err != nil {
			return err
		}
		if !containsID(ids, envID) {
			return nil
		}
		wasPinned, old = true, state[envID]
		delete(state, envID)
		kept := make([]string, 0, len(ids))
		for _, id := range ids {
			if id != envID {
				kept = append(kept, id)
			}
		}
		if err := writePinState(ctx, tx, kept, state); err != nil {
			return err
		}
		if !old.ExcludedByPin {
			return nil
		}
		rawExcl, err := readSetting(ctx, tx, selfUpdateAgentExclKey)
		if err != nil {
			return err
		}
		var excl []string
		for _, id := range normalizeIDList(decodeStringList(rawExcl)) {
			if id != envID {
				excl = append(excl, id)
			}
		}
		return writeSetting(ctx, tx, selfUpdateCategory, selfUpdateAgentExclKey, encodeStringList(excl))
	})
	return old, wasPinned, err
}

func containsID(list []string, id string) bool {
	for _, item := range list {
		if item == id {
			return true
		}
	}
	return false
}

// returnSource is the slice of the release store a return reads.
type returnSource interface {
	NewestStable(ctx context.Context, osName, arch string, wantImage bool) (agentreleases.ReturnCandidate, bool)
}

// ReturnResult describes one agent return. Pinned is true once the pin row is
// written, even when the move after it failed.
type ReturnResult struct {
	EnvID          string
	From           string
	To             string
	Build          string
	Pinned         bool
	AddedExclusion bool
	// WasPinned and PrevPin are the pin this return replaced, for the audit row.
	WasPinned bool
	PrevPin   string
}

// agentRef is the part of a live connection a return reads, copied out because
// AgentConnection holds locks.
type agentRef struct {
	EnvID, OS, Arch, Version, Build, ServerURL string
}

func (u *AgentUpdater) connection(envID string) (agentRef, bool) {
	if u.pool == nil {
		return agentRef{}, false
	}
	agents := u.pool.List()
	for i := range agents {
		if c := &agents[i]; c.EnvID == envID {
			return agentRef{EnvID: c.EnvID, OS: c.OS, Arch: c.Arch, Version: c.Version, Build: c.Build, ServerURL: c.ServerURL}, true
		}
	}
	return agentRef{}, false
}

func (u *AgentUpdater) hostVer() string {
	if u.hostVersion != nil {
		return u.hostVersion()
	}
	return appversion.Current()
}

// returnTarget picks the newest stable McHarbor can serve this platform: a ready
// store release holding the asset (an image for a container agent), or the
// bundled binary when this host is itself stable and no store release is newer.
func (u *AgentUpdater) returnTarget(ctx context.Context, osName, arch string, wantImage bool) (agentreleases.ReturnCandidate, bool) {
	var best agentreleases.ReturnCandidate
	found := false
	if u.returns != nil {
		best, found = u.returns.NewestStable(ctx, osName, arch, wantImage)
	}
	host := u.hostVer()
	if wantImage || versionorder.IsPrerelease(host) || (found && versionorder.Compare(host, best.Version) <= 0) {
		return best, found
	}
	if _, err := versionorder.Parse(host); err != nil {
		return best, found
	}
	// Not AgentBinaryPath: that prefers the selected release, which may be the
	// prerelease, while the version route serves this host's own bundled file.
	sum, err := u.bins.BundledBinaryDigest(osName, arch)
	if err != nil || sum == "" {
		u.logger.Warn("agent-return: hashing the bundled binary", "os", osName, "arch", arch, "error", err)
		return best, found
	}
	return agentreleases.ReturnCandidate{Version: host, BinarySHA: sum}, true
}

// returnPayload builds the update request for a candidate. Its URLs name the
// exact version, because the plain routes serve the global target, which may be
// the prerelease the agent is leaving.
func returnPayload(conn agentRef, c agentreleases.ReturnCandidate, wantImage bool) coreagent.AgentUpdatePayload {
	base := strings.TrimRight(strings.TrimSpace(conn.ServerURL), "/")
	suffix := conn.OS + "/" + conn.Arch + "/" + c.Version
	p := coreagent.AgentUpdatePayload{Version: c.Version, Build: c.Build}
	if c.BinarySHA != "" {
		p.BinaryURL = base + "/api/agent/binary/" + suffix
		p.BinarySHA256 = c.BinarySHA
	}
	if wantImage && c.ImageSHA != "" {
		p.ImageURL = base + "/api/agent/image/" + suffix
		p.ImageSHA256 = c.ImageSHA
		p.ImageRef = c.ImageRef
	}
	return p
}

func (u *AgentUpdater) sendMove(ctx context.Context, envID string, p coreagent.AgentUpdatePayload) (*coreagent.AgentUpdatePayload, error) {
	if u.send != nil {
		return u.send(ctx, envID, p)
	}
	if u.pool == nil {
		return nil, errReturnNotConnected
	}
	transport := u.pool.Transport(envID)
	if transport == nil {
		return nil, errReturnNotConnected
	}
	return transport.AgentUpdate(ctx, p)
}

// Return moves one connected agent running a prerelease to its platform's newest
// stable and pins it there. Every refusal happens before anything is written; once
// the pin is written a failed move keeps it, with its redacted reason, because
// removing it would let the next sweep move the agent straight back.
func (u *AgentUpdater) Return(ctx context.Context, db *sql.DB, envID string) (ReturnResult, error) {
	res := ReturnResult{EnvID: envID}
	conn, ok := u.connection(envID)
	if !ok {
		return res, errReturnNotConnected
	}
	res.From = conn.Version
	if !coreagent.AgentVersionAtLeast(conn.Version, coreagent.AgentUpdateMinVersion) {
		return res, errReturnUnsupported
	}
	if !versionorder.IsPrerelease(conn.Version) {
		return res, errReturnNotPrerelease
	}
	var kind string
	err := db.QueryRowContext(ctx,
		`SELECT COALESCE(agent_install_kind, '') FROM environments WHERE id = ? AND connection_type = 'agent'`, envID).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		return res, errReturnNotConnected
	}
	if err != nil {
		return res, fmt.Errorf("reading the agent install kind: %w", err)
	}
	// An empty kind is an agent that never reported its install mode. A container agent cannot take
	// a binary, so guessing native would pin it and then fail; refuse before the pin is written.
	if kind == "" {
		return res, errReturnUnknownKind
	}
	wantImage := kind == "container"
	if wantImage && !coreagent.SupportsImageLoad(conn.Version, conn.Build) {
		return res, errReturnNoStable
	}
	cand, ok := u.returnTarget(ctx, conn.OS, conn.Arch, wantImage)
	if !ok {
		return res, errReturnNoStable
	}
	res.To, res.Build = cand.Version, cand.Build

	pins := agentPins{db: db}
	added, err := pins.Add(ctx, envID, cand.Version)
	if err != nil {
		return res, fmt.Errorf("writing the pin: %w", err)
	}
	res.Pinned, res.AddedExclusion, res.WasPinned, res.PrevPin = true, added.AddedExclusion, added.WasPinned, added.PrevVersion

	label := targetLabel(cand.Version, cand.Build)
	u.setRun(envID, AgentRunState{Status: "updating", Target: label})
	payload := returnPayload(conn, cand, wantImage)
	// The agent answers only after its download, which can take as long as a
	// sweep's wait, so the request does not hold on for it.
	bg := context.WithoutCancel(ctx)
	u.returnWG.Add(1)
	go func() { // goroutine lifecycle: ends when the send returns or agentUpdateAckTimeout expires; returnWG tracks it.
		defer u.returnWG.Done()
		u.finishReturn(bg, pins, envID, cand.Version, label, payload)
	}()
	return res, nil
}

// finishReturn sends the move and records the outcome. A failure keeps the pin
// with its redacted reason; both writes use contexts that outlive the request.
func (u *AgentUpdater) finishReturn(ctx context.Context, pins agentPins, envID, version, label string, p coreagent.AgentUpdatePayload) {
	sendCtx, cancel := context.WithTimeout(ctx, agentUpdateAckTimeout)
	defer cancel()
	reply, err := u.sendMove(sendCtx, envID, p)
	if err == nil && reply == nil {
		err = errors.New("the agent sent no reply")
	}
	if err == nil && reply.Mode == "" {
		err = errors.New("the agent reported it was already current and downloaded nothing")
	}
	if err != nil {
		u.logger.Warn("agent-return: the move failed", "env", envID, "error", err)
		u.setRun(envID, AgentRunState{Status: "failed", Error: redact.ErrorDetail(err.Error()), Target: label})
		reasonCtx, stop := context.WithTimeout(ctx, returnReasonTimeout)
		defer stop()
		if rerr := pins.SetReason(reasonCtx, envID, err.Error()); rerr != nil {
			u.logger.Error("agent-return: recording the failure reason", "env", envID, "error", rerr)
		}
		return
	}
	u.markAttempt(envID, version)
	u.setRun(envID, AgentRunState{Status: "started", Mode: reply.Mode, Target: label})
}

// WaitReturns blocks until every agent return in flight has finished.
func (u *AgentUpdater) WaitReturns() { u.returnWG.Wait() }

// HandleAgentReturn serves POST /updates/agents/{envId}/return.
func (h *Handler) HandleAgentReturn(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	envID := chi.URLParam(r, "envId")
	if !envIDPattern.MatchString(envID) {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	if h.selfUpdate == nil || h.selfUpdate.agentUpdate == nil {
		response.InternalErrorCode(w, r, i18n.ErrAgentUpdateFailed)
		return
	}
	res, err := h.selfUpdate.agentUpdate.Return(r.Context(), h.app.DB, envID)
	if res.Pinned {
		h.auditReturn(r, user.Username, res)
	}
	switch {
	case err == nil:
		response.OK(w, map[string]any{"envId": envID, "from": res.From, "to": res.To, "build": res.Build, "pinned": true, "started": true})
	case errors.Is(err, errReturnNotConnected):
		response.ConflictCause(w, r, i18n.ErrAgentNotConnected, err)
	case errors.Is(err, errReturnNoStable):
		response.ConflictCause(w, r, i18n.ErrAgentNoStableRelease, err)
	case errors.Is(err, errReturnUnsupported), errors.Is(err, errReturnNotPrerelease), errors.Is(err, errReturnUnknownKind):
		response.ConflictCause(w, r, i18n.ErrAgentUpdateFailed, err)
	default:
		h.app.Logger.Error("agent return failed", "env", envID, "pinned", res.Pinned, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrAgentUpdateFailed, err)
	}
}

func (h *Handler) auditReturn(r *http.Request, username string, res ReturnResult) {
	outcome := "move requested"
	beforeState := map[string]any{"version": res.From, "pinned": res.WasPinned}
	if res.WasPinned {
		beforeState["pinnedVersion"] = res.PrevPin
	}
	before, errB := json.Marshal(beforeState)
	after, errA := json.Marshal(map[string]any{"version": res.To, "pinned": true, "exclusionAdded": res.AddedExclusion})
	if errB != nil || errA != nil {
		h.app.Logger.Error("agent return: marshal audit state", "before", errB, "after", errA)
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "agent_return",
		EntityType:    "agent",
		EntityID:      res.EnvID,
		EntityName:    "mcharbor-agent",
		EnvironmentID: res.EnvID,
		Details:       fmt.Sprintf("Agent returned to stable by %s: %s to %s (%s), pinned and excluded from sweeps (exclusion added: %t)", username, res.From, res.To, outcome, res.AddedExclusion),
		StateBefore:   string(before),
		StateAfter:    string(after),
		ChangedKeys:   "version,pinned,agentExcludedEnvIds",
	})
}

// HandleAgentPinRemove serves DELETE /updates/agents/{envId}/pin: the only way
// to release a pin, which lets sweeps move that agent again.
func (h *Handler) HandleAgentPinRemove(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	envID := chi.URLParam(r, "envId")
	if !envIDPattern.MatchString(envID) {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	old, was, err := agentPins{db: h.app.DB}.Remove(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("agent pin removal failed", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrAgentUpdateFailed, err)
		return
	}
	if !was {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	before, errB := json.Marshal(map[string]any{"pinned": true, "version": old.Version, "excluded": true, "reason": old.Reason})
	after, errA := json.Marshal(map[string]any{"pinned": false, "excluded": !old.ExcludedByPin})
	if errB != nil || errA != nil {
		h.app.Logger.Error("agent pin removal: marshal audit state", "before", errB, "after", errA)
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "agent_pin_remove",
		EntityType:    "agent",
		EntityID:      envID,
		EntityName:    "mcharbor-agent",
		EnvironmentID: envID,
		Details:       fmt.Sprintf("Agent pin on %s removed by %s (exclusion removed: %t)", old.Version, user.Username, old.ExcludedByPin),
		StateBefore:   string(before),
		StateAfter:    string(after),
		ChangedKeys:   "pinned,agentExcludedEnvIds",
	})
	response.OK(w, map[string]any{"envId": envID, "pinned": false})
}
