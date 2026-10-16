// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package incidents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	coreAudit "github.com/therealmcsparrow/mcharbor/core/audit"
)

// ModelInfo names the model that would receive an environment's evidence, so
// the opt-in can state exactly where it goes.
type ModelInfo struct {
	Provider   string `json:"provider"`
	Connection string `json:"connection"`
	Model      string `json:"model"`
	BaseURL    string `json:"baseUrl,omitempty"`
	Local      bool   `json:"local"`
}

// Model is the narrator's only way out of the process. It has no tools: it
// receives text and returns text, so nothing it says can act.
type Model interface {
	Info(ctx context.Context) (ModelInfo, error)
	Complete(ctx context.Context, system, prompt string, maxTokens int) (string, error)
}

// Refusals a narration can end in, before or instead of calling a model.
var (
	ErrNoModel     = errors.New("incidents: no AI model is configured and enabled")
	ErrNotOptedIn  = errors.New("incidents: this environment has not opted in to narration")
	ErrInProgress  = errors.New("incidents: a narration is already running")
	ErrCapReached  = errors.New("incidents: the hourly narration cap for this environment is reached")
	ErrModelFailed = errors.New("incidents: the model did not answer")
)

// Service narrates incidents and findings.
type Service struct {
	Store     *Store
	Assembler *Assembler
	Model     Model
	Audit     *coreAudit.Logger
	Logger    *slog.Logger
	Now       func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

const narratorSystem = `You are McHarbor's incident narrator. You explain infrastructure incidents — containers, virtual machines, system containers, Kubernetes and Swarm workloads — to an on-call engineer.
You only read: you never suggest running a command that changes anything, and you have no tools.
Use only the evidence given. Do not invent log lines, times, versions or numbers.
Answer with one JSON object and nothing else:
{"summary": "<at most three sentences: what happened and when it started>",
 "startedAt": "<RFC3339 time of the first symptom, from the evidence>",
 "changes": ["<each change shortly before the incident: image, deploy, audit action>"],
 "causes": [{"title": "<probable cause>", "evidence": ["<the exact evidence items that support it>"], "confidence": "low|medium|high"}],
 "confidence": "low|medium|high",
 "undetermined": false}
List at most three causes, most likely first. Each cause must cite evidence.
If the evidence does not support any cause, set "undetermined": true, "confidence": "low", leave "causes" empty,
and say in the summary what you looked at. That is a correct and useful answer.`

// promptEvidence trims a copy of the bundle to what a prompt should carry;
// the stored bundle keeps everything the view shows.
func promptEvidence(ev *Evidence) *Evidence {
	cp := *ev
	if len(cp.Logs) > 80 {
		cp.Logs = cp.Logs[len(cp.Logs)-80:]
	}
	if len(cp.Events) > 60 {
		cp.Events = cp.Events[len(cp.Events)-60:]
	}
	return &cp
}

// BuildPrompt is the exact text a model is shown for an incident. It is stored
// with the narrative so an operator can read what left the building.
func BuildPrompt(in *Incident, ev *Evidence) (string, error) {
	raw, err := json.Marshal(promptEvidence(ev))
	if err != nil {
		return "", fmt.Errorf("encoding evidence for the prompt: %w", err)
	}
	kind := in.SubjectType
	if kind == "" {
		kind = "container"
	}
	return fmt.Sprintf("Incident on %s %q (signal: %s, %d signals, first %s, last %s).\nEvidence (JSON):\n%s",
		kind, in.SubjectName, in.Signal, in.SignalCount, in.FirstSignalAt, in.LastSignalAt, raw), nil
}

var confidenceLevels = map[string]bool{"low": true, "medium": true, "high": true}

// ParseNarrative reads the model's answer. Anything that is not the required
// JSON is kept as a low-confidence, undetermined narrative rather than
// dropped, so the operator sees what the model said and that it is unreliable.
func ParseNarrative(text string) *Narrative {
	text = strings.TrimSpace(text)
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	var n Narrative
	if start >= 0 && end > start && json.Unmarshal([]byte(text[start:end+1]), &n) == nil && strings.TrimSpace(n.Summary) != "" {
		if !confidenceLevels[n.Confidence] {
			n.Confidence = "low"
		}
		kept := n.Causes[:0]
		for _, c := range n.Causes {
			if strings.TrimSpace(c.Title) == "" || len(c.Evidence) == 0 {
				continue
			}
			if !confidenceLevels[c.Confidence] {
				c.Confidence = "low"
			}
			kept = append(kept, c)
			if len(kept) == 3 {
				break
			}
		}
		n.Causes = kept
		if n.Changes == nil {
			n.Changes = []string{}
		}
		if len(n.Causes) == 0 {
			n.Undetermined, n.Confidence = true, "low"
		}
		return &n
	}
	if len(text) > 2000 {
		text = text[:2000]
	}
	return &Narrative{Summary: text, Changes: []string{}, Causes: []Cause{}, Confidence: "low", Undetermined: true}
}

// refreshEvidence re-assembles a bundle older than a couple of minutes, so a
// narration is never written against evidence from the start of a long incident.
func (s *Service) refreshEvidence(ctx context.Context, in *Incident) error {
	if in.Evidence != nil && in.EvidenceAt != "" {
		if at, err := time.Parse(time.RFC3339, in.EvidenceAt); err == nil && s.now().Sub(at) < 2*time.Minute {
			return nil
		}
	}
	ev, err := s.Assembler.Assemble(ctx, in)
	if err != nil {
		return err
	}
	if err := s.Store.SaveEvidence(ctx, in.ID, ev); err != nil {
		return err
	}
	in.Evidence = ev
	return nil
}

// RefreshEvidence re-assembles and stores an incident's bundle now.
func (s *Service) RefreshEvidence(ctx context.Context, id string) (*Incident, error) {
	in, err := s.Store.Get(ctx, id)
	if err != nil || in == nil {
		return in, err
	}
	in.EvidenceAt = ""
	if err := s.refreshEvidence(ctx, in); err != nil {
		return nil, err
	}
	return s.Store.Get(ctx, id)
}

// Narrate has the model explain an incident. It refuses before sending
// anything when the environment has not opted in, no model is configured, a
// narration is already running, or — for proactive runs — the cap is reached.
func (s *Service) Narrate(ctx context.Context, id, by string, proactive bool) (*Incident, error) {
	in, err := s.Store.Get(ctx, id)
	if err != nil || in == nil {
		return in, err
	}
	st, err := s.Store.Settings(ctx, in.EnvID)
	if err != nil {
		return nil, err
	}
	if !st.NarrateOptIn {
		return in, ErrNotOptedIn
	}
	if s.Model == nil {
		return in, ErrNoModel
	}
	info, err := s.Model.Info(ctx)
	if err != nil {
		return in, ErrNoModel
	}
	if proactive {
		n, err := s.Store.NarrationsSince(ctx, in.EnvID, s.now().Add(-time.Hour))
		if err != nil {
			return nil, err
		}
		if n >= st.HourlyCap {
			return in, ErrCapReached
		}
	}
	claimed, err := s.Store.ClaimNarration(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if !claimed {
		return in, ErrInProgress
	}
	if err := s.refreshEvidence(ctx, in); err != nil {
		if ferr := s.Store.FailNarration(ctx, in.ID, NarrativeFailed, "assembling evidence failed"); ferr != nil {
			err = errors.Join(err, ferr)
		}
		return nil, err
	}
	prompt, err := BuildPrompt(in, in.Evidence)
	if err != nil {
		return nil, errors.Join(err, s.Store.FailNarration(ctx, in.ID, NarrativeFailed, "building the prompt failed"))
	}
	callCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	text, err := s.Model.Complete(callCtx, narratorSystem, prompt, 1200)
	if err != nil {
		reason := err.Error()
		if ferr := s.Store.FailNarration(ctx, in.ID, NarrativeFailed, reason); ferr != nil {
			return nil, errors.Join(err, ferr)
		}
		s.logger().Warn("incidents: narration failed", "incident", in.ID, "provider", info.Provider, "error", err)
		return in, fmt.Errorf("%w: %w", ErrModelFailed, err)
	}
	sum := sha256.Sum256([]byte(narratorSystem + "\n\n" + prompt))
	meta := ModelMeta{
		Provider: info.Provider, Connection: info.Connection, Model: info.Model, RequestedBy: by,
		At: s.now().UTC().Format(time.RFC3339), PromptSHA256: hex.EncodeToString(sum[:]), Proactive: proactive,
	}
	n := ParseNarrative(text)
	if err := s.Store.SaveNarrative(ctx, in.ID, n, meta, prompt); err != nil {
		return nil, err
	}
	s.audit("incident.narrated", in, map[string]any{
		"provider": meta.Provider, "connection": meta.Connection, "model": meta.Model, "promptSha256": meta.PromptSHA256,
		"proactive": proactive, "requestedBy": by, "confidence": n.Confidence, "undetermined": n.Undetermined,
	})
	return s.Store.Get(ctx, in.ID)
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func (s *Service) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

func (s *Service) audit(action string, in *Incident, details map[string]any) {
	if s.Audit == nil {
		return
	}
	raw, err := json.Marshal(details)
	if err != nil {
		raw = []byte("{}")
	}
	e := coreAudit.Entry{Action: action, Severity: "info", EntityType: "incident", Details: string(raw)}
	if in != nil {
		e.EntityID, e.EntityName, e.EnvironmentID = in.ID, in.SubjectName, in.EnvID
	}
	s.Audit.Log(nil, e)
}
