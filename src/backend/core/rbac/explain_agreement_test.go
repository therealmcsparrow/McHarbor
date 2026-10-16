// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/db"
)

type agreementPolicy struct {
	effect, resourceType, expression string
	priority                         int
}

// agreementPolicySets covers allow, deny-overrides-allow, typed policies a request
// without a resource must skip, a broken expression that must be skipped, and policies
// reading env.id, env.name and env.type.
var agreementPolicySets = map[string][]agreementPolicy{
	"none": nil,
	"deny-prod-name": {
		{"allow", "", `true`, 20},
		{"deny", "", `env.name == "prod"`, 10},
	},
	"allow-not-prod-id": {{"allow", "", `env.id != "env-prod"`, 10}},
	"allow-tcp-type":    {{"allow", "", `env.type == "tcp"`, 10}},
	"allow-bob":         {{"allow", "", `user.username == "bob"`, 10}},
	"deny-overrides": {
		{"allow", "", `true`, 20},
		{"deny", "", `user.username == "alice"`, 10},
	},
	"deny-over-wildcard": {
		{"allow", "", `true`, 20},
		{"deny", "", `user.username == "carol"`, 10},
	},
	"typed": {
		{"deny", "t_res", `true`, 10},
		{"allow", "", `user.username == "alice"`, 20},
	},
	"broken-then-deny": {
		{"allow", "", `)))(`, 5},
		{"deny", "", `user.username != "alice"`, 10},
	},
}

