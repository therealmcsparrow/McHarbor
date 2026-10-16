// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_tools

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

// graphEdge is one hop in the McHarbor entity graph. Direction
// ("parent" / "child") is relative to the focal entity.
type graphEdge struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Name  string `json:"name,omitempty"`
	Label string `json:"label,omitempty"`
	Via   string `json:"via"` // human description of the relation
}

// graphWalkResult is the return shape of entity.relate. The cursor
// engine stores `focus` for the next turn; the LLM sees the full
// parents/children lists.
type graphWalkResult struct {
	Kind     string      `json:"kind"`
	ID       string      `json:"id"`
	Depth    int         `json:"depth"`
	Parents  []graphEdge `json:"parents"`
	Children []graphEdge `json:"children"`
	Focus    *graphFocal `json:"focus,omitempty"`
}

type graphFocal struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Name  string `json:"name,omitempty"`
	Label string `json:"label,omitempty"`
}

// walkGraph performs a bounded BFS from a focal entity, returning
// parents + children at each depth. Cycles are broken by tracking
// visited (kind, id) tuples; the visited set is per-walk so the
// model can re-anchor later.
//
// depth is clamped to [1, 3] to keep the response small and prevent
// runaway walks over very connected entities (env → stacks → containers).
func walkGraph(ctx context.Context, db *sql.DB, kind, id string, depth int) (*graphWalkResult, error) {
	if db == nil {
		return nil, &CallError{Code: ErrToolDisabled, Message: "graph walker has no database"}
	}
	if !IsValidFocalKind(kind) {
		return nil, &CallError{Code: ErrNotFound, Message: "unknown entity kind: " + kind}
	}
	if strings.TrimSpace(id) == "" {
		return nil, &CallError{Code: ErrNotFound, Message: "entity id is required"}
	}
	if depth <= 0 {
		depth = 1
	}
	if depth > 3 {
		depth = 3
	}

	// Confirm the focal exists; if not, surface ErrNotFound.
	focal, err := loadFocal(ctx, db, kind, id)
	if err != nil {
		return nil, err
	}
	if focal == nil {
		return nil, &CallError{Code: ErrNotFound, Message: kind + " " + id + " not found"}
	}

	res := &graphWalkResult{
		Kind:  kind,
		ID:    id,
		Depth: depth,
		Focus: &graphFocal{Kind: focal.Kind, ID: focal.ID, Name: focal.Name, Label: focal.Label},
	}

	visited := map[string]struct{}{graphKey(kind, id): {}}
	frontier := []graphFocal{*focal}

	for hop := 0; hop < depth; hop++ {
		next := make([]graphFocal, 0, len(frontier)*2)
		for _, n := range frontier {
			parents, children, perr := step(ctx, db, n)
			if perr != nil {
				// Per-hop errors degrade to "missing edge" rather
				// than failing the whole walk.
				continue
			}
			for _, e := range parents {
				if _, seen := visited[graphKey(e.Kind, e.ID)]; seen {
					continue
				}
				visited[graphKey(e.Kind, e.ID)] = struct{}{}
				res.Parents = append(res.Parents, e)
				next = append(next, graphFocal{Kind: e.Kind, ID: e.ID, Name: e.Name, Label: e.Label})
			}
			for _, e := range children {
				if _, seen := visited[graphKey(e.Kind, e.ID)]; seen {
					continue
				}
				visited[graphKey(e.Kind, e.ID)] = struct{}{}
				res.Children = append(res.Children, e)
				next = append(next, graphFocal{Kind: e.Kind, ID: e.ID, Name: e.Name, Label: e.Label})
			}
		}
		if len(next) == 0 {
			break
		}
		frontier = next
	}

	return res, nil
}

// step computes the immediate parents and children of one focal. The
// shape of each query is dictated by the McHarbor DB schema; see
// core/db/migrations/001_initial.sql and friends.
func step(ctx context.Context, db *sql.DB, f graphFocal) ([]graphEdge, []graphEdge, error) {
	switch f.Kind {
	case "container":
		return containerStep(ctx, db, f.ID)
	case "image":
		return imageStep(ctx, db, f.ID)
	case "stack":
		return stackStep(ctx, db, f.ID)
	case "env":
		return envStep(ctx, db, f.ID)
	case "workflow":
		return workflowStep(ctx, db, f.ID)
	case "scan":
		return scanStep(ctx, db, f.ID)
	case "user":
		return userStep(ctx, db, f.ID)
	default:
		return nil, nil, fmt.Errorf("unknown kind: %s", f.Kind)
	}
}

