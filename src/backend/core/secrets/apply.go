// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package secrets

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// ErrNoApplier reports that nothing knows how to re-materialise a resource
// type. It is not a failure of the rotation — a workflow reads its secrets at
// execution time and needs no applying at all — so the impact list marks those
// resources unsupported rather than stale-forever.
var ErrNoApplier = errors.New("no applier is registered for this resource type")

// ApplyTarget is one resource to re-materialise so it picks up the current
// value. It carries the name alongside the id because an applier may have to
// fall back to the name: references written before the id rebind landed, and
// stacks, which are addressed by name throughout.
type ApplyTarget struct {
	ResourceType  string
	ResourceID    string
	ResourceName  string
	EnvironmentID string
	SecretID      string
	SecretName    string
}

// ResourceProber answers whether a resource a reference points at still exists.
//
// Optional, and separate from Applier on purpose: an applier that cannot answer
// cheaply simply does not implement it, and its resources are then never
// reported missing rather than being reported wrongly.
type ResourceProber interface {
	// Resolve returns the resource's canonical identifier and whether it still
	// exists.
	//
	// The identifier matters as much as the boolean. A container is reachable
	// by name and by id, and the vault has historically recorded both, so two
	// reference rows can point at one running container. Returning the id it
	// actually resolved to is what lets the impact list collapse them instead
	// of showing the same container twice.
	//
	// An error means "could not tell" and leaves the reference alone — marking
	// a live resource missing would hide a genuinely stale one behind a prune
	// prompt.
	Resolve(ctx context.Context, target ApplyTarget) (canonicalID string, exists bool, err error)
}

// Applier re-materialises one resource type. Implemented by the containers and
// stacks modules, which core/secrets must not import — hence the interface and
// the registry below rather than a direct call.
type Applier interface {
	// Apply makes the resource run with the secret's current value. It is
	// allowed to be slow: a container recreate or a compose deploy is what it
	// usually is, and the caller runs targets one at a time for that reason.
	Apply(ctx context.Context, target ApplyTarget) error
}

// ApplierRegistry maps a resource type onto the module that can re-materialise
// it. One instance lives on AppDeps; each module registers itself at mount
// time, so the wiring direction matches PluginRoutes rather than inverting the
// core/module dependency.
type ApplierRegistry struct {
	mu  sync.RWMutex
	all map[string]Applier
}

// NewApplierRegistry returns an empty registry. A registry with nothing in it
// is valid and reports every resource as unsupported, which is the correct
// behaviour during start-up before the modules have mounted.
func NewApplierRegistry() *ApplierRegistry {
	return &ApplierRegistry{all: map[string]Applier{}}
}

// Register publishes an applier for one resource type, replacing any previous
// registration. Safe on a nil registry so a module need not guard its mount.
func (r *ApplierRegistry) Register(resourceType string, applier Applier) {
	if r == nil || applier == nil || resourceType == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.all == nil {
		r.all = map[string]Applier{}
	}
	r.all[resourceType] = applier
}

// Lookup returns the applier for a resource type, if one is registered.
func (r *ApplierRegistry) Lookup(resourceType string) (Applier, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	applier, ok := r.all[resourceType]
	return applier, ok
}

// ImpactResource is one resource that references a secret, with enough detail
// for an operator to decide whether to restart it.
type ImpactResource struct {
	ResourceType   string   `json:"resourceType"`
	ResourceID     string   `json:"resourceId"`
	ResourceName   string   `json:"resourceName"`
	EnvironmentID  *string  `json:"environmentId,omitempty"`
	Fields         []string `json:"fields"`
	AppliedVersion int      `json:"appliedVersion"`
	// Stale is true when the resource runs an older version than the secret
	// currently holds. A reference with no stamp counts as stale: it predates
	// the column, so its version is genuinely unknown.
	Stale bool `json:"stale"`
	// Missing is true when the applier reports the resource no longer exists.
	// A missing resource is never stale — there is nothing left to update — so
	// it stops inflating the out-of-date count and cannot be selected.
	Missing bool `json:"missing"`
	// SupersededIDs are the other identifiers whose rows point at this same
	// resource — a container's name beside the id of every incarnation a
	// recreate replaced. They are merged into this entry and are what a prune
	// deletes.
	SupersededIDs []string `json:"supersededIds,omitempty"`
	// Supported reports whether an applier exists for this resource type.
	// An unsupported resource is shown and never offered an apply button.
	Supported bool `json:"supported"`
}

