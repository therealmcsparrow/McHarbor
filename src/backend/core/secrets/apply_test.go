// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package secrets

import (
	"context"
	"errors"
	"testing"
)

// recordingApplier stands in for the containers and stacks modules, which
// core/secrets cannot import. It records what it was asked to do and can be
// told to fail on a named resource.
type recordingApplier struct {
	applied  []string
	failOn   string
	failWith error
	// gone names resources Resolve reports as removed.
	gone map[string]bool
	// canonical maps an identifier onto what it really points at, standing in
	// for a Docker inspect that resolves both a name and an id to one
	// container. An identifier absent from the map resolves to itself.
	canonical map[string]string
}

func (a *recordingApplier) Apply(_ context.Context, target ApplyTarget) error {
	if target.ResourceName == a.failOn {
		return a.failWith
	}
	a.applied = append(a.applied, target.ResourceName)
	return nil
}

func (a *recordingApplier) Resolve(_ context.Context, target ApplyTarget) (string, bool, error) {
	if a.gone[target.ResourceID] || a.gone[target.ResourceName] {
		return "", false, nil
	}
	if id, ok := a.canonical[target.ResourceID]; ok {
		return id, true, nil
	}
	return target.ResourceID, true, nil
}

// blindApplier implements no probe, so its resources can never be reported
// missing and are never merged. It guards the optional half of the
// ResourceProber contract.
type blindApplier struct{}

func (blindApplier) Apply(context.Context, ApplyTarget) error { return nil }

// seedReference records one resolved field, the way recordUse does at deploy
// time, without going through a real Docker call.
func seedReference(t *testing.T, svc *Service, secretID, resourceType, id, name, field string, version int) {
	t.Helper()
	if err := svc.TouchReference(context.Background(), SecretReference{
		SecretID:       secretID,
		ResourceType:   resourceType,
		ResourceID:     id,
		ResourceName:   &name,
		Field:          &field,
		AppliedVersion: version,
	}); err != nil {
		t.Fatalf("seeding reference %s/%s: %v", resourceType, name, err)
	}
}

// TestImpactGroupsByResourceAndMarksStale is the core of the visibility half:
// a stack with four variables pointing at one secret is one resource, and the
// oldest of its fields decides whether it is stale.
func TestImpactGroupsByResourceAndMarksStale(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	sec, _, err := svc.Create(ctx, "user1", "", CreateRequest{
		Name: "DB_PASSWORD", Scope: ScopeGlobal, Value: "v1",
	})
	if err != nil {
		t.Fatalf("creating secret: %v", err)
	}
	if _, _, err := svc.Rotate(ctx, "user1", "", sec.ID, RotateRequest{Value: "v2"}); err != nil {
		t.Fatalf("rotating secret: %v", err)
	}

	// One stack, two fields, one of them stamped with the old version.
	seedReference(t, svc, sec.ID, "stack", "stk1", "wordpress", "WORDPRESS_DB_PASSWORD", 2)
	seedReference(t, svc, sec.ID, "stack", "stk1", "wordpress", "MYSQL_ROOT_PASSWORD", 1)
	// A container already on the current version.
	seedReference(t, svc, sec.ID, "container", "cnt1", "pgadmin", "PGADMIN_PASSWORD", 2)

	reg := NewApplierRegistry()
	reg.Register("stack", &recordingApplier{})

	impact, err := svc.Impact(ctx, reg, sec.ID)
	if err != nil {
		t.Fatalf("computing impact: %v", err)
	}

	if len(impact.Resources) != 2 {
		t.Fatalf("expected 2 resources from 3 references, got %d", len(impact.Resources))
	}
	if impact.StaleCount != 1 {
		t.Fatalf("expected 1 stale resource, got %d", impact.StaleCount)
	}

	byName := map[string]ImpactResource{}
	for _, resource := range impact.Resources {
		byName[resource.ResourceName] = resource
	}

	stack := byName["wordpress"]
	if !stack.Stale {
		t.Error("a resource with one field on an older version is stale")
	}
	if len(stack.Fields) != 2 {
		t.Errorf("expected both fields on the stack, got %v", stack.Fields)
	}
	if !stack.Supported {
		t.Error("a registered resource type is supported")
	}

	container := byName["pgadmin"]
	if container.Stale {
		t.Error("a resource stamped with the current version is not stale")
	}
	if container.Supported {
		t.Error("an unregistered resource type must be reported unsupported, not applied")
	}
}

