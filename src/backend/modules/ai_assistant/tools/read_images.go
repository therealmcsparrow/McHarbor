// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

type imageLister interface {
	List(ctx context.Context, envID string) ([]map[string]any, error)
}

type imageInspecter interface {
	Inspect(ctx context.Context, envID, imageID string) (map[string]any, error)
}

func init() {
	Register(&Tool{
		Name:        "images.list",
		Description: "List all images on an environment: repo, tag, size, created, in-use-by count.",
		InputSchema: objectSchema(map[string]any{
			"env_id": stringSchema("Environment ID."),
		}, []string{}),
		Required: []rbac.Permission{rbac.PermImagesView},
		ReadOnly: true,
		MaxBytes: 64 * 1024,
		Handler: func(ctx context.Context, sc Scope, args json.RawMessage) (any, error) {
			var p struct {
				EnvID string `json:"env_id"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				slog.Debug("ai tool: unreadable optional arguments, using defaults", "error", err)
			}
			d := GetDeps()
			if d == nil || d.ImageList == nil {
				return nil, &CallError{Code: ErrToolDisabled, Message: "images service not wired"}
			}
			lister, ok := d.ImageList.(imageLister)
			if !ok {
				return nil, &CallError{Code: ErrToolDisabled, Message: "images service does not implement List"}
			}
			envID := p.EnvID
			if envID == "" {
				envID = sc.EnvID
			}
			out, err := lister.List(ctx, envID)
			if err != nil {
				return nil, fmt.Errorf("list images: %w", err)
			}
			return map[string]any{"envId": envID, "count": len(out), "items": out}, nil
		},
	})

	Register(&Tool{
		Name:        "images.inspect",
		Description: "Return image manifest + config for one image (repo:tag or ID).",
		InputSchema: objectSchema(map[string]any{
			"env_id":   stringSchema("Environment ID."),
			"image_id": stringSchema("Image ID or repo:tag."),
		}, []string{"image_id"}),
		Required: []rbac.Permission{rbac.PermImagesView},
		ReadOnly: true,
		MaxBytes: 128 * 1024,
		Handler: func(ctx context.Context, sc Scope, args json.RawMessage) (any, error) {
			var p struct {
				EnvID   string `json:"env_id"`
				ImageID string `json:"image_id"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return nil, err
			}
			if p.ImageID == "" {
				return nil, &CallError{Code: ErrNotFound, Message: "image_id is required"}
			}
			d := GetDeps()
			if d == nil || d.ImageList == nil {
				return nil, &CallError{Code: ErrToolDisabled, Message: "images inspect not wired"}
			}
			ins, ok := d.ImageList.(imageInspecter)
			if !ok {
				return nil, &CallError{Code: ErrToolDisabled, Message: "images service does not implement Inspect"}
			}
			envID := p.EnvID
			if envID == "" {
				envID = sc.EnvID
			}
			data, err := ins.Inspect(ctx, envID, p.ImageID)
			if err != nil {
				return nil, fmt.Errorf("inspect image: %w", err)
			}
			return map[string]any{"envId": envID, "imageId": p.ImageID, "data": data}, nil
		},
	})
}
