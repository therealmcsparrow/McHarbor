// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_assistant

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/rs/xid"
)

// KnowledgeResult is one hit from the embeddings index.
type KnowledgeResult struct {
	ID        string  `json:"id"`
	Source    string  `json:"source"`
	RefID     string  `json:"refId"`
	Chunk     string  `json:"chunk"`
	Score     float32 `json:"score"`
	Model     string  `json:"model"`
	CreatedAt string  `json:"createdAt,omitempty"`
}

// Skill is one row of ai_assistant_skills. System skills are seeded
// from migration 070 and read-only via the API.
type Skill struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	PromptTemplate string `json:"promptTemplate"`
	IsSystem       bool   `json:"isSystem"`
	CreatedBy      string `json:"createdBy,omitempty"`
	CreatedAt      string `json:"createdAt,omitempty"`
	UpdatedAt      string `json:"updatedAt,omitempty"`
}

// SkillInput is the body of POST /ai-assistant/skills.
type SkillInput struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	PromptTemplate string `json:"promptTemplate"`
}

// embedModel is the model name we stamp onto embeddings rows. Phase
// 5 ships a deterministic bag-of-words embedder; Phase 6+ can swap in
// a real embedding model.
const embedModel = "tf-idf-v1"

// embedDim is the dimensionality of the bag-of-words vector. 1024
// is large enough to keep word hashes from colliding too often
// while staying compact (4 KiB per chunk).
const embedDim = 1024

// tokenizer splits text into lowercase word tokens. Symbols and
// digits are stripped; the result is suitable for a bag-of-words
// embedding where order doesn't matter.
var tokenRE = regexp.MustCompile(`[a-z][a-z0-9_]+`)

func tokenize(s string) []string {
	s = strings.ToLower(s)
	return tokenRE.FindAllString(s, -1)
}

// embed returns a fixed-dimension float32 vector. The function is
// deterministic and pure-Go (no external model). It hashes each
// token into one of embedDim buckets and applies a sub-linear
// squashing so common words don't dominate.
func embed(text string) []float32 {
	vec := make([]float32, embedDim)
	for _, tok := range tokenize(text) {
		h := fnv32(tok) % uint32(embedDim)
		vec[h] += 1.0
	}
	// L2 normalise so cosine similarity == dot product.
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	if sum == 0 {
		return vec
	}
	norm := float32(1.0 / math.Sqrt(sum))
	for i := range vec {
		vec[i] *= norm
	}
	return vec
}

// fnv32 is the 32-bit FNV-1a hash. We use it instead of a map so the
// embedding is fully deterministic and order-independent.
func fnv32(s string) uint32 {
	const offset uint32 = 2166136261
	const prime uint32 = 16777619
	h := offset
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= prime
	}
	return h
}

// encodeEmbedding serializes a float32 vector to a BLOB.
func encodeEmbedding(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

// decodeEmbedding parses a BLOB back into a float32 vector.
func decodeEmbedding(b []byte) ([]float32, error) {
	if len(b)%4 != 0 {
		return nil, errors.New("embedding length not multiple of 4")
	}
	out := make([]float32, len(b)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out, nil
}

// cosine returns the cosine similarity of two equal-length vectors.
// Returns 0 when either vector is zero (avoiding division by zero).
func cosine(a, b []float32) float32 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var dot, na, nb float64
	for i := 0; i < n; i++ {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(na) * math.Sqrt(nb)))
}

