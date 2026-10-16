// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ConfigSection names one selectable part of a container's configuration.
//
// The key is a dotted path into the inspect document that the backup
// writes to `container/inspect.json`, which is the same document the
// restore-side config tree walks. Using the inspect shape rather than a
// bespoke vocabulary means a section selected at backup time is
// addressable at restore time with the identical key.
type ConfigSection struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// configIdentityPaths are always written when config is captured at all,
// including in choose mode.
//
// They are what makes the document identifiable as a backup *of this
// container*: without a name and an image reference a pruned inspect is an
// anonymous bag of settings that nothing downstream can sensibly apply.
// They are cheap — four scalar fields — so there is no reason to make the
// user opt into them.
var configIdentityPaths = []string{"Id", "Name", "Created", "Image"}

// configSectionCatalog is the fixed set of config sections a user can pick
// from. It is deliberately a catalog rather than a reflection over the
// inspect document: the point is to offer parts a person recognises and
// can reason about restoring, not to enumerate every field Docker emits.
func configSectionCatalog() []ConfigSection {
	return []ConfigSection{
		{Key: "Config.Env", Label: "Environment variables", Description: "Every variable set on the container."},
		{Key: "Config.Cmd", Label: "Command", Description: "The command the container runs."},
		{Key: "Config.Entrypoint", Label: "Entrypoint", Description: "The entrypoint the command runs under."},
		{Key: "Config.Labels", Label: "Labels", Description: "Container labels, including Compose metadata."},
		{Key: "Config.WorkingDir", Label: "Working directory", Description: "The container's working directory."},
		{Key: "Config.User", Label: "User", Description: "The user the container runs as."},
		{Key: "Config.ExposedPorts", Label: "Exposed ports", Description: "Ports the image declares."},
		{Key: "Config.Healthcheck", Label: "Healthcheck", Description: "The container's health probe."},
		{Key: "HostConfig.PortBindings", Label: "Port bindings", Description: "Host-to-container port mappings."},
		{Key: "HostConfig.Binds", Label: "Bind mounts", Description: "Host paths mounted into the container."},
		{Key: "HostConfig.Mounts", Label: "Mounts", Description: "Volume and bind mount definitions."},
		{Key: "HostConfig.RestartPolicy", Label: "Restart policy", Description: "When Docker restarts the container."},
		{Key: "HostConfig.NetworkMode", Label: "Network mode", Description: "The container's network mode."},
		{Key: "HostConfig.Resources", Label: "Resource limits", Description: "CPU, memory, and IO limits."},
		{Key: "HostConfig.CapAdd", Label: "Added capabilities", Description: "Linux capabilities granted."},
		{Key: "HostConfig.CapDrop", Label: "Dropped capabilities", Description: "Linux capabilities removed."},
		{Key: "HostConfig.Devices", Label: "Devices", Description: "Host devices passed through."},
		{Key: "NetworkSettings.Networks", Label: "Networks", Description: "Networks the container is attached to."},
	}
}

// configSectionKeys is the catalog reduced to a lookup set, used to reject
// a path the catalog never offered.
func configSectionKeys() map[string]struct{} {
	out := map[string]struct{}{}
	for _, section := range configSectionCatalog() {
		out[section.Key] = struct{}{}
	}
	return out
}

// pruneConfigDocument returns the inspect document reduced to the selected
// dotted section paths plus the identity fields.
//
// A path the catalog does not offer is dropped rather than applied, so a
// hand-crafted request cannot reach into arbitrary parts of the document.
// A selection that matches nothing still yields the identity fields, which
// keeps `container/inspect.json` a valid document rather than an empty one.
func pruneConfigDocument(raw []byte, paths []string) ([]byte, error) {
	var source map[string]any
	if err := json.Unmarshal(raw, &source); err != nil {
		return nil, fmt.Errorf("decoding container inspect for pruning: %w", err)
	}

	allowed := configSectionKeys()
	out := map[string]any{}
	for _, path := range configIdentityPaths {
		copyJSONPath(source, out, path)
	}
	for _, path := range paths {
		clean := strings.TrimSpace(path)
		if clean == "" {
			continue
		}
		// A path is either a whole section ("Config.Env") or one item
		// inside it ("Config.Env.PATH"). The split matches what
		// splitConfigPath already does on the restore side, so a
		// selection means the same thing captured or applied.
		if _, ok := allowed[clean]; ok {
			copyJSONPath(source, out, clean)
			continue
		}
		section, item, ok := splitConfigSelectionPath(clean, allowed)
		if !ok {
			continue
		}
		copyJSONItem(source, out, section, item)
	}

	pruned, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding pruned container inspect: %w", err)
	}
	return pruned, nil
}