// Impact answers "what would a rotation of this secret affect, and what is
// already out of date". It is the read half of the feature and costs one
// query — it is safe to poll from the UI.
type Impact struct {
	SecretID   string           `json:"secretId"`
	SecretName string           `json:"secretName"`
	Version    int              `json:"version"`
	AutoApply  bool             `json:"autoApply"`
	Resources  []ImpactResource `json:"resources"`
	StaleCount int              `json:"staleCount"`
	// MissingCount is how many resources point at something that is gone.
	MissingCount int `json:"missingCount"`
	// PrunableCount is how many reference rows a prune would delete: the ones
	// belonging to resources that are gone, plus every superseded duplicate.
	// It is the number the prune button shows, because it is what changes.
	PrunableCount int `json:"prunableCount"`
}

// ApplyRequest is the body of POST /api/secrets/{id}/apply. An empty Resources
// means "every stale resource", which is what the dialog's primary button
// sends; naming resources explicitly is how a cautious operator restarts one
// stack now and the rest after lunch.
type ApplyRequest struct {
	Resources []ApplyTargetRef `json:"resources"`
}

// ApplyTargetRef names one resource in an apply request.
type ApplyTargetRef struct {
	ResourceType string `json:"resourceType"`
	ResourceID   string `json:"resourceId"`
}

// ApplyStatus values, mirrored by the frontend's SecretApplyResult type and
// translated there under `apply.status.*`. A status rather than a message,
// because the alternative is a Docker or compose error on its way to a
// browser — and those carry image names, socket paths and host directories.
const (
	ApplyStatusApplied     = "applied"
	ApplyStatusFailed      = "failed"
	ApplyStatusUnsupported = "unsupported"
)

// ApplyResult is what happened to one resource.
type ApplyResult struct {
	ResourceType string `json:"resourceType"`
	ResourceID   string `json:"resourceId"`
	ResourceName string `json:"resourceName"`
	Status       string `json:"status"`
	// Detail is the real reason, for the server log and the audit entry only.
	// `json:"-"` rather than omitempty: a field that cannot be serialised
	// cannot leak, and every future edit to this struct inherits that.
	Detail string `json:"-"`
}

// Impact groups a secret's references by resource and marks the stale ones.
//
// References are per field, so a stack with four variables pointing at one
// secret produces four rows and exactly one resource.
func (s *Service) Impact(ctx context.Context, reg *ApplierRegistry, secretID string) (*Impact, error) {
	sec, err := s.ByID(ctx, secretID)
	if err != nil {
		return nil, err
	}
	refs, err := s.References(ctx, secretID)
	if err != nil {
		return nil, err
	}

	out := &Impact{
		SecretID:   sec.ID,
		SecretName: sec.Name,
		Version:    sec.Version,
		AutoApply:  sec.AutoApply,
		Resources:  []ImpactResource{},
	}

	byResource := map[string]*ImpactResource{}
	for _, ref := range refs {
		key := ref.ResourceType + "\x1f" + ref.ResourceID
		resource, seen := byResource[key]
		if !seen {
			name := ref.ResourceID
			if ref.ResourceName != nil && *ref.ResourceName != "" {
				name = *ref.ResourceName
			}
			_, supported := reg.Lookup(ref.ResourceType)
			resource = &ImpactResource{
				ResourceType:   ref.ResourceType,
				ResourceID:     ref.ResourceID,
				ResourceName:   name,
				EnvironmentID:  ref.EnvironmentID,
				Fields:         []string{},
				AppliedVersion: ref.AppliedVersion,
				Supported:      supported,
			}
			byResource[key] = resource
		}
		if ref.Field != nil && *ref.Field != "" {
			resource.Fields = append(resource.Fields, *ref.Field)
		}
		// The oldest field wins: one variable still carrying an old value is
		// enough to make the whole resource stale.
		if ref.AppliedVersion < resource.AppliedVersion {
			resource.AppliedVersion = ref.AppliedVersion
		}
	}

	for _, resource := range byResource {
		sort.Strings(resource.Fields)
		out.Resources = append(out.Resources, *resource)
	}

	out.Resources = collapseToCanonical(ctx, reg, impactTarget(sec), out.Resources)

	for i := range out.Resources {
		resource := &out.Resources[i]
		// Missing beats stale. There is nothing left to bring up to date, and
		// counting it would keep the badge lit on a container that is gone.
		resource.Stale = !resource.Missing && resource.AppliedVersion < sec.Version
		if resource.Stale {
			out.StaleCount++
		}
		if resource.Missing {
			out.MissingCount++
			out.PrunableCount++
		}
		out.PrunableCount += len(resource.SupersededIDs)
	}
	sort.Slice(out.Resources, func(i, j int) bool {
		if out.Resources[i].ResourceType != out.Resources[j].ResourceType {
			return out.Resources[i].ResourceType < out.Resources[j].ResourceType
		}
		return out.Resources[i].ResourceName < out.Resources[j].ResourceName
	})
	return out, nil
}

