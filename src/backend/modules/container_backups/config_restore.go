// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
)

// ContainerRecreator recreates a container from a full spec. The containers
// module owns the implementation (stop / rename / create / rollback,
// including the agent-environment and self-container special cases); this
// module only needs the one call, so it declares the contract and main.go
// injects the bridge from internal/bootstrap.
type ContainerRecreator interface {
	// Recreate returns the replacement's ID: a restore that also writes the
	// filesystem layer has to write it into the new container, not the old.
	Recreate(ctx context.Context, envID, containerID string, req coredocker.RecreateRequest) (string, error)
}

// Restoring container configuration means recreating the container: Docker
// cannot change environment, labels, ports or the entrypoint on a live one. That
// is the same operation the container edit screen performs, and it runs through
// the same container recreate path — which renames the original aside and
// puts it back if creating the replacement fails.
//
// replacementContainerSpec builds the new container from the *live* config and
// overlays only the non-nil fields of RecreateRequest. So a request carrying just
// Env restores the environment and leaves everything else exactly as it is.

// configRestorableFields maps a Config field name onto whether this pipeline can
// put it back. Args is derived from Cmd and Entrypoint at runtime and Volumes
// declares anonymous volumes at create time; neither has a RecreateRequest field,
// so both are reported as skipped rather than quietly dropped.
var configRestorableFields = map[string]bool{
	"Env":          true,
	"Labels":       true,
	"Cmd":          true,
	"Entrypoint":   true,
	"ExposedPorts": true,
	"WorkingDir":   true,
	"User":         true,
	"Image":        true,
	"Args":         false,
	"Volumes":      false,
}

// configRestoreSelection is the parsed form of the `config:` keys in a restore
// selection. whole names fields to restore entirely; names holds the per-entry
// picks under Env and Labels, which merge into the live values instead of
// replacing the whole field.
type configRestoreSelection struct {
	all    bool
	whole  map[string]struct{}
	names  map[string]map[string]struct{}
	rawKey map[string]string
}

// newConfigRestoreSelection parses the inner paths recorded against the config
// unit. Inner paths arrive without the "config:" prefix, e.g. "Config.Env.PATH".
func newConfigRestoreSelection(plan *restorePlan) *configRestoreSelection {
	sel := &configRestoreSelection{
		whole:  map[string]struct{}{},
		names:  map[string]map[string]struct{}{},
		rawKey: map[string]string{},
	}
	if plan == nil {
		return sel
	}
	if plan.all || plan.isWhole(runItemConfig) {
		sel.all = true
		return sel
	}
	for _, inner := range plan.subsets[runItemConfig] {
		field, name := splitConfigPath(inner)
		if field == "" {
			continue
		}
		sel.rawKey[field] = runItemConfig + ":Config." + field
		if name == "" {
			sel.whole[field] = struct{}{}
			continue
		}
		if sel.names[field] == nil {
			sel.names[field] = map[string]struct{}{}
		}
		sel.names[field][name] = struct{}{}
	}
	return sel
}

// splitConfigPath turns "Config.Env.PATH" into ("Env", "PATH") and "Config.Cmd"
// into ("Cmd", ""). A bare "Config" selects everything and yields ("", "").
func splitConfigPath(inner string) (field string, name string) {
	inner = strings.TrimSpace(inner)
	inner = strings.TrimPrefix(inner, "Config.")
	if inner == "" || inner == "Config" {
		return "", ""
	}
	if idx := strings.Index(inner, "."); idx > 0 {
		return inner[:idx], inner[idx+1:]
	}
	return inner, ""
}

