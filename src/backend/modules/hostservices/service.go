// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package hostservices

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"
	"unicode/utf8"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/selfhost"
)

// Server-side bounds on each agent round trip. Each is a little longer than
// the agent's own budget, so the agent's answer wins the race and the operator
// sees systemd's reason rather than a bare timeout.
const (
	listTimeout     = 60 * time.Second
	actionTimeout   = 110 * time.Second
	overrideTimeout = 90 * time.Second
	reloadTimeout   = 45 * time.Second
	// maxUnits bounds the list the server hands to the browser.
	maxUnits = 5000
)

// hostCaller is the one operation every managed host serves: a native agent
// over its WebSocket, or McHarbor's own machine through the self-host runner.
type hostCaller interface {
	HostService(ctx context.Context, payload coreagent.HostServicePayload) (*coreagent.HostServicePayload, error)
}

// agentLink is the slice of the agent transport this service uses, so tests
// substitute a scripted agent for a WebSocket.
type agentLink interface {
	hostCaller
	HostServicesSupported() bool
	HostServicesVersion() (string, string)
	HostServicesCapable() (capable, known bool)
}

// Target names the host a request acts on: the host behind an environment's
// native agent, or the machine McHarbor itself is installed on.
type Target struct {
	EnvID string
	Self  bool
}

// EnvTarget targets the host behind envID's native agent.
func EnvTarget(envID string) Target { return Target{EnvID: envID} }

// SelfTarget targets the machine McHarbor runs on, which no environment
// selector changes.
func SelfTarget() Target { return Target{Self: true} }

// protection is the guard that applies on the target's host.
func (t Target) protection(unit string) string {
	if t.Self {
		return SelfProtection(unit)
	}
	return Protection(unit)
}

// Refusal is a deliberate "no" that maps to one i18n code and HTTP status.
// Detail is systemd's own text when the host refused, for the operator.
type Refusal struct {
	Status int
	Code   i18n.MsgCode
	Detail string
	// unreachable marks the self host refusing because McHarbor could not reach
	// it at all; Detail then carries the reason the host updates tab also shows.
	unreachable bool
}

// Error renders the refusal for the server log.
func (r *Refusal) Error() string {
	if r.Detail != "" {
		return string(r.Code) + ": " + r.Detail
	}
	return string(r.Code)
}

// Service talks to a native agent, or to McHarbor's own machine, about a
// host's systemd units.
type Service struct {
	db      *sql.DB
	resolve func(envID string) agentLink
	self    hostCaller
}

// NewService binds the service to the agent pool for environment hosts and to
// the system module's self-host runner for the machine McHarbor runs on. A
// relayed agent connection is used like a local one.
func NewService(db *sql.DB, pool *coreagent.AgentPool, sys selfhost.Runner) *Service {
	svc := &Service{db: db, resolve: func(envID string) agentLink {
		if pool == nil {
			return nil
		}
		t := pool.ResolveTransport(envID)
		if t == nil {
			return nil
		}
		return t
	}}
	if sys != nil {
		svc.self = newSelfLink(sys)
	}
	return svc
}

// ErrEnvironmentNotFound is returned when envID names no environment.
var ErrEnvironmentNotFound = errors.New("environment not found")

