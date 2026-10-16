// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/secrets"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

func newResolverTestService(t *testing.T) (*Service, *secrets.Service) {
	t.Helper()
	dir := t.TempDir()

	database, err := coredb.OpenSQLite(filepath.Join(dir, "stacks_test.db"))
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() { closeutil.Report(database.Close(), "database") })
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrating: %v", err)
	}

	enc, err := encryption.New(dir, "")
	if err != nil {
		t.Fatalf("creating encryption service: %v", err)
	}

	// NewService builds its own resolver from db + enc, which is the property
	// under test: there is no injection point a construction site can forget.
	// The seeding service takes a permissive authorizer; the resolver the
	// stacks service builds for itself deliberately takes none, because it
	// only ever reads, and a nil authorizer fails closed on the one write
	// that needs authority.
	return NewService(database, nil, dir, enc), secrets.NewService(database, enc, allowAll{})
}

// allowAll is a test authorizer. Seeding a global secret needs
// secrets.manage_global, which a resolve-only service is right to refuse.
type allowAll struct{}

func (allowAll) HasPermission(context.Context, string, string, rbac.Permission) (bool, error) {
	return true, nil
}

// TestNewServiceBuildsItsOwnResolver is the structural guarantee. Seven places
// construct a stacks service; a stacks service without a resolver deploys
// "${secret:DB_PASSWORD}" to a container as a literal string and nothing
// errors anywhere.
func TestNewServiceBuildsItsOwnResolver(t *testing.T) {
	svc, _ := newResolverTestService(t)
	if svc.resolver == nil {
		t.Fatal("NewService must build a secret resolver from the db and encryption service it is given")
	}
}

// TestResolveSecretsSubstitutesBeforeTheDockerCall exercises the choke point
// runDockerCompose calls. It sits above the agent branch, so one call site
// serves local and agent deployments both — see the comment at that call.
func TestResolveSecretsSubstitutesBeforeTheDockerCall(t *testing.T) {
	svc, vault := newResolverTestService(t)
	ctx := context.Background()

	if _, _, err := vault.Create(ctx, "", "", secrets.CreateRequest{
		Name: "DB_PASSWORD", Scope: secrets.ScopeGlobal, Value: "s3cr3t",
	}); err != nil {
		t.Fatalf("creating secret: %v", err)
	}

	stored := map[string]string{
		"WORDPRESS_DB_PASSWORD": "${secret:DB_PASSWORD}",
		"WORDPRESS_DB_USER":     "wordpress",
	}
	resolved, _, err := svc.resolveSecrets(ctx, filepath.Join("x", "wordpress"), stored, "")
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}
	if resolved["WORDPRESS_DB_PASSWORD"] != "s3cr3t" {
		t.Errorf("got %q, want s3cr3t", resolved["WORDPRESS_DB_PASSWORD"])
	}
	if resolved["WORDPRESS_DB_USER"] != "wordpress" {
		t.Errorf("a plain value should pass through, got %q", resolved["WORDPRESS_DB_USER"])
	}
	if stored["WORDPRESS_DB_PASSWORD"] != "${secret:DB_PASSWORD}" {
		t.Fatal("the stored map was mutated; the reference would be overwritten with plaintext on the next save")
	}
}

// TestResolveSecretsFailsTheDeployOnAMiss is blast-radius item four: never
// substitute empty, and name the reference in the message the operator sees.
func TestResolveSecretsFailsTheDeployOnAMiss(t *testing.T) {
	svc, _ := newResolverTestService(t)

	_, _, err := svc.resolveSecrets(context.Background(), filepath.Join("x", "wordpress"),
		map[string]string{"DB_PASSWORD": "${secret:NOT_THERE}"}, "")
	if err == nil {
		t.Fatal("a missing secret must fail the deploy")
	}

	msg := composeSecretError(err)
	if !strings.Contains(msg, "NOT_THERE") {
		t.Errorf("the operator-facing message must name the reference, got %q", msg)
	}
}

// TestComposeSecretErrorHidesInternalDetail is the other half: anything that
// is not an unresolved reference stays generic, because those messages carry
// database and file-path detail that has no business in an API response.
func TestComposeSecretErrorHidesInternalDetail(t *testing.T) {
	msg := composeSecretError(context.DeadlineExceeded)
	if strings.Contains(msg, "deadline") {
		t.Errorf("internal error text leaked into the client message: %q", msg)
	}
}
