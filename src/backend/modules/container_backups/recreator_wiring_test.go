// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/config"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

type stubRecreator struct{}

func (stubRecreator) Recreate(context.Context, string, string, coredocker.RecreateRequest) (string, error) {
	return "new-id", nil
}

// TestMountedHandlerCanRestoreConfiguration is the "not supported" refusal: the
// handler builds its own service, and only the scheduler's got the recreator,
// so every configuration restore from the UI found none and was refused.
func TestMountedHandlerCanRestoreConfiguration(t *testing.T) {
	app := &router.AppDeps{
		Config: &config.Config{DataDir: t.TempDir()},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	recreator := stubRecreator{}
	h := newMountedHandler(app, nil, recreator)
	if h.service.containerRecreator == nil {
		t.Fatal("the HTTP handler's service has no container recreator, so a config restore is refused as unsupported")
	}
}