func openAgreementDB(t *testing.T) (*sql.DB, *Service, *PolicyService) {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "agree.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Logf("closing database: %v", err)
		}
	})
	if err := db.Migrate(database, db.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	if err := UpsertCatalog(database); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO users (id, username, password_hash) VALUES ('alice', 'alice', 'x'), ('bob', 'bob', 'x'), ('carol', 'carol', 'x')`,
		`INSERT INTO environments (id, name, connection_type) VALUES ('env-prod', 'prod', 'socket'), ('env-dev', 'dev', 'tcp')`,
		`INSERT INTO roles (id, name, permissions) VALUES ('r_ops', 'r_ops', '["containers.manage"]'), ('r_all', 'r_all', '["*"]')`,
		`INSERT INTO user_roles (id, user_id, role_id, environment_id) VALUES ('ur1', 'alice', 'r_ops', 'env-dev')`,
		`INSERT INTO user_roles (id, user_id, role_id) VALUES ('ur2', 'carol', 'r_all')`,
		`UPDATE settings SET value = 'true' WHERE key = 'rbac_v3_enabled'`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	svc := NewService(database)
	policies := NewPolicyService(database)
	svc.SetPolicyService(policies)
	svc.SetAttributeBundle(NewAttributeBundle(nil, nil))
	svc.SetExecutor(NewABACExecutor(nil))
	svc.SetRolloutV3(NewRolloutV3(database))
	return database, svc, policies
}

// loadAgreementSet replaces every policy with set; rows are inserted directly because
// the editor's validation would refuse the broken expression the matrix needs.
func loadAgreementSet(t *testing.T, database *sql.DB, name string, set []agreementPolicy) {
	t.Helper()
	for _, q := range []string{`DELETE FROM rbac_policy_bindings`, `DELETE FROM rbac_policy_history`, `DELETE FROM rbac_policies`} {
		if _, err := database.Exec(q); err != nil {
			t.Fatalf("clearing policies: %v", err)
		}
	}
	for i, p := range set {
		id := fmt.Sprintf("%s-%d", name, i)
		if _, err := database.Exec(`INSERT INTO rbac_policies (id, name, description, effect, resource_type, expression, priority, enabled, created_by)
			VALUES (?, ?, '', ?, ?, ?, ?, 1, 'alice')`, id, id, p.effect, p.resourceType, p.expression, p.priority); err != nil {
			t.Fatalf("seeding policy %s: %v", id, err)
		}
		if _, err := database.Exec(`INSERT INTO rbac_policy_bindings (id, policy_id, permission_key) VALUES (?, ?, 'containers.manage')`, id+"-b", id); err != nil {
			t.Fatalf("binding policy %s: %v", id, err)
		}
	}
}

func countABACDenies(t *testing.T) *int {
	t.Helper()
	n := new(int)
	SetABACDeniedHook(func(AuthzRequest, string, *Attributes) { *n++ })
	t.Cleanup(func() { SetABACDeniedHook(nil) })
	return n
}

// TestExplainAndAuthorizeAgreeOnPolicyMatrix guards the shared evalPolicies walk: for
// every policy set, subject, environment and resource the simulator names the policy
// the live gate applies, reports the same decisions, and writes no audit of its own.
func TestExplainAndAuthorizeAgreeOnPolicyMatrix(t *testing.T) {
	database, svc, policyService := openAgreementDB(t)
	denies := countABACDenies(t)
	ctx := context.Background()
	perm := Permission("containers.manage")
	resources := [][2]string{{"", ""}, {"t_res", "r1"}, {"t_other", "o1"}}
	cases, abacDecided := 0, 0

	for setName, set := range agreementPolicySets {
		loadAgreementSet(t, database, setName, set)
		active, err := policyService.LoadActiveForPermission(ctx, string(perm))
		if err != nil || len(active) != len(set) {
			t.Fatalf("%s: loaded %d of %d policies: %v", setName, len(active), len(set), err)
		}
		for _, user := range []string{"alice", "bob", "carol"} {
			for _, env := range []string{"", "env-prod", "env-dev", "env-ghost"} {
				for _, res := range resources {
					name := fmt.Sprintf("%s/%s/%q/%q", setName, user, env, res[0])
					req := AuthzRequest{UserID: user, Permission: perm, EnvironmentID: env, ResourceType: res[0], ResourceID: res[1]}
					cases++

					*denies = 0
					matched, allow := svc.PolicyDecision(ctx, req)
					liveRecords := *denies
					authorized, err := svc.AuthorizeContext(ctx, req)
					if err != nil {
						t.Fatalf("%s: authorize: %v", name, err)
					}
					roles, err := svc.HasPermission(ctx, user, env, perm)
					if err != nil {
						t.Fatalf("%s: role check: %v", name, err)
					}
					gate := roles
					if matched {
						gate = allow
						abacDecided++
					}
					if authorized != gate {
						t.Errorf("%s: Authorize=%v but the route gate decides %v", name, authorized, gate)
					}
					if matched && !allow && liveRecords == 0 {
						t.Errorf("%s: a live deny must be audited", name)
					}

					*denies = 0
					picked := svc.explainMatchingPolicy(ctx, active, req)
					if (picked != nil) != matched {
						t.Errorf("%s: explain picked %v, gate matched=%v", name, picked, matched)
					} else if picked != nil && (picked.Effect != "deny") != allow {
						t.Errorf("%s: explain picked %s (%s), gate allow=%v", name, picked.ID, picked.Effect, allow)
					}
					if res[0] != "" {
						continue
					}
					got, err := svc.Explain(ctx, ExplainRequest{UserID: user, EnvironmentID: env, Permission: perm})
					if err != nil {
						t.Fatalf("%s: explain: %v", name, err)
					}
					if *denies != 0 {
						t.Errorf("%s: explain wrote %d ABAC audit records", name, *denies)
					}
					if got.Allowed != gate || got.PolicyAllowed != authorized {
						t.Errorf("%s: explain allowed=%v policyAllowed=%v, gate=%v authorize=%v", name, got.Allowed, got.PolicyAllowed, gate, authorized)
					}
					wantKind := ""
					if matched {
						wantKind = map[bool]string{true: ReasonABACAllow, false: ReasonABACDeny}[allow]
					}
					if gotKind := abacReasonKind(got.Reasons); gotKind != wantKind {
						t.Errorf("%s: explain ABAC reason %q, gate implies %q", name, gotKind, wantKind)
					}
				}
			}
		}
	}
	if abacDecided == 0 || abacDecided == cases {
		t.Fatalf("matrix must mix policy-decided and role-decided cases: %d of %d", abacDecided, cases)
	}
}

func abacReasonKind(rs []ExplainReason) string {
	for _, r := range rs {
		if r.Kind == ReasonABACAllow || r.Kind == ReasonABACDeny {
			return r.Kind
		}
	}
	return ""
}

// TestExplainKeepsEnvStubAndNoResource pins the two places the simulator's attributes
// deliberately differ from the route gate's: a failed environment lookup still exposes
// the id, and no resource is hydrated even when the request names one.
func TestExplainKeepsEnvStubAndNoResource(t *testing.T) {
	database, svc, policyService := openAgreementDB(t)
	ctx := context.Background()
	perm := Permission("containers.manage")

	loadAgreementSet(t, database, "noresource", []agreementPolicy{{"deny", "t_res", `resource.id == ""`, 10}})
	active, err := policyService.LoadActiveForPermission(ctx, string(perm))
	if err != nil {
		t.Fatal(err)
	}
	req := AuthzRequest{UserID: "bob", Permission: perm, EnvironmentID: "env-dev", ResourceType: "t_res", ResourceID: "r1"}
	if svc.explainMatchingPolicy(ctx, active, req) == nil {
		t.Fatal("explain must evaluate without a resource bundle")
	}
	if matched, _ := svc.PolicyDecision(ctx, req); matched {
		t.Fatal("the route gate stubs the named resource, so resource.id is r1")
	}

	loadAgreementSet(t, database, "envstub", []agreementPolicy{{"deny", "", `env.id == "env-dev"`, 10}})
	if active, err = policyService.LoadActiveForPermission(ctx, string(perm)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`ALTER TABLE environments RENAME TO environments_hidden`); err != nil {
		t.Fatal(err)
	}
	if svc.explainMatchingPolicy(ctx, active, AuthzRequest{UserID: "bob", Permission: perm, EnvironmentID: "env-dev"}) == nil {
		t.Fatal("a failed environment lookup must still expose env.id to explain")
	}
}

// TestRouteGateSeesEnvironmentID is the agreement case the matrix once had to leave
// out: an env-conditioned deny binds at the route gate as it does in Authorize and
// Explain, by id and by name.
func TestRouteGateSeesEnvironmentID(t *testing.T) {
	database, svc, _ := openAgreementDB(t)
	ctx := context.Background()
	for _, expr := range []string{`env.id == "env-prod"`, `env.name == "prod"`} {
		loadAgreementSet(t, database, "envdeny", []agreementPolicy{{"deny", "", expr, 10}})
		req := AuthzRequest{UserID: "carol", Permission: Permission("containers.manage"), EnvironmentID: "env-prod"}
		if matched, allow := svc.PolicyDecision(ctx, req); !matched || allow {
			t.Fatalf("%s: route gate matched=%v allow=%v, want an env-conditioned deny", expr, matched, allow)
		}
		if ok, err := svc.AuthorizeContext(ctx, req); err != nil || ok {
			t.Fatalf("%s: Authorize = %v, %v, want a deny", expr, ok, err)
		}
	}
}

// gateCall drives a real RequirePermission-gated route, so the test covers how the
// middleware reads ?env= as well as the decision itself.
func gateCall(t *testing.T, svc *Service, userID, envID string) int {
	t.Helper()
	r := chi.NewRouter()
	r.With(RequirePermission(svc, Permission("containers.manage"))).Post("/api/containers/restart",
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	req := httptest.NewRequest(http.MethodPost, "/api/containers/restart?env="+envID, nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: userID, Username: userID}))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec.Code
}

// TestRouteGateEnvPoliciesNeitherLeakNorOverGrant guards the security finding: a
// deny on env.name must block the route for a wildcard role, and an allow written as
// env.id != "env-prod" must not grant in prod, which it did while env.id read "".
func TestRouteGateEnvPoliciesNeitherLeakNorOverGrant(t *testing.T) {
	database, svc, _ := openAgreementDB(t)

	loadAgreementSet(t, database, "deny-name", []agreementPolicy{{"deny", "", `env.name == "prod"`, 10}})
	if got := gateCall(t, svc, "carol", "env-prod"); got != http.StatusForbidden {
		t.Fatalf("deny on env.name: carol in prod got %d, want 403", got)
	}
	if got := gateCall(t, svc, "carol", "env-dev"); got != http.StatusOK {
		t.Fatalf("deny on env.name: carol in dev got %d, want 200", got)
	}

	loadAgreementSet(t, database, "allow-id", []agreementPolicy{{"allow", "", `env.id != "env-prod"`, 10}})
	if got := gateCall(t, svc, "bob", "env-prod"); got != http.StatusForbidden {
		t.Fatalf("allow env.id != env-prod: bob (no role) in prod got %d, want 403", got)
	}
	if got := gateCall(t, svc, "bob", "env-dev"); got != http.StatusOK {
		t.Fatalf("allow env.id != env-prod: bob in dev got %d, want the policy to grant", got)
	}
}

// TestUnresolvedEnvironmentFailsClosed pins the choice made for a failed environment
// read: env.id still decides, but a policy reading env.name or env.type can deny and
// can never grant, so an outage cannot turn `env.name != "prod"` into a prod grant.
func TestUnresolvedEnvironmentFailsClosed(t *testing.T) {
	database, svc, policyService := openAgreementDB(t)
	ctx := context.Background()
	perm := Permission("containers.manage")
	if _, err := database.Exec(`ALTER TABLE environments RENAME TO environments_hidden`); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name, user, env string
		set             []agreementPolicy
		want            int
	}{
		{"allow on name never grants", "bob", "env-prod", []agreementPolicy{{"allow", "", `env.name != "prod"`, 10}}, http.StatusForbidden},
		{"allow on type never grants", "bob", "env-dev", []agreementPolicy{{"allow", "", `env["type"] == "tcp"`, 10}}, http.StatusForbidden},
		{"deny on name applies", "carol", "env-dev", []agreementPolicy{{"deny", "", `env.name == "prod"`, 10}}, http.StatusForbidden},
		{"allow on id still grants", "bob", "env-dev", []agreementPolicy{{"allow", "", `env.id != "env-prod"`, 10}}, http.StatusOK},
		{"allow on id still refuses prod", "bob", "env-prod", []agreementPolicy{{"allow", "", `env.id != "env-prod"`, 10}}, http.StatusForbidden},
		{"deny on id is evaluated", "carol", "env-dev", []agreementPolicy{{"deny", "", `env.id == "env-prod"`, 10}}, http.StatusOK},
	}
	for _, c := range cases {
		loadAgreementSet(t, database, "unresolved", c.set)
		if got := gateCall(t, svc, c.user, c.env); got != c.want {
			t.Errorf("%s: route gate %d, want %d", c.name, got, c.want)
		}
		req := AuthzRequest{UserID: c.user, Permission: perm, EnvironmentID: c.env}
		if ok, err := svc.AuthorizeContext(ctx, req); err != nil || ok != (c.want == http.StatusOK) {
			t.Errorf("%s: Authorize = %v, %v", c.name, ok, err)
		}
		active, err := policyService.LoadActiveForPermission(ctx, string(perm))
		if err != nil {
			t.Fatal(err)
		}
		matched, allow := svc.PolicyDecision(ctx, req)
		picked := svc.explainMatchingPolicy(ctx, active, req)
		if (picked != nil) != matched || (picked != nil && (picked.Effect != "deny") != allow) {
			t.Errorf("%s: explain picked %v, gate matched=%v allow=%v", c.name, picked, matched, allow)
		}
	}
}

// TestFetchEnvAttributesReadsConnectionType guards the column the lookup selects:
// env.type is the connection type, and a missing row is the bare id, not an error.
func TestFetchEnvAttributesReadsConnectionType(t *testing.T) {
	database, _, _ := openAgreementDB(t)
	e, err := FetchEnvAttributes(context.Background(), database, "env-dev")
	if err != nil || e.ID != "env-dev" || e.Name != "dev" || e.Type != "tcp" {
		t.Fatalf("FetchEnvAttributes = %+v, %v", e, err)
	}
	if e, err = FetchEnvAttributes(context.Background(), database, "env-ghost"); err != nil || e.ID != "env-ghost" || e.Name != "" {
		t.Fatalf("missing env = %+v, %v, want the bare id", e, err)
	}
}

func TestReadsUnresolvedEnv(t *testing.T) {
	for expr, want := range map[string]bool{
		`env.id == "x"`:                        false,
		`env . id != "x" && user.id == "a"`:    false,
		`user.username == "bob"`:               false,
		`env.name == "prod"`:                   true,
		`env.id == "x" || env.type == "agent"`: true,
		`env["name"] == "prod"`:                true,
		`env.identity == "x"`:                  true,
		`JSON.stringify(env).length > 0`:       true,
		`"env-prod" != env.id`:                 false,
		`resource.labels["env"] == 'env'`:      false,
		"`${env.name}` == \"prod\"":            true,
	} {
		if got := readsUnresolvedEnv(expr); got != want {
			t.Errorf("readsUnresolvedEnv(%q) = %v, want %v", expr, got, want)
		}
	}
}