// fields returns the Config field names this selection touches.
func (s *configRestoreSelection) fields() []string {
	if s.all {
		out := make([]string, 0, len(configRestorableFields))
		for field := range configRestorableFields {
			out = append(out, field)
		}
		sort.Strings(out)
		return out
	}
	seen := map[string]struct{}{}
	for field := range s.whole {
		seen[field] = struct{}{}
	}
	for field := range s.names {
		seen[field] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for field := range seen {
		out = append(out, field)
	}
	sort.Strings(out)
	return out
}

// savedConfig is the slice of container/inspect.json this restore reads. The
// archive stores a full ContainerJSON; only Config is needed to rebuild settings.
type savedConfig struct {
	Config *container.Config `json:"Config"`
}

// buildConfigRecreateRequest overlays the archived settings that were selected
// onto a request, leaving every unselected field nil so Recreate preserves the
// container's current value for it.
//
// applied lists the selection keys that produced a change; skipped lists fields
// that were asked for but cannot be put back.
func buildConfigRecreateRequest(
	inspectJSON []byte,
	live *container.Config,
	sel *configRestoreSelection,
) (req coredocker.RecreateRequest, applied []string, skipped []string, err error) {
	var saved savedConfig
	if err := json.Unmarshal(inspectJSON, &saved); err != nil {
		return req, nil, nil, fmt.Errorf("decoding archived container config: %w", err)
	}
	if saved.Config == nil {
		return req, nil, nil, ErrBackupRestoreConfigMissing
	}
	cfg := saved.Config

	// The live config is the baseline a per-entry pick merges into. A container
	// that vanished between inspect and here leaves it nil; treat that as an
	// empty baseline rather than panicking mid-restore.
	var liveEnv []string
	var liveLabels map[string]string
	if live != nil {
		liveEnv = live.Env
		liveLabels = live.Labels
	}

	for _, field := range sel.fields() {
		restorable, known := configRestorableFields[field]
		if !known || !restorable {
			skipped = append(skipped, "Config."+field)
			continue
		}
		changed := false
		switch field {
		case "Env":
			merged, ok := mergeConfigList(envPairs(liveEnv), envPairs(cfg.Env), sel.pick(field))
			if ok {
				req.Env = flattenPairs(merged)
				changed = true
			}
		case "Labels":
			merged, ok := mergeConfigMap(liveLabels, cfg.Labels, sel.pick(field))
			if ok {
				req.Labels = merged
				changed = true
			}
		case "Cmd":
			req.Cmd = []string(cfg.Cmd)
			changed = req.Cmd != nil
		case "Entrypoint":
			req.Entrypoint = []string(cfg.Entrypoint)
			changed = req.Entrypoint != nil
		case "ExposedPorts":
			if len(cfg.ExposedPorts) > 0 {
				ports := map[string]struct{}{}
				for port := range cfg.ExposedPorts {
					ports[port.String()] = struct{}{}
				}
				req.ExposedPorts = ports
				changed = true
			}
		case "WorkingDir":
			value := cfg.WorkingDir
			req.WorkingDir = &value
			changed = true
		case "User":
			value := cfg.User
			req.User = &value
			changed = true
		case "Image":
			if strings.TrimSpace(cfg.Image) != "" {
				req.Image = cfg.Image
				changed = true
			}
		}
		if changed {
			applied = append(applied, sel.key(field))
		}
	}
	return req, applied, skipped, nil
}

// pick returns the individual entry names selected under a field, or nil when
// the whole field was selected.
func (s *configRestoreSelection) pick(field string) map[string]struct{} {
	if s.all {
		return nil
	}
	if _, whole := s.whole[field]; whole {
		return nil
	}
	return s.names[field]
}

// key returns the selection key to record for a restored field.
func (s *configRestoreSelection) key(field string) string {
	if raw, ok := s.rawKey[field]; ok {
		return raw
	}
	return runItemConfig + ":Config." + field
}

// mergeConfigMap overlays the archived map onto the live one. With names set,
// only those keys are taken from the archive and everything else the container
// currently has is preserved; with names nil the archived map replaces it.
func mergeConfigMap(live, saved map[string]string, names map[string]struct{}) (map[string]string, bool) {
	if names == nil {
		if saved == nil {
			return nil, false
		}
		out := map[string]string{}
		for k, v := range saved {
			out[k] = v
		}
		return out, true
	}
	out := map[string]string{}
	for k, v := range live {
		out[k] = v
	}
	changed := false
	for name := range names {
		if value, ok := saved[name]; ok {
			out[name] = value
			changed = true
			continue
		}
		// The archive did not carry this entry, so restoring it means
		// removing it from the container.
		if _, present := out[name]; present {
			delete(out, name)
			changed = true
		}
	}
	return out, changed
}

// mergeConfigList is mergeConfigMap for NAME=VALUE lists such as Env, keeping the
// archived ordering when the whole field is taken.
func mergeConfigList(live, saved map[string]string, names map[string]struct{}) (map[string]string, bool) {
	return mergeConfigMap(live, saved, names)
}

// envPairs splits a NAME=VALUE list into a map. Entries with no "=" are kept with
// an empty value so they survive a round trip.
func envPairs(env []string) map[string]string {
	out := map[string]string{}
	for _, entry := range env {
		name, value, found := strings.Cut(entry, "=")
		if !found {
			out[entry] = ""
			continue
		}
		out[name] = value
	}
	return out
}

// flattenPairs rebuilds a NAME=VALUE list, sorted so a recreate does not churn
// the container spec purely on map iteration order.
func flattenPairs(pairs map[string]string) []string {
	names := make([]string, 0, len(pairs))
	for name := range pairs {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, name+"="+pairs[name])
	}
	return out
}

// restoreContainerConfig recreates the container with the archived settings the
// operator selected. It runs after the archive walk so that mounts restored in
// the same operation are already in place — they live outside the container
// layer and survive the recreate.
func (s *Service) restoreContainerConfig(
	ctx context.Context,
	run *BackupRun,
	progressRunID string,
	inspectJSON []byte,
	plan *restorePlan,
) ([]string, string, error) {
	if len(inspectJSON) == 0 {
		return nil, "", ErrBackupRestoreConfigMissing
	}
	if progressRunID != "" {
		s.updateRunProgress(ctx, progressRunID, "restore_config", "Applying container configuration.")
	}
	cli, err := s.client(ctx, run.EnvironmentID)
	if err != nil {
		return nil, "", err
	}
	inspectCtx, cancelInspect := context.WithTimeout(ctx, coredocker.CallTimeout)
	infoRes, err := cli.ContainerInspect(inspectCtx, run.ContainerID, client.ContainerInspectOptions{})
	info := infoRes.Container
	cancelInspect()
	if err != nil {
		return nil, "", fmt.Errorf("inspecting container before config restore: %w", err)
	}

	sel := newConfigRestoreSelection(plan)
	req, applied, skipped, err := buildConfigRecreateRequest(inspectJSON, info.Config, sel)
	if err != nil {
		return nil, "", err
	}
	if len(skipped) > 0 && s.logger != nil {
		s.logger.Warn("container config restore skipped unsupported fields",
			"run", run.ID, "container", run.ContainerID, "fields", strings.Join(skipped, ","))
	}
	if len(applied) == 0 {
		return nil, "", ErrBackupRestoreConfigUnsupported
	}

	// Recreate owns the stop / rename / create / rollback dance, including the
	// agent-environment and self-container special cases.
	if s.containerRecreator == nil {
		return nil, "", ErrBackupRestoreConfigUnsupported
	}
	newID, err := s.containerRecreator.Recreate(ctx, run.EnvironmentID, run.ContainerID, req)
	if err != nil {
		return nil, "", fmt.Errorf("recreating container for config restore: %w", err)
	}
	return applied, newID, nil
}