func containerStep(ctx context.Context, db *sql.DB, id string) ([]graphEdge, []graphEdge, error) {
	var parents, children []graphEdge
	// Parents: container → stack (via container_stack_links) → env.
	var stackName, envID string
	err := db.QueryRowContext(ctx,
		`SELECT stack_name, COALESCE(environment_id, '') FROM container_stack_links WHERE container_id = ? LIMIT 1`,
		id,
	).Scan(&stackName, &envID)
	if err == nil {
		children = append(children, graphEdge{Kind: "stack", ID: stackName, Name: stackName, Via: "container belongs to stack"})
		if envID != "" {
			parents = append(parents, graphEdge{Kind: "env", ID: envID, Via: "stack lives in env"})
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, err
	}
	// Children: container → image (look up the image's last scan).
	var imageRef string
	if err := db.QueryRowContext(ctx,
		`SELECT image_ref FROM container_events
		 WHERE container_id = ? AND image_ref != ''
		 ORDER BY timestamp DESC LIMIT 1`,
		id,
	).Scan(&imageRef); err == nil && imageRef != "" {
		// The container's image is identified by ref (e.g. "nginx:1.27")
		// not by ID; downstream scans.index look up by ref.
		children = append(children, graphEdge{Kind: "image", ID: imageRef, Name: imageRef, Label: "container image", Via: "container runs image"})
	}
	return parents, children, nil
}

func imageStep(ctx context.Context, db *sql.DB, id string) ([]graphEdge, []graphEdge, error) {
	var parents, children []graphEdge
	// Children: image → scans (by image_ref).
	rows, err := db.QueryContext(ctx,
		`SELECT id, COALESCE(severity, ''), total_vulns, status
		 FROM scans WHERE image_ref = ? ORDER BY created_at DESC LIMIT 10`,
		id,
	)
	if err == nil {
		defer func() {
			if err := rows.Close(); err != nil {
				slog.Warn("closing sql rows failed", "error", err)
			}
		}()
		for rows.Next() {
			var sid, sev, status string
			var totalVulns int
			if err := rows.Scan(&sid, &sev, &totalVulns, &status); err != nil {
				continue
			}
			label := fmt.Sprintf("%s vulns=%d", status, totalVulns)
			children = append(children, graphEdge{
				Kind: "scan", ID: sid, Label: label, Via: "image has scan",
			})
		}
		if err := rows.Err(); err != nil {
			return nil, nil, err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, err
	}
	// Parents: image → containers that ran it. We only have refs
	// in container_events, so we surface the count rather than
	// every container to keep the response bounded.
	var runningCount int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT container_id) FROM container_events WHERE image_ref = ?`,
		id,
	).Scan(&runningCount); err == nil && runningCount > 0 {
		parents = append(parents, graphEdge{
			Kind: "container", ID: id, Name: "containers(" + id + ")",
			Label: fmt.Sprintf("%d containers run this image", runningCount),
			Via:   "image is used by containers",
		})
	}
	return parents, children, nil
}

func stackStep(ctx context.Context, db *sql.DB, id string) ([]graphEdge, []graphEdge, error) {
	var parents, children []graphEdge
	// Parents: stack → env (via environment_id).
	var envID, name string
	err := db.QueryRowContext(ctx,
		`SELECT COALESCE(environment_id, ''), name FROM stacks WHERE id = ?`,
		id,
	).Scan(&envID, &name)
	if err == nil && envID != "" {
		parents = append(parents, graphEdge{Kind: "env", ID: envID, Via: "stack lives in env"})
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, err
	}
	// Children: stack → containers (via container_stack_links).
	rows, err := db.QueryContext(ctx,
		`SELECT container_id, COALESCE(service_name, '') FROM container_stack_links WHERE stack_name = ? LIMIT 50`,
		name,
	)
	if err == nil {
		defer func() {
			if err := rows.Close(); err != nil {
				slog.Warn("closing sql rows failed", "error", err)
			}
		}()
		for rows.Next() {
			var cid, svc string
			if err := rows.Scan(&cid, &svc); err != nil {
				continue
			}
			children = append(children, graphEdge{
				Kind: "container", ID: cid, Name: svc, Via: "stack contains container",
			})
		}
		if err := rows.Err(); err != nil {
			return nil, nil, err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, err
	}
	return parents, children, nil
}

func envStep(ctx context.Context, db *sql.DB, id string) ([]graphEdge, []graphEdge, error) {
	var parents, children []graphEdge
	// Children: env → stacks (by environment_id).
	rows, err := db.QueryContext(ctx,
		`SELECT id, name FROM stacks WHERE environment_id = ? LIMIT 50`,
		id,
	)
	if err == nil {
		defer func() {
			if err := rows.Close(); err != nil {
				slog.Warn("closing sql rows failed", "error", err)
			}
		}()
		for rows.Next() {
			var sid, sname string
			if err := rows.Scan(&sid, &sname); err != nil {
				continue
			}
			children = append(children, graphEdge{
				Kind: "stack", ID: sid, Name: sname, Via: "env contains stack",
			})
		}
		if err := rows.Err(); err != nil {
			return nil, nil, err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, err
	}
	// Children: env → scans (scans table doesn't track env_id, but
	// images in this env are referenced by the running containers
	// — we surface a synthetic edge so the model knows scans are
	// in scope).
	var scanCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM scans`).Scan(&scanCount); err == nil && scanCount > 0 {
		children = append(children, graphEdge{
			Kind: "scan", ID: id, Label: fmt.Sprintf("%d scans in the system", scanCount),
			Via: "env has access to scans (look up by image_ref)",
		})
	}
	return parents, children, nil
}

