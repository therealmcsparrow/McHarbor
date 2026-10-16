// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
)

// stackSelfTarget is the Refusal target for a stack that runs McHarbor.
const stackSelfTarget = "stack"

// TargetsMcHarbor reports whether the stack called name runs a McHarbor container (this
// node, a peer or a helper) on its environment, by the managed-stack match the self-update
// path uses and by classifying every container the stack owns.
func (s *Service) TargetsMcHarbor(ctx context.Context, envID, name string) (bool, error) {
	if s.dockerPool == nil {
		return false, nil
	}
	st, err := s.byName(ctx, name)
	if err != nil {
		return false, err
	}
	if st != nil && st.EnvironmentID != nil && *st.EnvironmentID != "" {
		envID = *st.EnvironmentID
	}
	if st != nil {
		if self, selfErr := s.stackTargetsSelf(ctx, envID, st); selfErr == nil && self {
			return true, nil
		}
	}
	containers, err := s.StackContainers(ctx, envID, name)
	if err != nil {
		return false, err
	}
	for _, c := range containers {
		if docker.ClassifySummary(c.ID, c.Names, c.Image, c.Labels) != docker.SelfTargetNone {
			return true, nil
		}
	}
	return false, nil
}

// guardSelfStack refuses op on a stack that runs McHarbor to a caller without
// host_self.manage and reports whether the handler may continue. A stack whose daemon
// cannot be read passes: the operation itself then fails on the same daemon.
func (h *Handler) guardSelfStack(w http.ResponseWriter, r *http.Request, name, op string) bool {
	envID := response.ParseEnvID(r)
	targets, err := h.service.TargetsMcHarbor(r.Context(), envID, name)
	if err != nil {
		h.app.Logger.Warn("stacks: self-target check failed", "name", name, "env", envID, "op", op, "error", err)
		return true
	}
	if !targets {
		return true
	}
	allowed, err := selfguard.Allowed(r.Context())
	if err != nil {
		h.app.Logger.Error("stacks: host_self.manage check failed", "name", name, "env", envID, "op", op, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return false
	}
	if allowed {
		return true
	}
	selfguard.Refuse(w, r, h.app.AuditLog, &selfguard.Refusal{Code: i18n.ErrHostSelfStackRequired, Target: stackSelfTarget},
		audit.Entry{EntityType: "stack", EntityName: name, EnvironmentID: envID, Details: "op=" + op})
	return false
}

// writeComposeRefusal answers a compose result the host-access check refused; the service
// has already audited it against this request.
func writeComposeRefusal(w http.ResponseWriter, r *http.Request, result *ComposeResult) bool {
	if result == nil || result.Refusal == nil {
		return false
	}
	selfguard.Deny(w, r, result.Refusal)
	return true
}

// writeRefusalError answers an error carrying a host_self Refusal.
func writeRefusalError(w http.ResponseWriter, r *http.Request, err error) bool {
	var ref *selfguard.Refusal
	if !errors.As(err, &ref) {
		return false
	}
	selfguard.Deny(w, r, ref)
	return true
}

// composeHostAccessRefusal checks a compose file about to be brought up on envID: on the
// environment McHarbor runs on, a service that reaches its host, or a project that is
// McHarbor's own, needs host_self.manage. It returns nil when the up may go ahead. The
// decision is Attended: a background up (a secret rotation, a webhook, a scheduled update)
// has nobody whose host_self.manage could vouch for it, so it is refused.
func (s *Service) composeHostAccessRefusal(ctx context.Context, composeFile, projectPath string, envVars map[string]string, envID string) *selfguard.Refusal {
	if s.dockerPool == nil {
		return nil
	}
	path := composeFile
	if !filepath.IsAbs(path) {
		path = filepath.Join(projectPath, path)
	}
	content, readErr := os.ReadFile(path)
	reasons, code := s.composeHostAccessReasons(ctx, content, readErr, projectPath, envVars, envID, true)
	if len(reasons) == 0 {
		return nil
	}
	allowed, err := selfguard.Attended(ctx)
	if err == nil && allowed {
		return nil
	}
	ref := &selfguard.Refusal{Code: code, Target: "host_access", Reasons: reasons}
	selfguard.AuditRefusal(ctx, ref, audit.Entry{
		EntityType: "stack", EntityName: filepath.Base(projectPath), EnvironmentID: envID, Details: "op=compose_up",
	})
	return ref
}

// composeHostAccessReasons lists, as "service:reason" or a bare file-level reason, how
// content would reach McHarbor's host when brought up on envID, with the i18n code a
// refusal carries; nothing when envID is not McHarbor's. readErr is the error reading
// content, which cannot then be vouched for. probeLinks inspects the bind sources on the
// daemon for symlinks; only an up does, since a save cannot know what will be linked later.
func (s *Service) composeHostAccessReasons(ctx context.Context, content []byte, readErr error, projectPath string, envVars map[string]string, envID string, probeLinks bool) ([]string, i18n.MsgCode) {
	host, err := s.dockerPool.SelfHost(ctx, envID)
	if err != nil {
		slog.Warn("stacks: self-host detection failed; treating the environment as McHarbor's", "env", envID, "error", err)
		host = docker.SelfHost{Hosted: true}
	}
	if !host.Hosted {
		return nil, ""
	}
	var probe docker.BindPathProbe
	if probeLinks {
		probe = s.dockerPool.BindSymlinkProbeFor(ctx, envID, host)
	}
	var reasons []string
	var findings []docker.ComposeHostAccess
	if readErr != nil {
		// An unreadable file cannot be vouched for; compose would fail on it anyway.
		reasons = []string{"unreadable_compose"}
	} else if findings, err = docker.ComposeHostAccessReasonsProbe(content, projectPath, envVars, host, s.dockerPool.VolumeLookupFor(ctx, envID), probe); err != nil {
		reasons = []string{"unparsed_compose"}
	}
	for _, f := range findings {
		for _, reason := range f.Reasons {
			if f.Service == "" {
				reasons = append(reasons, reason)
			} else {
				reasons = append(reasons, fmt.Sprintf("%s:%s", f.Service, reason))
			}
		}
	}
	code := i18n.ErrHostSelfHostAccessRequired
	for _, reason := range reasons {
		if strings.HasSuffix(reason, docker.HostAccessOwnProject) {
			code = i18n.ErrHostSelfStackRequired
		}
	}
	return reasons, code
}

// ownedStackRefusal is the host_self check for the owned-stack seam, which GitOps calls
// from a promotion or a preview teardown with nobody behind it. A stack that runs McHarbor,
// or compose that would reach McHarbor's host, needs an attached host_self.manage decision:
// a bare context is refused rather than read as McHarbor acting for itself. compose is nil
// for a teardown.
func (s *Service) ownedStackRefusal(ctx context.Context, envID, name string, existing *ownedStackRow, compose []byte, projectPath string, envVars map[string]string) error {
	if s.dockerPool == nil {
		return nil
	}
	if existing != nil {
		targets, err := s.TargetsMcHarbor(ctx, envID, name)
		if err != nil {
			return fmt.Errorf("stacks: checking whether %s runs McHarbor: %w", name, err)
		}
		if targets {
			if err := selfguard.RequireAttended(ctx, i18n.ErrHostSelfStackRequired, stackSelfTarget, nil); err != nil {
				slog.Warn("stacks: refused an owned-stack change to a stack that runs McHarbor", "stack", name, "env", envID, "error", err)
				return err
			}
		}
	}
	if compose == nil {
		return nil
	}
	reasons, code := s.composeHostAccessReasons(ctx, compose, nil, projectPath, envVars, envID, true)
	if len(reasons) == 0 {
		return nil
	}
	if err := selfguard.RequireAttended(ctx, code, "host_access", reasons); err != nil {
		slog.Warn("stacks: refused an owned-stack deploy that reaches McHarbor's host", "stack", name, "env", envID, "reasons", reasons, "error", err)
		return err
	}
	return nil
}

// composeSaveRefusal runs the compose host-access check on a stack edit before anything is
// written, so content that reaches McHarbor's host is not stored for a later up to find. An
// unresolved source is left to the up check, which sees the resolved secrets.
func (s *Service) composeSaveRefusal(ctx context.Context, st *Stack, req UpdateRequest) error {
	if s.dockerPool == nil || (req.Compose == nil && req.EnvVars == nil) {
		return nil
	}
	envID := ""
	if st.EnvironmentID != nil {
		envID = *st.EnvironmentID
	}
	var content []byte
	var readErr error
	if req.Compose != nil {
		content = []byte(*req.Compose)
	} else {
		content, readErr = os.ReadFile(filepath.Join(st.ProjectPath, st.ComposeFile))
	}
	envVars, err := s.loadEnvVars(ctx, st.ID)
	if err != nil {
		return fmt.Errorf("reading stack environment: %w", err)
	}
	if req.EnvVars != nil {
		envVars = mergeMaskedEnvVars(req.EnvVars, envVars)
	}
	all, code := s.composeHostAccessReasons(ctx, content, readErr, st.ProjectPath, envVars, envID, false)
	var reasons []string
	for _, reason := range all {
		if !strings.HasSuffix(reason, docker.HostAccessUnresolved) {
			reasons = append(reasons, reason)
		}
	}
	if len(reasons) == 0 {
		return nil
	}
	err = selfguard.RequireAttended(ctx, code, "host_access", reasons)
	var ref *selfguard.Refusal
	if errors.As(err, &ref) {
		selfguard.AuditRefusal(ctx, ref, audit.Entry{
			EntityType: "stack", EntityName: st.Name, EnvironmentID: envID, Details: "op=save",
		})
	}
	return err
}