// TestImpactTreatsUnstampedReferencesAsStale covers the rows migration 121
// left at zero. Claiming they are current would hide exactly the drift the
// column was added to surface.
func TestImpactTreatsUnstampedReferencesAsStale(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	sec, _, err := svc.Create(ctx, "user1", "", CreateRequest{
		Name: "API_TOKEN", Scope: ScopeGlobal, Value: "v1",
	})
	if err != nil {
		t.Fatalf("creating secret: %v", err)
	}
	seedReference(t, svc, sec.ID, "stack", "stk1", "legacy", "TOKEN", 0)

	impact, err := svc.Impact(ctx, NewApplierRegistry(), sec.ID)
	if err != nil {
		t.Fatalf("computing impact: %v", err)
	}
	if impact.StaleCount != 1 {
		t.Fatalf("an unstamped reference must be stale, got staleCount=%d", impact.StaleCount)
	}
}

// TestApplyContinuesPastAFailure is the behaviour the operator asked for: one
// resource failing says nothing about the next, so the run finishes and every
// resource carries its own verdict.
func TestApplyContinuesPastAFailure(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	sec, _, err := svc.Create(ctx, "user1", "", CreateRequest{
		Name: "DB_PASSWORD", Scope: ScopeGlobal, Value: "v1",
	})
	if err != nil {
		t.Fatalf("creating secret: %v", err)
	}
	if _, _, err := svc.Rotate(ctx, "user1", "", sec.ID, RotateRequest{Value: "v2"}); err != nil {
		t.Fatalf("rotating secret: %v", err)
	}

	// Impact sorts by type then name, so these run alpha, bravo, charlie.
	for _, name := range []string{"alpha", "bravo", "charlie"} {
		seedReference(t, svc, sec.ID, "stack", "stk-"+name, name, "PASSWORD", 1)
	}

	applier := &recordingApplier{failOn: "bravo", failWith: errors.New("compose exited 1")}
	reg := NewApplierRegistry()
	reg.Register("stack", applier)

	results, err := svc.Apply(ctx, reg, sec.ID, nil)
	if err != nil {
		t.Fatalf("applying: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("every selected resource must be reported, got %d", len(results))
	}
	if results[0].Status != ApplyStatusApplied {
		t.Errorf("alpha should be applied, got %s", results[0].Status)
	}
	if results[1].Status != ApplyStatusFailed {
		t.Errorf("bravo should be failed, got %s", results[1].Status)
	}
	if results[2].Status != ApplyStatusApplied {
		t.Errorf("charlie must still be attempted after bravo failed, got %s", results[2].Status)
	}
	if len(applier.applied) != 2 || applier.applied[0] != "alpha" || applier.applied[1] != "charlie" {
		t.Errorf("everything but the failure should run, applied=%v", applier.applied)
	}
	if results[1].Detail == "" {
		t.Error("the failure detail is what the server log needs")
	}
}

// TestApplyOnlyTouchesStaleResourcesByDefault keeps the primary button honest:
// "apply" must not restart something that is already correct.
func TestApplyOnlyTouchesStaleResourcesByDefault(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	sec, _, err := svc.Create(ctx, "user1", "", CreateRequest{
		Name: "DB_PASSWORD", Scope: ScopeGlobal, Value: "v1",
	})
	if err != nil {
		t.Fatalf("creating secret: %v", err)
	}
	if _, _, err := svc.Rotate(ctx, "user1", "", sec.ID, RotateRequest{Value: "v2"}); err != nil {
		t.Fatalf("rotating secret: %v", err)
	}

	seedReference(t, svc, sec.ID, "stack", "stk1", "old", "PASSWORD", 1)
	seedReference(t, svc, sec.ID, "stack", "stk2", "current", "PASSWORD", 2)

	applier := &recordingApplier{}
	reg := NewApplierRegistry()
	reg.Register("stack", applier)

	results, err := svc.Apply(ctx, reg, sec.ID, nil)
	if err != nil {
		t.Fatalf("applying: %v", err)
	}
	if len(results) != 1 || results[0].ResourceName != "old" {
		t.Fatalf("only the stale resource should be applied, got %+v", results)
	}

	// Naming a resource explicitly overrides that: an operator who asks for a
	// redeploy of something current is entitled to get one.
	results, err = svc.Apply(ctx, reg, sec.ID, []ApplyTargetRef{
		{ResourceType: "stack", ResourceID: "stk2"},
	})
	if err != nil {
		t.Fatalf("applying named resource: %v", err)
	}
	if len(results) != 1 || results[0].ResourceName != "current" {
		t.Fatalf("an explicitly named resource should be applied, got %+v", results)
	}
}

// TestRebindReferenceMovesToTheContainerID covers the identifier handover: a
// container is resolved before it exists, so its references start life keyed
// on the name and have to survive the move onto the id.
func TestRebindReferenceMovesToTheContainerID(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	sec, _, err := svc.Create(ctx, "user1", "", CreateRequest{
		Name: "DB_PASSWORD", Scope: ScopeGlobal, Value: "v1",
	})
	if err != nil {
		t.Fatalf("creating secret: %v", err)
	}
	seedReference(t, svc, sec.ID, "container", "postgres", "postgres", "POSTGRES_PASSWORD", 1)

	if err := svc.RebindReference(ctx, "container", "postgres", "abc123", "postgres"); err != nil {
		t.Fatalf("rebinding: %v", err)
	}

	refs, err := svc.References(ctx, sec.ID)
	if err != nil {
		t.Fatalf("listing references: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("rebinding must move the row, not duplicate it: got %d", len(refs))
	}
	if refs[0].ResourceID != "abc123" {
		t.Errorf("expected the container id, got %s", refs[0].ResourceID)
	}
	if refs[0].ResourceName == nil || *refs[0].ResourceName != "postgres" {
		t.Errorf("the readable name must survive the rebind, got %v", refs[0].ResourceName)
	}

	// Re-resolving after the rebind has to find the same row rather than
	// inserting a second one, which is what the recomputed ref_key buys.
	field := "POSTGRES_PASSWORD"
	name := "postgres"
	if err := svc.TouchReference(ctx, SecretReference{
		SecretID: sec.ID, ResourceType: "container", ResourceID: "abc123",
		ResourceName: &name, Field: &field, AppliedVersion: 1,
	}); err != nil {
		t.Fatalf("re-touching after rebind: %v", err)
	}
	refs, err = svc.References(ctx, sec.ID)
	if err != nil {
		t.Fatalf("listing references after re-touch: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("the rebound key must still be the upsert target, got %d rows", len(refs))
	}
}

// TestAutoApplyDefaultsOff is a one-line test guarding a decision that matters
// more than its size: a rotation must not restart anything unless asked.
func TestAutoApplyDefaultsOff(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	sec, _, err := svc.Create(ctx, "user1", "", CreateRequest{
		Name: "DB_PASSWORD", Scope: ScopeGlobal, Value: "v1",
	})
	if err != nil {
		t.Fatalf("creating secret: %v", err)
	}
	if sec.AutoApply {
		t.Fatal("auto-apply must be off unless the request asks for it")
	}

	enabled, err := svc.AutoApplyEnabled(ctx, sec.ID)
	if err != nil {
		t.Fatalf("reading the flag: %v", err)
	}
	if enabled {
		t.Fatal("AutoApplyEnabled must agree with the created row")
	}

	on := true
	patched, err := svc.Patch(ctx, "user1", "", sec.ID, PatchRequest{AutoApply: &on})
	if err != nil {
		t.Fatalf("patching: %v", err)
	}
	if !patched.AutoApply {
		t.Fatal("patching auto-apply on must stick")
	}
}

// TestImpactMarksMissingResources covers the self-healing half: a reference to
// a container that no longer exists is reported gone, and is deliberately not
// counted as stale — there is nothing left to bring up to date.
func TestImpactMarksMissingResources(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	sec, _, err := svc.Create(ctx, "user1", "", CreateRequest{
		Name: "DB_PASSWORD", Scope: ScopeGlobal, Value: "v1",
	})
	if err != nil {
		t.Fatalf("creating secret: %v", err)
	}
	if _, _, err := svc.Rotate(ctx, "user1", "", sec.ID, RotateRequest{Value: "v2"}); err != nil {
		t.Fatalf("rotating secret: %v", err)
	}

	seedReference(t, svc, sec.ID, "container", "cnt-live", "live", "PASSWORD", 1)
	seedReference(t, svc, sec.ID, "container", "cnt-dead", "dead", "PASSWORD", 1)

	reg := NewApplierRegistry()
	reg.Register("container", &recordingApplier{gone: map[string]bool{"cnt-dead": true}})

	impact, err := svc.Impact(ctx, reg, sec.ID)
	if err != nil {
		t.Fatalf("computing impact: %v", err)
	}
	if impact.MissingCount != 1 {
		t.Fatalf("expected 1 missing resource, got %d", impact.MissingCount)
	}
	if impact.StaleCount != 1 {
		t.Fatalf("a missing resource must not count as stale, staleCount=%d", impact.StaleCount)
	}

	for _, resource := range impact.Resources {
		if resource.ResourceName == "dead" && !resource.Missing {
			t.Error("the removed container should be reported missing")
		}
		if resource.ResourceName == "dead" && resource.Stale {
			t.Error("a missing resource is never stale")
		}
	}
}

// TestApplySkipsMissingResources: a gone resource cannot be applied to even
// when named explicitly, because there is nothing there to recreate.
func TestApplySkipsMissingResources(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	sec, _, err := svc.Create(ctx, "user1", "", CreateRequest{
		Name: "DB_PASSWORD", Scope: ScopeGlobal, Value: "v1",
	})
	if err != nil {
		t.Fatalf("creating secret: %v", err)
	}
	seedReference(t, svc, sec.ID, "container", "cnt-dead", "dead", "PASSWORD", 0)

	applier := &recordingApplier{gone: map[string]bool{"cnt-dead": true}}
	reg := NewApplierRegistry()
	reg.Register("container", applier)

	results, err := svc.Apply(ctx, reg, sec.ID, []ApplyTargetRef{
		{ResourceType: "container", ResourceID: "cnt-dead"},
	})
	if err != nil {
		t.Fatalf("applying: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("a missing resource must not be applied to, got %+v", results)
	}
	if len(applier.applied) != 0 {
		t.Fatalf("the applier should never have been called, applied=%v", applier.applied)
	}
}

// TestImpactCollapsesDuplicateIdentities reproduces the exact shape the live
// database had: one WordPress container recorded under its name and under the
// id of three incarnations, two of them dead. It must read as one container.
func TestImpactCollapsesDuplicateIdentities(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	sec, _, err := svc.Create(ctx, "user1", "", CreateRequest{
		Name: "WORDPRESS_DB_PASSWORD", Scope: ScopeGlobal, Value: "v1",
	})
	if err != nil {
		t.Fatalf("creating secret: %v", err)
	}

	seedReference(t, svc, sec.ID, "container", "wordpress", "wordpress", "WORDPRESS_DB_USER", 0)
	seedReference(t, svc, sec.ID, "container", "33ce9d31", "33ce9d31", "WORDPRESS_DB_USER", 0)
	seedReference(t, svc, sec.ID, "container", "6bd93eb6", "6bd93eb6", "WORDPRESS_DB_PASSWORD", 0)
	seedReference(t, svc, sec.ID, "container", "5fcb671b", "wordpress", "WORDPRESS_DB_PASSWORD", 1)
	seedReference(t, svc, sec.ID, "container", "5fcb671b", "wordpress", "WORDPRESS_DB_USER", 1)

	reg := NewApplierRegistry()
	reg.Register("container", &recordingApplier{
		// The name and the live id are the same container; the other two ids
		// are corpses.
		canonical: map[string]string{"wordpress": "5fcb671b"},
		gone:      map[string]bool{"33ce9d31": true, "6bd93eb6": true},
	})

	impact, err := svc.Impact(ctx, reg, sec.ID)
	if err != nil {
		t.Fatalf("computing impact: %v", err)
	}

	if len(impact.Resources) != 3 {
		t.Fatalf("expected the live container plus two corpses, got %d: %+v",
			len(impact.Resources), impact.Resources)
	}

	var live *ImpactResource
	for i := range impact.Resources {
		if impact.Resources[i].ResourceID == "5fcb671b" {
			live = &impact.Resources[i]
		}
	}
	if live == nil {
		t.Fatal("the live container should be keyed on its own id")
	}
	if len(live.SupersededIDs) != 1 || live.SupersededIDs[0] != "wordpress" {
		t.Errorf("the name-keyed row should be folded in, got %v", live.SupersededIDs)
	}
	if len(live.Fields) != 2 {
		t.Errorf("both fields should survive the merge, got %v", live.Fields)
	}
	if live.Missing {
		t.Error("the live container is not missing")
	}

	if impact.MissingCount != 2 {
		t.Fatalf("expected 2 missing resources, got %d", impact.MissingCount)
	}
	// Two corpse rows plus the one superseded duplicate.
	if impact.PrunableCount != 3 {
		t.Fatalf("expected 3 prunable references, got %d", impact.PrunableCount)
	}
}

// TestPruneRemovesCorpsesAndDuplicatesOnly is the guard on the cleanup: it must
// take the dead rows and the duplicates, and leave the one that is real.
func TestPruneRemovesCorpsesAndDuplicatesOnly(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	sec, _, err := svc.Create(ctx, "user1", "", CreateRequest{
		Name: "WORDPRESS_DB_PASSWORD", Scope: ScopeGlobal, Value: "v1",
	})
	if err != nil {
		t.Fatalf("creating secret: %v", err)
	}

	seedReference(t, svc, sec.ID, "container", "wordpress", "wordpress", "WORDPRESS_DB_USER", 0)
	seedReference(t, svc, sec.ID, "container", "33ce9d31", "33ce9d31", "WORDPRESS_DB_USER", 0)
	seedReference(t, svc, sec.ID, "container", "5fcb671b", "wordpress", "WORDPRESS_DB_USER", 1)

	reg := NewApplierRegistry()
	reg.Register("container", &recordingApplier{
		canonical: map[string]string{"wordpress": "5fcb671b"},
		gone:      map[string]bool{"33ce9d31": true},
	})

	pruned, err := svc.PruneReferences(ctx, reg, sec.ID, nil)
	if err != nil {
		t.Fatalf("pruning: %v", err)
	}
	if pruned != 2 {
		t.Fatalf("expected the corpse and the duplicate to go, pruned %d", pruned)
	}

	refs, err := svc.References(ctx, sec.ID)
	if err != nil {
		t.Fatalf("listing references: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("exactly the live reference should survive, got %d", len(refs))
	}
	if refs[0].ResourceID != "5fcb671b" {
		t.Errorf("the surviving reference should be the live container, got %s", refs[0].ResourceID)
	}
}

// TestImpactToleratesAnApplierWithNoProbe: Exists is optional, and an applier
// that does not implement it must leave its resources unmarked rather than
// having them reported gone.
func TestImpactToleratesAnApplierWithNoProbe(t *testing.T) {
	svc, _ := newTestService(t, fakeAuthorizer{global: true})
	ctx := context.Background()

	sec, _, err := svc.Create(ctx, "user1", "", CreateRequest{
		Name: "DB_PASSWORD", Scope: ScopeGlobal, Value: "v1",
	})
	if err != nil {
		t.Fatalf("creating secret: %v", err)
	}
	seedReference(t, svc, sec.ID, "stack", "stk1", "wordpress", "PASSWORD", 1)

	reg := NewApplierRegistry()
	reg.Register("stack", blindApplier{})

	impact, err := svc.Impact(ctx, reg, sec.ID)
	if err != nil {
		t.Fatalf("computing impact: %v", err)
	}
	if impact.MissingCount != 0 {
		t.Fatalf("an applier that cannot probe must report nothing missing, got %d", impact.MissingCount)
	}
}
