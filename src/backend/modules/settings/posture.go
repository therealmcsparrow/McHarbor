// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package settings

import (
	"net/http"
	"sync/atomic"

	"github.com/therealmcsparrow/mcharbor/core/config"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// PostureFinding is one boot posture warning as the panel and the CLI show it.
// It carries only the env var and the fixed message, never a configured value.
type PostureFinding struct {
	EnvVar   string `json:"envVar"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// SecurityPosture is the security-posture payload. An empty Findings list is
// the all-clear, so it is always a list and never null.
type SecurityPosture struct {
	Findings []PostureFinding `json:"findings"`
}

// PostureSeverityCritical and PostureSeverityWarning grade a finding; only a
// disabled authentication layer is critical, the rest are hardening advice.
const (
	PostureSeverityCritical = "critical"
	PostureSeverityWarning  = "warning"
)

// bootPosture holds the facts main settled at boot. The effective TLS state and
// the certificate issuer are only known there, after the router is built.
var bootPosture atomic.Pointer[config.PostureFacts]

// RecordPosture stores the facts the boot posture check ran against, so the
// panel reports exactly what the log said rather than re-deriving it.
func RecordPosture(f config.PostureFacts) {
	bootPosture.Store(&f)
}

// Posture returns the findings for the recorded boot facts, and false when
// nothing was recorded yet.
func Posture() (SecurityPosture, bool) {
	facts := bootPosture.Load()
	if facts == nil {
		return SecurityPosture{Findings: []PostureFinding{}}, false
	}
	return postureFrom(*facts), true
}

func postureFrom(f config.PostureFacts) SecurityPosture {
	warnings := config.PostureWarnings(f)
	out := SecurityPosture{Findings: make([]PostureFinding, 0, len(warnings))}
	for _, w := range warnings {
		severity := PostureSeverityWarning
		if w.EnvVar == "AUTH_DISABLE" {
			severity = PostureSeverityCritical
		}
		out.Findings = append(out.Findings, PostureFinding{EnvVar: w.EnvVar, Severity: severity, Message: w.Message})
	}
	return out
}

// HandleSecurityPosture returns the boot posture findings. It is read-only and
// gated on settings.view in routes.go.
func (h *Handler) HandleSecurityPosture(w http.ResponseWriter, r *http.Request) {
	posture, ok := Posture()
	if !ok {
		h.app.Logger.Error("settings: security posture requested before boot recorded it")
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	response.OK(w, posture)
}