// applyTimeout bounds one resource. A compose deploy that pulls images is the
// slow case; beyond this the operator is better served by an error naming the
// resource than by a request that never returns.
const applyTimeout = 10 * time.Minute

// probeTimeout bounds one existence check, and probeWorkers bounds how many run
// at once. Both exist because a probe on an agent environment is a round trip
// over the WebSocket transport: sequentially, a secret used by thirty
// containers would make the impact dialog take a minute to open.
const (
	probeTimeout = 10 * time.Second
	probeWorkers = 8
)

// impactTarget builds the ApplyTarget skeleton every probe and apply for this
// secret shares, so the secret's identity is filled in one place.
func impactTarget(sec *Secret) ApplyTarget {
	return ApplyTarget{SecretID: sec.ID, SecretName: sec.Name}
}

// probeResult is what one applier said about one resource.
type probeResult struct {
	canonicalID string
	exists      bool
	answered    bool
}

// collapseToCanonical asks each resource's applier what it really is, then
// merges the entries that turn out to be the same thing.
//
// This is what turns "one WordPress container, four rows" back into one row.
// The references were written under whatever identifier the resolver happened
// to hold — the container's name before it existed, the id of each incarnation
// afterwards — and nothing ever reconciled them.
func collapseToCanonical(
	ctx context.Context, reg *ApplierRegistry, base ApplyTarget, resources []ImpactResource,
) []ImpactResource {
	if len(resources) == 0 {
		return resources
	}

	probes := probeAll(ctx, reg, base, resources)

	// Group by what the applier says each row actually points at. A row nobody
	// could resolve keeps its own identity and stands alone.
	groups := map[string][]int{}
	order := []string{}
	for i := range resources {
		key := resources[i].ResourceType + "\x1f" + resources[i].ResourceID
		if probes[i].answered && probes[i].exists && probes[i].canonicalID != "" {
			key = resources[i].ResourceType + "\x1f" + probes[i].canonicalID
		}
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], i)
	}

	out := make([]ImpactResource, 0, len(order))
	for _, key := range order {
		members := groups[key]

		// Prefer the row already keyed on the canonical identifier: it is the
		// one a later resolve will upsert into, so keeping it means the next
		// deploy updates this entry rather than minting a fifth.
		keeper := members[0]
		for _, i := range members {
			if probes[i].answered && resources[i].ResourceID == probes[i].canonicalID {
				keeper = i
				break
			}
		}

		merged := resources[keeper]
		merged.Missing = probes[keeper].answered && !probes[keeper].exists
		if probes[keeper].answered && probes[keeper].canonicalID != "" && !merged.Missing {
			merged.ResourceID = probes[keeper].canonicalID
		}

		seenField := map[string]bool{}
		for _, field := range merged.Fields {
			seenField[field] = true
		}
		for _, i := range members {
			if i == keeper {
				continue
			}
			merged.SupersededIDs = append(merged.SupersededIDs, resources[i].ResourceID)
			for _, field := range resources[i].Fields {
				if !seenField[field] {
					seenField[field] = true
					merged.Fields = append(merged.Fields, field)
				}
			}
			// The oldest identity decides: one row still on an older version
			// means the resource has not fully caught up.
			if resources[i].AppliedVersion < merged.AppliedVersion {
				merged.AppliedVersion = resources[i].AppliedVersion
			}
			if merged.ResourceName == merged.ResourceID && resources[i].ResourceName != resources[i].ResourceID {
				merged.ResourceName = resources[i].ResourceName
			}
		}
		sort.Strings(merged.Fields)
		sort.Strings(merged.SupersededIDs)
		out = append(out, merged)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].ResourceType != out[j].ResourceType {
			return out[i].ResourceType < out[j].ResourceType
		}
		return out[i].ResourceName < out[j].ResourceName
	})
	return out
}

