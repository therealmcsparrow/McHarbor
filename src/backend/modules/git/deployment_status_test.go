// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package git

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

func TestEveryDeploymentStatusTheHandlerWritesIsAccepted(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "git.db")+"?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Log(err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	svc := NewService(database, nil)
	for _, status := range []string{DeploymentSuccess, DeploymentFailure} {
		if _, err := svc.CreateDeployment(context.Background(), "repo-1", "", "main", status, "test"); err != nil {
			t.Fatalf("status %q rejected by git_deployments: %v", status, err)
		}
	}
}
