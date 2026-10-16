// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package images

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/moby/moby/api/types/image"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/envredact"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// permImageEnvReveal gates ?reveal=true on image inspect and history. It reuses
// images.manage: whoever may pull, tag and import images already controls what they carry.
const permImageEnvReveal = rbac.PermImagesManage

// envInstruction matches an ENV step in a history entry, in both the classic builder's
// `/bin/sh -c #(nop)  ENV …` form and BuildKit's bare `ENV …` form.
var envInstruction = regexp.MustCompile(`^((?:/bin/sh -c #\(nop\)\s+)?ENV\s+)(.*)$`)

// buildArgPrefix matches the `|N KEY=value …` prefix a RUN step records for the N build
// arguments in scope, which is where a --build-arg credential ends up in the history.
var buildArgPrefix = regexp.MustCompile(`^((?:RUN\s+)?\|(\d+)\s+)(.*)$`)

// envPolicy is the redaction rule for one caller. Pinned catches an image that baked one of
// McHarbor's own keys in under any name; a revealed view still withholds those.
func (h *Handler) envPolicy(reveal bool) envredact.Policy {
	var extra []string
	if h.app.BackupCrypto != nil {
		extra = append(extra, h.app.BackupCrypto.KeyMaterialBase64())
	}
	return envredact.Policy{Reveal: reveal, Pinned: envredact.PinnedFromConfig(h.app.Config, extra...)}
}

// mayRevealEnv answers the reveal request itself when the caller lacks permImageEnvReveal on
// envID, and reports whether the handler may continue. It fails closed: an RBAC error is a
// 500, never a reveal.
func (h *Handler) mayRevealEnv(w http.ResponseWriter, r *http.Request, envID string) bool {
	if h.app.RBACService == nil {
		response.ForbiddenCode(w, r, i18n.ErrRBACPermissionDenied)
		return false
	}
	allowed, err := h.app.RBACService.AllowedOn(r, envID, permImageEnvReveal)
	if err != nil {
		h.app.Logger.Error("checking the image env reveal permission failed", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return false
	}
	if !allowed {
		rbac.RecordDenied(r, auth.UserFromContext(r.Context()), permImageEnvReveal, envID)
		response.ForbiddenCode(w, r, i18n.ErrRBACPermissionDenied)
		return false
	}
	return true
}

// auditEnvReveal records one reveal at warning severity, naming what stayed withheld so the
// trail tells a pinned-value refusal apart from a full reveal.
func (h *Handler) auditEnvReveal(r *http.Request, envID, entityID, entityName, what string, withheld []string) {
	if h.app.AuditLog == nil {
		return
	}
	details := what + " revealed"
	if len(withheld) > 0 {
		details += "; still withheld: " + strings.Join(withheld, ",")
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "reveal_env",
		Severity:      "warning",
		EntityType:    "image",
		EntityID:      entityID,
		EntityName:    entityName,
		Details:       details,
		EnvironmentID: envID,
	})
}

// redactImage withholds info's Config.Env values for this policy and returns the names it
// withheld. Config is replaced by a copy, so a caller's other reference keeps the values.
func redactImage(p envredact.Policy, info *image.InspectResponse) []string {
	if info == nil || info.Config == nil {
		return nil
	}
	cfg := *info.Config
	env, withheld := p.EnvList(cfg.Env)
	cfg.Env = env
	info.Config = &cfg
	return withheld
}

// redactHistory returns a copy of items whose ENV steps and build-argument prefixes carry the
// Marker in place of each withheld value, and the names it withheld.
func redactHistory(p envredact.Policy, items []image.HistoryResponseItem) ([]image.HistoryResponseItem, []string) {
	if items == nil {
		return nil, nil
	}
	out := make([]image.HistoryResponseItem, len(items))
	var withheld []string
	for i, item := range items {
		var names []string
		item.CreatedBy, names = redactCreatedBy(p, item.CreatedBy)
		withheld = append(withheld, names...)
		out[i] = item
	}
	return out, withheld
}

// redactCreatedBy masks the values in one history entry's ENV step or build-arg prefix.
func redactCreatedBy(p envredact.Policy, createdBy string) (string, []string) {
	if m := envInstruction.FindStringSubmatch(createdBy); m != nil {
		body, names := redactEnvBody(p, m[2])
		return m[1] + body, names
	}
	if m := buildArgPrefix.FindStringSubmatch(createdBy); m != nil {
		count, err := strconv.Atoi(m[2])
		if err != nil || count <= 0 {
			return createdBy, nil
		}
		rest, names := redactPairs(p, m[3], count)
		return m[1] + rest, names
	}
	return createdBy, nil
}

// redactEnvBody masks an ENV instruction's arguments: `K=v K2=v2` pairs, or the legacy
// `K value with spaces` form, where everything after the name is the value.
func redactEnvBody(p envredact.Policy, body string) (string, []string) {
	words := shellWords(body)
	if len(words) == 0 {
		return body, nil
	}
	first := body[words[0][0]:words[0][1]]
	if !strings.Contains(first, "=") {
		value := strings.TrimSpace(body[words[0][1]:])
		if p.Withhold(first, unquote(value)) {
			return first + " " + envredact.Marker, []string{first}
		}
		return body, nil
	}
	return redactPairs(p, body, len(words))
}

// redactPairs masks the values of the first n KEY=value words of text and leaves the rest,
// including its spacing, exactly as it was.
func redactPairs(p envredact.Policy, text string, n int) (string, []string) {
	words := shellWords(text)
	if n > len(words) {
		n = len(words)
	}
	var b strings.Builder
	var withheld []string
	last := 0
	for _, w := range words[:n] {
		word := text[w[0]:w[1]]
		name, value, ok := strings.Cut(word, "=")
		if !ok || !p.Withhold(name, unquote(value)) {
			continue
		}
		b.WriteString(text[last:w[0]])
		b.WriteString(name + "=" + envredact.Marker)
		last = w[1]
		withheld = append(withheld, name)
	}
	if withheld == nil {
		return text, nil
	}
	b.WriteString(text[last:])
	return b.String(), withheld
}

// shellWords returns the [start, end) offsets of each whitespace-separated word in s, keeping
// quoted runs and backslash escapes inside the word they belong to.
func shellWords(s string) [][2]int {
	var words [][2]int
	start := -1
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == '\\' && quote == '"' && i+1 < len(s) {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '\\' && i+1 < len(s):
			if start < 0 {
				start = i
			}
			i++
		case c == '"' || c == '\'':
			if start < 0 {
				start = i
			}
			quote = c
		case c == ' ' || c == '\t' || c == '\n':
			if start >= 0 {
				words = append(words, [2]int{start, i})
				start = -1
			}
		default:
			if start < 0 {
				start = i
			}
		}
	}
	if start >= 0 {
		words = append(words, [2]int{start, len(s)})
	}
	return words
}

// unquote strips one pair of matching surrounding quotes, so `""` reads as the empty value
// it is rather than as a two-character credential.
func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}

// imageName is the audit row's display name: the first tag, or the short id.
func imageName(info image.InspectResponse) string {
	if len(info.RepoTags) > 0 {
		return info.RepoTags[0]
	}
	id := strings.TrimPrefix(info.ID, "sha256:")
	if len(id) > 12 {
		id = id[:12]
	}
	return id
}