// probeAll resolves every resource concurrently.
//
// Bounded on purpose: a probe on an agent environment is a round trip over the
// WebSocket transport, and asking about thirty containers in series is what
// makes a dialog feel broken.
func probeAll(
	ctx context.Context, reg *ApplierRegistry, base ApplyTarget, resources []ImpactResource,
) []probeResult {
	probes := make([]probeResult, len(resources))

	work := make(chan int)
	var wg sync.WaitGroup

	workers := probeWorkers
	if len(resources) < workers {
		workers = len(resources)
	}
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				resource := resources[i]
				applier, ok := reg.Lookup(resource.ResourceType)
				if !ok {
					continue
				}
				prober, ok := applier.(ResourceProber)
				if !ok {
					continue
				}

				target := base
				target.ResourceType = resource.ResourceType
				target.ResourceID = resource.ResourceID
				target.ResourceName = resource.ResourceName
				if resource.EnvironmentID != nil {
					target.EnvironmentID = *resource.EnvironmentID
				}

				probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
				canonicalID, exists, err := prober.Resolve(probeCtx, target)
				cancel()
				if err != nil {
					slog.Warn("probing a secret reference failed; leaving it alone",
						"resource_type", resource.ResourceType,
						"resource", resource.ResourceName, "error", err)
					continue
				}
				probes[i] = probeResult{canonicalID: canonicalID, exists: exists, answered: true}
			}
		}()
	}

	for i := range resources {
		work <- i
	}
	close(work)
	wg.Wait()

	return probes
}

// PruneReferences deletes the reference rows for resources that no longer
// exist, or for the ones explicitly named.
//
// Explicit rather than automatic, and never called from a read: a GET that
// quietly deletes rows removes the evidence that something upstream is failing
// to clean up after itself.
func (s *Service) PruneReferences(
	ctx context.Context, reg *ApplierRegistry, secretID string, requested []ApplyTargetRef,
) (int, error) {
	impact, err := s.Impact(ctx, reg, secretID)
	if err != nil {
		return 0, err
	}

	wanted := map[string]bool{}
	for _, ref := range requested {
		wanted[ref.ResourceType+"\x1f"+ref.ResourceID] = true
	}

	pruned := 0
	// bounded by the resources one secret is referenced from
	for _, resource := range impact.Resources {
		key := resource.ResourceType + "\x1f" + resource.ResourceID
		if len(wanted) > 0 && !wanted[key] {
			continue
		}

		// A resource that is gone loses its own row too. A live one keeps it
		// and loses only the duplicates that resolved onto it.
		doomed := append([]string{}, resource.SupersededIDs...)
		if resource.Missing {
			doomed = append(doomed, resource.ResourceID)
		}

		// bounded by the superseded ids of one resource, plus the resource itself when it is gone
		for _, resourceID := range doomed {
			result, err := s.db.ExecContext(ctx, `
				DELETE FROM secret_references
				WHERE secret_id = ? AND resource_type = ? AND resource_id = ?
			`, secretID, resource.ResourceType, resourceID)
			if err != nil {
				return pruned, fmt.Errorf("pruning secret references: %w", err)
			}
			pruned += int(coreRowsAffected(result))
		}
	}
	return pruned, nil
}

// Apply re-materialises the named resources, or every stale one when none are
// named, so they run with the secret's current value.
//
// **Sequential, and it finishes.** One at a time, because recreating forty
// containers at once is a self-inflicted outage — but a failure on one says
// nothing about the next, so the run continues and every resource gets its own
// status. The caller reads the results slice rather than a single verdict; a
// rotation to a bad value will fail every resource in turn, and that is what
// the per-resource statuses are there to make obvious.
func (s *Service) Apply(
	ctx context.Context, reg *ApplierRegistry, secretID string, requested []ApplyTargetRef,
) ([]ApplyResult, error) {
	impact, err := s.Impact(ctx, reg, secretID)
	if err != nil {
		return nil, err
	}

	wanted := map[string]bool{}
	for _, ref := range requested {
		wanted[ref.ResourceType+"\x1f"+ref.ResourceID] = true
	}

	selected := []ImpactResource{}
	for _, resource := range impact.Resources {
		// A resource that is gone cannot be applied to, even by name: there is
		// nothing there to recreate. Prune is the action for those.
		if resource.Missing {
			continue
		}
		key := resource.ResourceType + "\x1f" + resource.ResourceID
		if len(wanted) > 0 {
			if wanted[key] {
				selected = append(selected, resource)
			}
			continue
		}
		if resource.Stale {
			selected = append(selected, resource)
		}
	}

	results := make([]ApplyResult, 0, len(selected))
	for _, resource := range selected {
		result := ApplyResult{
			ResourceType: resource.ResourceType,
			ResourceID:   resource.ResourceID,
			ResourceName: resource.ResourceName,
		}
		applier, ok := reg.Lookup(resource.ResourceType)
		if !ok {
			result.Status = ApplyStatusUnsupported
			result.Detail = ErrNoApplier.Error()
			results = append(results, result)
			continue
		}

		target := ApplyTarget{
			ResourceType: resource.ResourceType,
			ResourceID:   resource.ResourceID,
			ResourceName: resource.ResourceName,
			SecretID:     impact.SecretID,
			SecretName:   impact.SecretName,
		}
		if resource.EnvironmentID != nil {
			target.EnvironmentID = *resource.EnvironmentID
		}

		applyCtx, cancel := context.WithTimeout(ctx, applyTimeout)
		applyErr := applier.Apply(applyCtx, target)
		cancel()
		if applyErr == nil {
			result.Status = ApplyStatusApplied
			results = append(results, result)
			continue
		}

		result.Status = ApplyStatusFailed
		result.Detail = fmt.Sprintf("applying %s %s: %v",
			resource.ResourceType, resource.ResourceName, applyErr)
		results = append(results, result)
	}
	return results, nil
}