func workflowStep(ctx context.Context, db *sql.DB, id string) ([]graphEdge, []graphEdge, error) {
	var parents, children []graphEdge
	// Parents: workflow → creator (user).
	var createdBy string
	err := db.QueryRowContext(ctx,
		`SELECT COALESCE(created_by, '') FROM workflows WHERE id = ?`, id,
	).Scan(&createdBy)
	if err == nil && createdBy != "" {
		parents = append(parents, graphEdge{Kind: "user", ID: createdBy, Via: "workflow created by user"})
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, err
	}
	// Children: workflow → runs.
	rows, err := db.QueryContext(ctx,
		`SELECT id, status, started_at FROM workflow_runs WHERE workflow_id = ? ORDER BY started_at DESC LIMIT 10`,
		id,
	)
	if err == nil {
		defer func() {
			if err := rows.Close(); err != nil {
				slog.Warn("closing sql rows failed", "error", err)
			}
		}()
		for rows.Next() {
			var rid, status, startedAt string
			if err := rows.Scan(&rid, &status, &startedAt); err != nil {
				continue
			}
			children = append(children, graphEdge{
				Kind: "workflow", ID: rid, Label: status + " @ " + startedAt, Via: "workflow has run",
			})
		}
		if err := rows.Err(); err != nil {
			return nil, nil, err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, err
	}
	return parents, children, nil
}

func scanStep(ctx context.Context, db *sql.DB, id string) ([]graphEdge, []graphEdge, error) {
	var parents, children []graphEdge
	// Parents: scan → image (by image_ref).
	var imageRef, status string
	var totalVulns int
	err := db.QueryRowContext(ctx,
		`SELECT image_ref, status, total_vulns FROM scans WHERE id = ?`, id,
	).Scan(&imageRef, &status, &totalVulns)
	if err != nil {
		return nil, nil, err
	}
	parents = append(parents, graphEdge{Kind: "image", ID: imageRef, Name: imageRef, Via: "scan inspected image"})
	// Children: scan → vulnerabilities (capped).
	rows, err := db.QueryContext(ctx,
		`SELECT id, vuln_id, severity, COALESCE(pkg_name, '') FROM scan_vulnerabilities WHERE scan_id = ? LIMIT 25`,
		id,
	)
	if err == nil {
		defer func() {
			if err := rows.Close(); err != nil {
				slog.Warn("closing sql rows failed", "error", err)
			}
		}()
		for rows.Next() {
			var vid, vulnID, severity, pkg string
			if err := rows.Scan(&vid, &vulnID, &severity, &pkg); err != nil {
				continue
			}
			children = append(children, graphEdge{
				Kind: "scan", ID: vid, Name: vulnID, Label: severity + " " + pkg, Via: "vulnerability of scan",
			})
		}
		if err := rows.Err(); err != nil {
			return nil, nil, err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, err
	}
	return parents, children, nil
}

func userStep(ctx context.Context, db *sql.DB, id string) ([]graphEdge, []graphEdge, error) {
	var parents, children []graphEdge
	// Parents: user → groups (via group_members).
	grows, err := db.QueryContext(ctx,
		`SELECT g.id, g.name FROM groups g
		 JOIN group_members gm ON gm.group_id = g.id
		 WHERE gm.user_id = ? LIMIT 25`,
		id,
	)
	if err == nil {
		defer func() {
			if err := grows.Close(); err != nil {
				slog.Warn("closing sql rows failed", "error", err)
			}
		}()
		for grows.Next() {
			var gid, gname string
			if err := grows.Scan(&gid, &gname); err != nil {
				continue
			}
			parents = append(parents, graphEdge{Kind: "user", ID: gid, Name: gname, Via: "user is in group"})
		}
		if err := grows.Err(); err != nil {
			return nil, nil, err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, err
	}
	// Children: user → workflows (by created_by).
	wrows, err := db.QueryContext(ctx,
		`SELECT id, name FROM workflows WHERE created_by = ? LIMIT 25`,
		id,
	)
	if err == nil {
		defer func() {
			if err := wrows.Close(); err != nil {
				slog.Warn("closing sql rows failed", "error", err)
			}
		}()
		for wrows.Next() {
			var wid, wname string
			if err := wrows.Scan(&wid, &wname); err != nil {
				continue
			}
			children = append(children, graphEdge{Kind: "workflow", ID: wid, Name: wname, Via: "user created workflow"})
		}
		if err := wrows.Err(); err != nil {
			return nil, nil, err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, err
	}
	return parents, children, nil
}

// loadFocal confirms the focal exists and returns a focal with the
// best display name we can pull. Returns (nil, nil) when not found.
func loadFocal(ctx context.Context, db *sql.DB, kind, id string) (*graphFocal, error) {
	switch kind {
	case "container":
		var name string
		err := db.QueryRowContext(ctx,
			`SELECT COALESCE(container_name, '') FROM container_events
			 WHERE container_id = ? ORDER BY timestamp DESC LIMIT 1`,
			id,
		).Scan(&name)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &graphFocal{Kind: kind, ID: id, Name: name}, nil
	case "image":
		return &graphFocal{Kind: kind, ID: id, Name: id, Label: "image ref"}, nil
	case "stack":
		var name string
		err := db.QueryRowContext(ctx,
			`SELECT name FROM stacks WHERE id = ?`, id,
		).Scan(&name)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &graphFocal{Kind: kind, ID: id, Name: name}, nil
	case "env":
		var name string
		err := db.QueryRowContext(ctx,
			`SELECT name FROM environments WHERE id = ?`, id,
		).Scan(&name)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &graphFocal{Kind: kind, ID: id, Name: name}, nil
	case "workflow":
		var name string
		err := db.QueryRowContext(ctx,
			`SELECT name FROM workflows WHERE id = ?`, id,
		).Scan(&name)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &graphFocal{Kind: kind, ID: id, Name: name}, nil
	case "scan":
		var imageRef, status string
		var totalVulns int
		err := db.QueryRowContext(ctx,
			`SELECT image_ref, status, total_vulns FROM scans WHERE id = ?`, id,
		).Scan(&imageRef, &status, &totalVulns)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &graphFocal{
			Kind:  kind,
			ID:    id,
			Name:  imageRef,
			Label: fmt.Sprintf("%s vulns=%d", status, totalVulns),
		}, nil
	case "user":
		var name string
		err := db.QueryRowContext(ctx,
			`SELECT COALESCE(username, '') FROM users WHERE id = ?`, id,
		).Scan(&name)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &graphFocal{Kind: kind, ID: id, Name: name}, nil
	default:
		return nil, &CallError{Code: ErrNotFound, Message: "unknown entity kind: " + kind}
	}
}

func graphKey(kind, id string) string { return kind + "\x00" + id }