// AddKnowledge indexes a single chunk. Re-indexing the same
// (source, ref_id) pair replaces the existing row so updates
// propagate without manual cleanup.
func (s *Service) AddKnowledge(ctx context.Context, source, refID, chunk string) error {
	if strings.TrimSpace(chunk) == "" {
		return errors.New("empty chunk")
	}
	if source == "" {
		source = "manual"
	}
	vec := embed(chunk)
	blob := encodeEmbedding(vec)
	id := xid.New().String()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO ai_assistant_embeddings (id, source, ref_id, chunk, embedding, model, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   chunk = excluded.chunk,
		   embedding = excluded.embedding,
		   model = excluded.model`,
		id, source, refID, chunk, blob, embedModel, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("index chunk: %w", err)
	}
	return nil
}

// SearchKnowledge returns the top-k chunks ranked by cosine
// similarity to the query. Returns up to topK hits; an empty
// index returns an empty slice.
func (s *Service) SearchKnowledge(ctx context.Context, query string, topK int) ([]KnowledgeResult, error) {
	if topK <= 0 {
		topK = 5
	}
	if topK > 20 {
		topK = 20
	}
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	qvec := embed(query)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, source, COALESCE(ref_id, ''), chunk, embedding, model, created_at
		 FROM ai_assistant_embeddings
		 WHERE model = ?
		 LIMIT 1000`,
		embedModel,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()
	type scored struct {
		res   KnowledgeResult
		vec   []float32
		score float32
	}
	var hits []scored
	for rows.Next() {
		var (
			id, source, refID, chunk, model, createdAt string
			blob                                       []byte
		)
		if err := rows.Scan(&id, &source, &refID, &chunk, &blob, &model, &createdAt); err != nil {
			return nil, err
		}
		vec, err := decodeEmbedding(blob)
		if err != nil {
			continue
		}
		s := cosine(qvec, vec)
		hits = append(hits, scored{
			res: KnowledgeResult{
				ID: id, Source: source, RefID: refID,
				Chunk: chunk, Model: model, CreatedAt: createdAt,
			},
			vec: vec, score: s,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	if len(hits) > topK {
		hits = hits[:topK]
	}
	out := make([]KnowledgeResult, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.res)
	}
	return out, nil
}

// --- Skills CRUD ---

// ListSkills returns system + user skills. System skills are read-only
// via the API; the frontend uses the isSystem flag to render them
// in a separate section.
func (s *Service) ListSkills(ctx context.Context) ([]Skill, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, COALESCE(description, ''), prompt_template, is_system,
		        COALESCE(created_by, ''), created_at, updated_at
		 FROM ai_assistant_skills
		 ORDER BY is_system DESC, name ASC
		 LIMIT 1000`,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()
	out := make([]Skill, 0)
	for rows.Next() {
		var sk Skill
		var isSys int
		if err := rows.Scan(&sk.ID, &sk.Name, &sk.Description, &sk.PromptTemplate,
			&isSys, &sk.CreatedBy, &sk.CreatedAt, &sk.UpdatedAt,
		); err != nil {
			return nil, err
		}
		sk.IsSystem = isSys == 1
		out = append(out, sk)
	}
	return out, rows.Err()
}

// SkillByID returns one skill by id.
func (s *Service) SkillByID(ctx context.Context, id string) (*Skill, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, COALESCE(description, ''), prompt_template, is_system,
		        COALESCE(created_by, ''), created_at, updated_at
		 FROM ai_assistant_skills WHERE id = ?`,
		id,
	)
	var sk Skill
	var isSys int
	if err := row.Scan(&sk.ID, &sk.Name, &sk.Description, &sk.PromptTemplate,
		&isSys, &sk.CreatedBy, &sk.CreatedAt, &sk.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	sk.IsSystem = isSys == 1
	return &sk, nil
}

// CreateSkill inserts a new user-defined skill.
func (s *Service) CreateSkill(ctx context.Context, in SkillInput, userID string) (*Skill, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, errors.New("name is required")
	}
	if strings.TrimSpace(in.PromptTemplate) == "" {
		return nil, errors.New("promptTemplate is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	sk := &Skill{
		ID:             xid.New().String(),
		Name:           in.Name,
		Description:    in.Description,
		PromptTemplate: in.PromptTemplate,
		IsSystem:       false,
		CreatedBy:      userID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO ai_assistant_skills (id, name, description, prompt_template, is_system, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 0, ?, ?, ?)`,
		sk.ID, sk.Name, sk.Description, sk.PromptTemplate, sk.CreatedBy, sk.CreatedAt, sk.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create skill: %w", err)
	}
	return sk, nil
}

// UpdateSkill modifies a user-defined skill. System skills are
// immutable via the API.
func (s *Service) UpdateSkill(ctx context.Context, id string, in SkillInput) (*Skill, error) {
	cur, err := s.SkillByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if cur == nil {
		return nil, fmt.Errorf("skill not found")
	}
	if cur.IsSystem {
		return nil, fmt.Errorf("system skills are read-only")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if strings.TrimSpace(in.Name) == "" {
		in.Name = cur.Name
	}
	if strings.TrimSpace(in.PromptTemplate) == "" {
		in.PromptTemplate = cur.PromptTemplate
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE ai_assistant_skills
		 SET name = ?, description = ?, prompt_template = ?, updated_at = ?
		 WHERE id = ?`,
		in.Name, in.Description, in.PromptTemplate, now, id,
	)
	if err != nil {
		return nil, fmt.Errorf("update skill: %w", err)
	}
	return s.SkillByID(ctx, id)
}

// DeleteSkill removes a user-defined skill.
func (s *Service) DeleteSkill(ctx context.Context, id string) error {
	cur, err := s.SkillByID(ctx, id)
	if err != nil {
		return err
	}
	if cur == nil {
		return nil
	}
	if cur.IsSystem {
		return fmt.Errorf("system skills are read-only")
	}
	_, err = s.db.ExecContext(ctx,
		`DELETE FROM ai_assistant_skills WHERE id = ?`, id,
	)
	return err
}

// SeedDefaultKnowledge indexes a small set of high-signal chunks
// covering common McHarbor operations. Idempotent: it skips chunks
// that already exist with the same (source, ref_id) so re-running
// on every startup is safe.
func (s *Service) SeedDefaultKnowledge(ctx context.Context) error {
	type seed struct {
		Source string
		RefID  string
		Chunk  string
	}
	seeds := []seed{
		{Source: "docs", RefID: "containers.list",
			Chunk: "List containers on an environment with containers.list. Args: env_id. Use containers.inspect to get a deeper view of one container; containers.logs reads stdout/stderr; containers.stats reads live CPU/memory metrics."},
		{Source: "docs", RefID: "stacks.list",
			Chunk: "List Compose stacks with stacks.list. A stack groups related containers; the assistant can map containers → stacks via container_stack_links, then to environments via stacks.environment_id."},
		{Source: "docs", RefID: "scans",
			Chunk: "Trivy-based vulnerability scans live in the scans table. Each scan records image_ref, status, total_vulns, and per-severity counts. Use scans.list to enumerate, scans.get to drill in."},
		{Source: "docs", RefID: "audit.search",
			Chunk: "The audit_log records every mutating action across the platform. Use audit.search to find when a container was restarted, a user was added, or a settings change occurred. Useful for postmortems."},
		{Source: "runbook", RefID: "rb_container_restart_loop",
			Chunk: "Container restart loop: check container logs (tail=200) for crash output, then look at the image's latest scan (scans.list by image_ref). Common causes: missing env var, port conflict, bad image tag."},
		{Source: "runbook", RefID: "rb_high_cpu",
			Chunk: "High CPU: use containers.stats to identify the top consumer. Confirm it's not a runaway thread inside the app, then either restart the container or scale the stack. Check recent deploys via audit.search (action=stack.deploy)."},
		{Source: "runbook", RefID: "rb_backup_failure",
			Chunk: "Backup failure: read the most recent backup record in the backups table, check the error column. Common causes: source env offline, target volume full, agent disconnected. Restore path: pick the last good backup and use backups.restore."},
		{Source: "docs", RefID: "automation.authoring",
			Chunk: "The assistant can author automation objects when the user holds the matching manage permission: workflows.create (workflows.manage), alerts.create (alerts.manage), updates.policy.create (updates.manage), schedules.create (schedules.manage). All four are consent-gated — they queue a pending action the user approves before anything is written."},
		{Source: "runbook", RefID: "rb_new_alert_rule",
			Chunk: "Creating an alert rule: call alerts.conditions first to get the valid source_type / condition_key pairs plus each condition's operators, unit and default threshold, then call alerts.create with one of those pairs. With no destination_channel_ids the rule delivers in-app only."},
		{Source: "settings", RefID: "ai.provider",
			Chunk: "AI assistant providers: openai, anthropic, google (Gemini), mistral, xai, deepseek, cohere, perplexity, groq, ollama (local), openrouter, github-models. Each provider supports its own model list. OAuth login is supported for github-models and (soon) anthropic."},
	}
	// One existence probe per seed, bounded by the literal seed slice above
	// and run once at start-up.
	for _, sd := range seeds {
		var exists int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM ai_assistant_embeddings WHERE source = ? AND ref_id = ?`,
			sd.Source, sd.RefID,
		).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		if err := s.AddKnowledge(ctx, sd.Source, sd.RefID, sd.Chunk); err != nil {
			return err
		}
	}
	return nil
}

// prevent unused import when callers go through service-only paths.
var _ = json.Marshal