// copyJSONPath copies one dotted path from src into dst, creating the
// intermediate objects it needs. A path that is absent from src is skipped
// silently: a container with no healthcheck should not fail a backup that
// happened to list one.
func copyJSONPath(src map[string]any, dst map[string]any, path string) {
	segments := strings.Split(path, ".")
	value := any(src)
	for _, segment := range segments {
		node, ok := value.(map[string]any)
		if !ok {
			return
		}
		next, exists := node[segment]
		if !exists {
			return
		}
		value = next
	}

	cursor := dst
	for index, segment := range segments {
		if index == len(segments)-1 {
			cursor[segment] = value
			return
		}
		child, ok := cursor[segment].(map[string]any)
		if !ok {
			child = map[string]any{}
			cursor[segment] = child
		}
		cursor = child
	}
}

// splitConfigSelectionPath separates "Config.Env.PATH" into the section
// "Config.Env" and the item "PATH".
//
// The item name may itself contain dots — "80/tcp" does not, but a label
// like "com.docker.compose.project" very much does — so the split is
// anchored on the longest catalog section that prefixes the path, not on
// the last dot.
func splitConfigSelectionPath(
	path string,
	allowed map[string]struct{},
) (section string, item string, ok bool) {
	for candidate := range allowed {
		if !strings.HasPrefix(path, candidate+".") {
			continue
		}
		rest := strings.TrimPrefix(path, candidate+".")
		if rest == "" {
			continue
		}
		// Longest match wins, so a section that is itself a prefix of
		// another cannot swallow the wrong path.
		if len(candidate) > len(section) {
			section, item, ok = candidate, rest, true
		}
	}
	return section, item, ok
}

// copyJSONItem copies a single named entry out of one config section.
//
// Docker uses two shapes for the fields worth picking apart, and both are
// handled: an array of "KEY=VALUE" strings (Config.Env), and an object
// keyed by name (Config.Labels, HostConfig.PortBindings,
// NetworkSettings.Networks). Anything else — a scalar, or an array of
// plain values — has no addressable items, so the selection is dropped
// rather than reinterpreted.
func copyJSONItem(src map[string]any, dst map[string]any, section, item string) {
	value := resolveJSONPath(src, section)
	if value == nil {
		return
	}

	switch typed := value.(type) {
	case map[string]any:
		entry, exists := typed[item]
		if !exists {
			return
		}
		target := ensureJSONPath(dst, section, map[string]any{})
		container, ok := target.(map[string]any)
		if !ok {
			return
		}
		container[item] = entry

	case []any:
		kept := []any{}
		for _, element := range typed {
			text, isString := element.(string)
			if !isString {
				continue
			}
			// "KEY=VALUE" matches on KEY; a bare string matches whole,
			// which is what makes Cmd and Entrypoint entries selectable.
			name := text
			if idx := strings.Index(text, "="); idx > 0 {
				name = text[:idx]
			}
			if name == item {
				kept = append(kept, element)
			}
		}
		if len(kept) == 0 {
			return
		}
		existing := resolveJSONPath(dst, section)
		if current, ok := existing.([]any); ok {
			kept = append(current, kept...)
		}
		ensureJSONPath(dst, section, kept)
	}
}

// resolveJSONPath returns the value at a dotted path, or nil.
func resolveJSONPath(root map[string]any, path string) any {
	value := any(root)
	for _, segment := range strings.Split(path, ".") {
		node, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		next, exists := node[segment]
		if !exists {
			return nil
		}
		value = next
	}
	return value
}

// ensureJSONPath writes value at a dotted path unless something is already
// there, and returns whatever now occupies the slot. Creating the
// intermediate objects here is what lets several items of the same section
// accumulate into one container across calls.
func ensureJSONPath(root map[string]any, path string, value any) any {
	segments := strings.Split(path, ".")
	cursor := root
	for index, segment := range segments {
		if index == len(segments)-1 {
			if existing, ok := cursor[segment]; ok {
				if _, isMap := existing.(map[string]any); isMap {
					return existing
				}
			}
			cursor[segment] = value
			return value
		}
		child, ok := cursor[segment].(map[string]any)
		if !ok {
			child = map[string]any{}
			cursor[segment] = child
		}
		cursor = child
	}
	return value
}
