// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_tools

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// entity.relate walks the McHarbor entity graph from a starting
// entity and returns its parents and children up to `depth` hops.
// Phase 2 replaces the Phase-1 stub with a real DB-driven walker;
// see graph_walker.go for the per-kind logic.
func init() {
	Register(&Tool{
		Name:        "entity.relate",
		Description: "Walk the McHarbor entity graph from a starting entity. Returns parents (e.g. container→stack→env) and children (e.g. stack→containers, env→images). Use depth 1-3. Pass kind in {container, image, stack, env, workflow, scan, user}.",
		InputSchema: objectSchema(map[string]any{
			"kind":  stringSchema("Entity kind: container, image, stack, env, workflow, scan, user."),
			"id":    stringSchema("Entity ID."),
			"depth": intSchema("How many hops to walk. Default 1, max 3."),
		}, []string{"kind", "id"}),
		Required: []rbac.Permission{},
		ReadOnly: true,
		MaxBytes: 32 * 1024,
		Handler: func(ctx context.Context, sc Scope, args json.RawMessage) (any, error) {
			var p struct {
				Kind  string `json:"kind"`
				ID    string `json:"id"`
				Depth int    `json:"depth"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return nil, &CallError{Code: -32602, Message: "invalid args"}
			}
			if p.Kind == "" || p.ID == "" {
				return nil, &CallError{Code: ErrNotFound, Message: "kind and id are required"}
			}
			d := GetDeps()
			if d == nil || d.DB == nil {
				return nil, &CallError{Code: ErrToolDisabled, Message: "graph walker has no database"}
			}
			res, err := walkGraph(ctx, d.DB, p.Kind, p.ID, p.Depth)
			if err != nil {
				return nil, err
			}
			// Convert to a generic map for the JSON response.
			out := map[string]any{
				"kind":     res.Kind,
				"id":       res.ID,
				"depth":    res.Depth,
				"parents":  res.Parents,
				"children": res.Children,
			}
			if res.Focus != nil {
				out["focus"] = res.Focus
			}
			return out, nil
		},
	})

	// knowledge.search: cosine similarity over the embeddings index.
	// The model is `tf-idf-v1` (a pure-Go bag-of-words embedder) so
	// the search has no external dependencies. Phase 6+ can swap in
	// a real embedding model without changing this tool's contract.
	Register(&Tool{
		Name:        "knowledge.search",
		Description: "Search the in-process knowledge index (docs, settings, runbooks, workflows). Returns up to top_k hits ranked by relevance.",
		InputSchema: objectSchema(map[string]any{
			"query": stringSchema("Natural-language query."),
			"top_k": intSchema("Max results. Default 5, max 20."),
		}, []string{"query"}),
		Required: []rbac.Permission{},
		ReadOnly: true,
		MaxBytes: 16 * 1024,
		Handler: func(ctx context.Context, _ Scope, args json.RawMessage) (any, error) {
			var p struct {
				Query string `json:"query"`
				TopK  int    `json:"top_k"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return nil, &CallError{Code: -32602, Message: "invalid args"}
			}
			if strings.TrimSpace(p.Query) == "" {
				return nil, &CallError{Code: -32602, Message: "query is required"}
			}
			if p.TopK <= 0 {
				p.TopK = 5
			}
			if p.TopK > 20 {
				p.TopK = 20
			}
			d := GetDeps()
			if d == nil || d.DB == nil {
				return nil, &CallError{Code: ErrToolDisabled, Message: "knowledge index has no database"}
			}
			hits, err := knowledgeSearch(ctx, d.DB, p.Query, p.TopK)
			if err != nil {
				return nil, fmt.Errorf("knowledge search: %w", err)
			}
			return map[string]any{
				"query":   p.Query,
				"topK":    p.TopK,
				"count":   len(hits),
				"results": hits,
			}, nil
		},
	})
}

// knowledgeSearch runs a TF-IDF cosine search over the embeddings
// table. See knowledge.go (parent package) for the same logic — this
// duplicate is here so the ai_tools package remains self-contained:
// callers in the tool registry can't import the parent.
func knowledgeSearch(ctx context.Context, db *sql.DB, query string, topK int) ([]map[string]any, error) {
	const embedDim = 1024
	const model = "tf-idf-v1"
	qvec := embedText(query, embedDim)
	rows, err := db.QueryContext(ctx,
		`SELECT id, source, COALESCE(ref_id, ''), chunk, embedding FROM ai_assistant_embeddings WHERE model = ? LIMIT 1000`,
		model,
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
		res   map[string]any
		score float32
	}
	var hits []scored
	for rows.Next() {
		var id, source, refID, chunk string
		var blob []byte
		if err := rows.Scan(&id, &source, &refID, &chunk, &blob); err != nil {
			return nil, err
		}
		if len(blob) != 4*embedDim {
			continue
		}
		vec := make([]float32, embedDim)
		for i := range vec {
			vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:]))
		}
		s := cosine32(qvec, vec)
		hits = append(hits, scored{
			res: map[string]any{
				"id":     id,
				"source": source,
				"refId":  refID,
				"chunk":  chunk,
				"score":  s,
			},
			score: s,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	if len(hits) > topK {
		hits = hits[:topK]
	}
	out := make([]map[string]any, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.res)
	}
	return out, nil
}

var tokenRE2 = regexp.MustCompile(`[a-z][a-z0-9_]+`)

func embedText(s string, dim int) []float32 {
	vec := make([]float32, dim)
	for _, tok := range tokenRE2.FindAllString(strings.ToLower(s), -1) {
		vec[fnv32b(tok)%uint32(dim)] += 1.0
	}
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	if sum == 0 {
		return vec
	}
	inv := float32(1.0 / math.Sqrt(sum))
	for i := range vec {
		vec[i] *= inv
	}
	return vec
}

func fnv32b(s string) uint32 {
	const offset uint32 = 2166136261
	const prime uint32 = 16777619
	h := offset
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= prime
	}
	return h
}

func cosine32(a, b []float32) float32 {
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