// Availability works out whether envID's host can be managed, in the order an
// operator would fix it: an agent at all, a native one, connected, new enough,
// and on a host that actually runs systemd.
func (s *Service) Availability(ctx context.Context, envID string) (Availability, agentLink, error) {
	avail := Availability{MinVersion: coreagent.HostServicesMinVersion, MinBuild: coreagent.HostServicesMinBuild}
	var connType string
	var installKind sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT connection_type, agent_install_kind FROM environments WHERE id = ?`, envID,
	).Scan(&connType, &installKind)
	if errors.Is(err, sql.ErrNoRows) {
		return avail, nil, ErrEnvironmentNotFound
	}
	if err != nil {
		return avail, nil, fmt.Errorf("reading environment %s: %w", envID, err)
	}
	avail.InstallKind = installKind.String
	if connType != "agent" {
		avail.Reason = "not_agent"
		return avail, nil, nil
	}
	if installKind.String != "" && installKind.String != "native" {
		avail.Reason = "not_native"
		return avail, nil, nil
	}
	link := s.resolve(envID)
	if link == nil {
		avail.Reason = "offline"
		return avail, nil, nil
	}
	avail.AgentVersion, avail.AgentBuild = link.HostServicesVersion()
	if !link.HostServicesSupported() {
		avail.Reason = "agent_outdated"
		return avail, nil, nil
	}
	if capable, known := link.HostServicesCapable(); known && !capable {
		avail.Reason = "no_systemd"
		return avail, nil, nil
	}
	avail.Supported = true
	return avail, link, nil
}

// require returns the caller for the target's host or the refusal that
// explains its absence. The self host is reached on demand, so whether it is
// reachable is only learned from the call itself.
func (s *Service) require(ctx context.Context, target Target) (hostCaller, error) {
	if target.Self {
		if s.self == nil {
			return nil, &Refusal{Status: http.StatusConflict, Code: i18n.ErrHostServicesUnsupported, unreachable: true}
		}
		return s.self, nil
	}
	avail, link, err := s.Availability(ctx, target.EnvID)
	if err != nil {
		return nil, err
	}
	if link != nil {
		return link, nil
	}
	switch avail.Reason {
	case "offline":
		return nil, &Refusal{Status: http.StatusConflict, Code: i18n.ErrHostServicesAgentOffline}
	case "agent_outdated":
		return nil, &Refusal{Status: http.StatusConflict, Code: i18n.ErrHostServicesAgentOutdated}
	}
	return nil, &Refusal{Status: http.StatusConflict, Code: i18n.ErrHostServicesUnsupported}
}

// call sends one request and turns the host's own refusal codes into
// Refusals, so a handler never has to read the host's English.
func call(ctx context.Context, link hostCaller, timeout time.Duration, req coreagent.HostServicePayload, failCode i18n.MsgCode) (*coreagent.HostServicePayload, error) {
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	reply, err := link.HostService(callCtx, req)
	if err != nil {
		return nil, fmt.Errorf("host service %s %s: %w", req.Op, req.Unit, err)
	}
	if reply == nil {
		return nil, fmt.Errorf("host service %s %s: empty reply", req.Op, req.Unit)
	}
	if reply.Success {
		return reply, nil
	}
	switch reply.ErrorCode {
	case "invalid_unit":
		return nil, &Refusal{Status: http.StatusBadRequest, Code: i18n.ErrHostServicesInvalidUnit}
	case "invalid_action":
		return nil, &Refusal{Status: http.StatusBadRequest, Code: i18n.ErrHostServicesInvalidAction}
	case "invalid_content":
		return nil, &Refusal{Status: http.StatusBadRequest, Code: i18n.ErrHostServicesInvalidContent, Detail: reply.Error}
	case "protected":
		return nil, &Refusal{Status: http.StatusConflict, Code: i18n.ErrHostServicesProtected, Detail: reply.Error}
	case "unsupported":
		return nil, &Refusal{Status: http.StatusConflict, Code: i18n.ErrHostServicesUnsupported, Detail: reply.Error}
	}
	return nil, &Refusal{Status: http.StatusConflict, Code: failCode, Detail: reply.Error}
}

// List returns every service unit on the target's host with the guard's
// verdict. A host that cannot be managed answers with its Availability and no
// units, so the table explains itself instead of failing.
func (s *Service) List(ctx context.Context, target Target) (*UnitList, error) {
	if target.Self {
		return s.listSelf(ctx)
	}
	avail, link, err := s.Availability(ctx, target.EnvID)
	if err != nil {
		return nil, err
	}
	resp := &UnitList{Availability: avail, Units: []Unit{}}
	if link == nil {
		return resp, nil
	}
	reply, err := call(ctx, link, listTimeout, coreagent.HostServicePayload{Op: coreagent.HostServiceOpList}, i18n.ErrHostServicesListFailed)
	if err != nil {
		return nil, err
	}
	fillUnits(resp, reply, target)
	return resp, nil
}

// listSelf lists McHarbor's own host. Whether it is reachable, and runs
// systemd, is only known once the list has been attempted.
func (s *Service) listSelf(ctx context.Context) (*UnitList, error) {
	resp := &UnitList{Units: []Unit{}}
	link, err := s.require(ctx, SelfTarget())
	if err == nil {
		var reply *coreagent.HostServicePayload
		reply, err = call(ctx, link, listTimeout, coreagent.HostServicePayload{Op: coreagent.HostServiceOpList}, i18n.ErrHostServicesListFailed)
		if err == nil {
			resp.Availability.Supported = true
			fillUnits(resp, reply, SelfTarget())
			return resp, nil
		}
	}
	var refusal *Refusal
	if errors.As(err, &refusal) {
		switch {
		case refusal.unreachable:
			resp.Availability.Reason = "self_unreachable"
			resp.Availability.Detail = refusal.Detail
			return resp, nil
		case refusal.Code == i18n.ErrHostServicesUnsupported:
			resp.Availability.Reason = "self_no_systemd"
			return resp, nil
		}
	}
	return nil, err
}

// fillUnits copies the host's units into resp with the target's guard verdict,
// dropping any name the server would itself refuse to act on.
func fillUnits(resp *UnitList, reply *coreagent.HostServicePayload, target Target) {
	resp.Truncated = reply.Truncated
	for i, u := range reply.Units {
		if i >= maxUnits {
			resp.Truncated = true
			break
		}
		if ValidateUnit(u.Name) != nil {
			continue
		}
		resp.Units = append(resp.Units, Unit{
			Name: u.Name, Description: u.Description, LoadState: u.LoadState, ActiveState: u.ActiveState,
			SubState: u.SubState, UnitFileState: u.UnitFileState, MainPID: u.MainPID, Result: u.Result,
			FailureReason: u.FailureReason, Template: u.Template,
			Protected:        target.protection(u.Name),
			IsAgent:          !target.Self && IsAgentUnit(u.Name),
			RestartsMcHarbor: target.Self && RestartsMcHarbor(u.Name),
		})
	}
}

// Action runs start, stop, restart, enable or disable. The guard is checked
// here before the host is contacted, and again by the host side.
func (s *Service) Action(ctx context.Context, target Target, unit, action string) (*ActionResult, error) {
	if err := ValidateUnit(unit); err != nil {
		return nil, &Refusal{Status: http.StatusBadRequest, Code: i18n.ErrHostServicesInvalidUnit}
	}
	if !ValidAction(action) {
		return nil, &Refusal{Status: http.StatusBadRequest, Code: i18n.ErrHostServicesInvalidAction}
	}
	if actionRefusedBy(target.protection, unit, action) {
		return nil, &Refusal{Status: http.StatusConflict, Code: i18n.ErrHostServicesProtected}
	}
	link, err := s.require(ctx, target)
	if err != nil {
		return nil, err
	}
	reply, err := call(ctx, link, actionTimeout,
		coreagent.HostServicePayload{Op: coreagent.HostServiceOpAction, Unit: unit, Action: action},
		i18n.ErrHostServicesActionFailed)
	if err != nil {
		return nil, err
	}
	return &ActionResult{Unit: unit, Action: action, Warning: reply.Warning}, nil
}

// Override returns the effective unit (systemctl cat) and the drop-in McHarbor
// manages, which is empty and Exists=false until the first save.
func (s *Service) Override(ctx context.Context, target Target, unit string) (*Override, error) {
	if err := ValidateUnit(unit); err != nil {
		return nil, &Refusal{Status: http.StatusBadRequest, Code: i18n.ErrHostServicesInvalidUnit}
	}
	link, err := s.require(ctx, target)
	if err != nil {
		return nil, err
	}
	reply, err := call(ctx, link, listTimeout,
		coreagent.HostServicePayload{Op: coreagent.HostServiceOpOverrideGet, Unit: unit},
		i18n.ErrHostServicesOverrideFailed)
	if err != nil {
		return nil, err
	}
	return &Override{Unit: unit, Effective: reply.Effective, Content: reply.Override, Exists: reply.OverrideExists,
		Path: reply.OverridePath, Truncated: reply.Truncated}, nil
}

// ContentHash is the sha256 of an override, recorded in the audit row so a
// later reader can tell which text was written without storing the text.
func ContentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// SaveOverride writes the drop-in atomically on the host, then reloads and
// verifies as asked.
func (s *Service) SaveOverride(ctx context.Context, target Target, unit string, req OverrideRequest) (*OverrideResult, error) {
	if err := ValidateUnit(unit); err != nil {
		return nil, &Refusal{Status: http.StatusBadRequest, Code: i18n.ErrHostServicesInvalidUnit}
	}
	if len(req.Content) > overrideMaxBytes || !utf8.ValidString(req.Content) {
		return nil, &Refusal{Status: http.StatusBadRequest, Code: i18n.ErrHostServicesInvalidContent}
	}
	link, err := s.require(ctx, target)
	if err != nil {
		return nil, err
	}
	reply, err := call(ctx, link, overrideTimeout, coreagent.HostServicePayload{
		Op: coreagent.HostServiceOpOverridePut, Unit: unit, Content: req.Content, Reload: req.Reload, Verify: req.Verify,
	}, i18n.ErrHostServicesOverrideFailed)
	if err != nil {
		return nil, err
	}
	return &OverrideResult{Unit: unit, Path: reply.OverridePath, Reloaded: reply.Reloaded,
		VerifyOutput: reply.VerifyOutput, SHA256: ContentHash(req.Content)}, nil
}

// RevertOverride removes the drop-in, and its directory when empty, and
// reloads systemd.
func (s *Service) RevertOverride(ctx context.Context, target Target, unit string) error {
	if err := ValidateUnit(unit); err != nil {
		return &Refusal{Status: http.StatusBadRequest, Code: i18n.ErrHostServicesInvalidUnit}
	}
	link, err := s.require(ctx, target)
	if err != nil {
		return err
	}
	_, err = call(ctx, link, overrideTimeout,
		coreagent.HostServicePayload{Op: coreagent.HostServiceOpOverrideDel, Unit: unit},
		i18n.ErrHostServicesOverrideFailed)
	return err
}

// DaemonReload runs systemctl daemon-reload on the target's host.
func (s *Service) DaemonReload(ctx context.Context, target Target) error {
	link, err := s.require(ctx, target)
	if err != nil {
		return err
	}
	_, err = call(ctx, link, reloadTimeout,
		coreagent.HostServicePayload{Op: coreagent.HostServiceOpDaemonReload},
		i18n.ErrHostServicesReloadFailed)
	return err
}