// AutoApplyEnabled reports whether a secret propagates its own rotations,
// read without loading the rest of the row so the rotate path can ask cheaply.
func (s *Service) AutoApplyEnabled(ctx context.Context, secretID string) (bool, error) {
	var flag int
	err := s.db.QueryRowContext(ctx,
		"SELECT auto_apply FROM secrets WHERE id = ?", secretID).Scan(&flag)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("reading auto-apply flag: %w", err)
	}
	return flag == 1, nil
}

// RebindReference moves a resource's references from one identifier to
// another, recomputing ref_key so the upsert on the next deploy still matches.
//
// A container is resolved before it exists, so its references are first
// written against the name the caller asked for and rebound onto the container
// id once Docker has created it. That is also what survives a rename: the id
// does not change, so the reference no longer orphans itself.
func (s *Service) RebindReference(
	ctx context.Context, resourceType, fromResourceID, toResourceID, resourceName string,
) error {
	if resourceType == "" || fromResourceID == "" || toResourceID == "" {
		return nil
	}
	if fromResourceID == toResourceID {
		// Nothing moves, but the display name may still be new.
		_, err := s.db.ExecContext(ctx,
			"UPDATE secret_references SET resource_name = ? WHERE resource_type = ? AND resource_id = ?",
			nullIfEmpty(resourceName), resourceType, toResourceID)
		if err != nil {
			return fmt.Errorf("naming secret references: %w", err)
		}
		return nil
	}

	type binding struct {
		id, secretID string
		field        sql.NullString
	}
	bindings, err := func() ([]binding, error) {
		rows, err := s.db.QueryContext(ctx, `
		SELECT id, secret_id, field FROM secret_references
		WHERE resource_type = ? AND resource_id = ? LIMIT 1000
	`, resourceType, fromResourceID)
		if err != nil {
			return nil, fmt.Errorf("loading secret references to rebind: %w", err)
		}
		defer closeRows(rows)
		out := []binding{}
		for rows.Next() {
			var b binding
			if err := rows.Scan(&b.id, &b.secretID, &b.field); err != nil {
				return nil, fmt.Errorf("scanning secret reference to rebind: %w", err)
			}
			out = append(out, b)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterating secret references to rebind: %w", err)
		}
		return out, nil
	}()
	if err != nil {
		return err
	}

	// bounded by the references bound to one resource
	for _, b := range bindings {
		var field *string
		if b.field.Valid {
			field = &b.field.String
		}
		key := refKey(b.secretID, resourceType, toResourceID, field)
		// A reference may already exist against the destination id — the same
		// container recreated twice. Delete the row being moved rather than
		// letting the unique index reject the whole rebind.
		_, err := s.db.ExecContext(ctx, `
			UPDATE secret_references
			SET resource_id = ?, resource_name = ?, ref_key = ?
			WHERE id = ?
		`, toResourceID, nullIfEmpty(resourceName), key, b.id)
		if err == nil {
			continue
		}
		if !isUniqueViolation(err) {
			return fmt.Errorf("rebinding secret reference: %w", err)
		}
		if _, delErr := s.db.ExecContext(ctx,
			"DELETE FROM secret_references WHERE id = ?", b.id); delErr != nil {
			return fmt.Errorf("dropping superseded secret reference: %w", delErr)
		}
	}
	return nil
}
