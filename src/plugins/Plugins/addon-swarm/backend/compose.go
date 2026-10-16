// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// This file converts a Compose file into SwarmKit specs.
//
// SwarmKit has no server-side stack deploy: `docker stack deploy` is entirely
// client-side, and the daemon only ever sees the resulting ServiceCreate /
// ServiceUpdate / NetworkCreate calls. McHarbor therefore has to do the same
// conversion the CLI does. The subset modelled here is the subset that has a
// meaning under Swarm; anything Compose-only is rejected by name rather than
// dropped silently, so an operator learns why their file was refused instead
// of wondering why half of it did not take effect.

// composeFile is the parsed top level of a Compose document.
type composeFile struct {
	Version  string                    `yaml:"version"`
	Services map[string]composeService `yaml:"services"`
	Networks map[string]composeNetwork `yaml:"networks"`
	Volumes  map[string]yaml.Node      `yaml:"volumes"`
	Secrets  map[string]composeExtern  `yaml:"secrets"`
	Configs  map[string]composeExtern  `yaml:"configs"`
}

type composeService struct {
	Image       string      `yaml:"image"`
	Build       yaml.Node   `yaml:"build"`
	Command     yaml.Node   `yaml:"command"`
	Entrypoint  yaml.Node   `yaml:"entrypoint"`
	Environment yaml.Node   `yaml:"environment"`
	Labels      yaml.Node   `yaml:"labels"`
	Ports       []yaml.Node `yaml:"ports"`
	Volumes     []yaml.Node `yaml:"volumes"`
	Networks    yaml.Node   `yaml:"networks"`
	Secrets     []yaml.Node `yaml:"secrets"`
	Configs     []yaml.Node `yaml:"configs"`
	Hostname    string      `yaml:"hostname"`
	User        string      `yaml:"user"`
	WorkingDir  string      `yaml:"working_dir"`
	StopSignal  string      `yaml:"stop_signal"`
	TTY         bool        `yaml:"tty"`
	StdinOpen   bool        `yaml:"stdin_open"`
	ReadOnly    bool        `yaml:"read_only"`
	Restart     string      `yaml:"restart"`
	DependsOn   yaml.Node   `yaml:"depends_on"`
	Deploy      composeDeploy `yaml:"deploy"`
}

type composeDeploy struct {
	Mode          string             `yaml:"mode"`
	Replicas      *uint64            `yaml:"replicas"`
	Labels        yaml.Node          `yaml:"labels"`
	EndpointMode  string             `yaml:"endpoint_mode"`
	Placement     composePlacement   `yaml:"placement"`
	Resources     composeResources   `yaml:"resources"`
	RestartPolicy composeRestart     `yaml:"restart_policy"`
	UpdateConfig  composeUpdate      `yaml:"update_config"`
}

type composePlacement struct {
	Constraints []string `yaml:"constraints"`
	MaxReplicas uint64   `yaml:"max_replicas_per_node"`
}

type composeResources struct {
	Limits       composeResourceEntry `yaml:"limits"`
	Reservations composeResourceEntry `yaml:"reservations"`
}

type composeResourceEntry struct {
	CPUs   string `yaml:"cpus"`
	Memory string `yaml:"memory"`
}

type composeRestart struct {
	Condition   string  `yaml:"condition"`
	Delay       string  `yaml:"delay"`
	MaxAttempts *uint64 `yaml:"max_attempts"`
	Window      string  `yaml:"window"`
}

type composeUpdate struct {
	Parallelism   *uint64 `yaml:"parallelism"`
	Delay         string  `yaml:"delay"`
	FailureAction string  `yaml:"failure_action"`
	Order         string  `yaml:"order"`
}

type composeNetwork struct {
	Driver     string            `yaml:"driver"`
	External   yaml.Node         `yaml:"external"`
	Attachable bool              `yaml:"attachable"`
	Internal   bool              `yaml:"internal"`
	Labels     yaml.Node         `yaml:"labels"`
	Name       string            `yaml:"name"`
	DriverOpts map[string]string `yaml:"driver_opts"`
}

type composeExtern struct {
	External yaml.Node `yaml:"external"`
	Name     string    `yaml:"name"`
	File     string    `yaml:"file"`
}

// parseCompose reads a Compose document and reports every Swarm-incompatible
// key it contains, keyed by the service that used it.
func parseCompose(src string) (composeFile, []StackServiceError, error) {
	var file composeFile
	if err := yaml.Unmarshal([]byte(src), &file); err != nil {
		return composeFile{}, nil, fmt.Errorf("compose parse: %w", err)
	}
	if len(file.Services) == 0 {
		return composeFile{}, nil, errNoServices
	}

	var rejected []StackServiceError
	for name, svc := range file.Services {
		// `build` has no meaning to a cluster: there is no build context on a
		// remote node and the image would never exist there.
		if !svc.Build.IsZero() {
			rejected = append(rejected, StackServiceError{Service: name, Reason: "build"})
		}
		// Compose's `restart` is a per-container policy; Swarm's equivalent
		// lives under deploy.restart_policy. Accepting the former silently
		// would mean a service that never restarts when the operator asked
		// for always.
		if svc.Restart != "" {
			rejected = append(rejected, StackServiceError{Service: name, Reason: "restart"})
		}
		// Long-form depends_on carries start-order conditions the orchestrator
		// cannot honour — SwarmKit schedules every task independently.
		if svc.DependsOn.Kind == yaml.MappingNode {
			rejected = append(rejected, StackServiceError{Service: name, Reason: "depends_on.condition"})
		}
		if svc.Image == "" {
			rejected = append(rejected, StackServiceError{Service: name, Reason: "image"})
		}
	}

	return file, rejected, nil
}

// ---------------------------------------------------------------------------
// YAML shape helpers. Compose accepts most collections in two shapes and real
// files in the wild use both.
// ---------------------------------------------------------------------------

// stringList reads a value that may be a bare string or a sequence.
func stringList(n yaml.Node) []string {
	switch n.Kind {
	case yaml.ScalarNode:
		var s string
		if err := n.Decode(&s); err != nil || strings.TrimSpace(s) == "" {
			return nil
		}
		return strings.Fields(s)
	case yaml.SequenceNode:
		var out []string
		if err := n.Decode(&out); err != nil {
			return nil
		}
		return out
	default:
		return nil
	}
}

// keyValues reads a value that may be a `KEY=value` sequence or a mapping.
func keyValues(n yaml.Node) map[string]string {
	out := map[string]string{}
	switch n.Kind {
	case yaml.MappingNode:
		raw := map[string]yaml.Node{}
		if err := n.Decode(&raw); err != nil {
			return out
		}
		for k, v := range raw {
			out[k] = scalarString(v)
		}
	case yaml.SequenceNode:
		var items []string
		if err := n.Decode(&items); err != nil {
			return out
		}
		for _, item := range items {
			k, v, found := strings.Cut(item, "=")
			if !found {
				out[item] = ""
				continue
			}
			out[k] = v
		}
	}
	return out
}

// scalarString renders any scalar as a string, so `replicas: 3` and
// `LOG_LEVEL: true` do not become empty values.
func scalarString(n yaml.Node) string {
	if n.Kind != yaml.ScalarNode {
		return ""
	}
	return n.Value
}

// envList renders an environment block as the `KEY=value` list SwarmKit takes.
func envList(n yaml.Node) []string {
	pairs := keyValues(n)
	out := make([]string, 0, len(pairs))
	for k, v := range pairs {
		out = append(out, k+"="+v)
	}
	return out
}

// networkNames reads a service's `networks:` in either shape.
func networkNames(n yaml.Node) []string {
	switch n.Kind {
	case yaml.SequenceNode:
		var out []string
		if err := n.Decode(&out); err != nil {
			return nil
		}
		return out
	case yaml.MappingNode:
		raw := map[string]yaml.Node{}
		if err := n.Decode(&raw); err != nil {
			return nil
		}
		out := make([]string, 0, len(raw))
		for k := range raw {
			out = append(out, k)
		}
		return out
	default:
		return nil
	}
}

// isExternal reads Compose's `external:`, which is a bool in the short form
// and a mapping carrying a name in the long one.
func isExternal(n yaml.Node) bool {
	switch n.Kind {
	case yaml.ScalarNode:
		var b bool
		if err := n.Decode(&b); err != nil {
			return false
		}
		return b
	case yaml.MappingNode:
		return true
	default:
		return false
	}
}

// externalName reads the name out of the long `external: {name: ...}` form.
func externalName(n yaml.Node) string {
	if n.Kind != yaml.MappingNode {
		return ""
	}
	raw := struct {
		Name string `yaml:"name"`
	}{}
	if err := n.Decode(&raw); err != nil {
		return ""
	}
	return raw.Name
}

// ---------------------------------------------------------------------------
// Value parsers
// ---------------------------------------------------------------------------

// parsePort reads a Compose port entry in either the string short form
// ("8080:80/tcp", "80", "127.0.0.1:8080:80") or the long mapping form.
func parsePort(n yaml.Node) (ServicePort, error) {
	if n.Kind == yaml.MappingNode {
		long := struct {
			Target    uint32 `yaml:"target"`
			Published any    `yaml:"published"`
			Protocol  string `yaml:"protocol"`
			Mode      string `yaml:"mode"`
		}{}
		if err := n.Decode(&long); err != nil {
			return ServicePort{}, fmt.Errorf("port entry: %w", err)
		}
		published := uint32(0)
		switch v := long.Published.(type) {
		case int:
			published = uint32(v)
		case string:
			if parsed, err := strconv.ParseUint(v, 10, 32); err == nil {
				published = uint32(parsed)
			}
		}
		return ServicePort{
			TargetPort:    long.Target,
			PublishedPort: published,
			Protocol:      defaultString(long.Protocol, "tcp"),
			PublishMode:   defaultString(long.Mode, "ingress"),
		}, nil
	}

	raw := strings.TrimSpace(n.Value)
	if raw == "" {
		return ServicePort{}, fmt.Errorf("empty port entry")
	}

	proto := "tcp"
	if spec, p, found := strings.Cut(raw, "/"); found {
		raw = spec
		proto = p
	}

	parts := strings.Split(raw, ":")
	// A host-IP binding ("127.0.0.1:8080:80") has no Swarm equivalent — the
	// routing mesh publishes on every node — so the IP is dropped and the
	// port pair is kept.
	if len(parts) == 3 {
		parts = parts[1:]
	}

	switch len(parts) {
	case 1:
		target, err := strconv.ParseUint(parts[0], 10, 32)
		if err != nil {
			return ServicePort{}, fmt.Errorf("port %q: %w", raw, err)
		}
		return ServicePort{TargetPort: uint32(target), Protocol: proto, PublishMode: "ingress"}, nil
	case 2:
		published, err := strconv.ParseUint(parts[0], 10, 32)
		if err != nil {
			return ServicePort{}, fmt.Errorf("published port %q: %w", parts[0], err)
		}
		target, err := strconv.ParseUint(parts[1], 10, 32)
		if err != nil {
			return ServicePort{}, fmt.Errorf("target port %q: %w", parts[1], err)
		}
		return ServicePort{
			TargetPort:    uint32(target),
			PublishedPort: uint32(published),
			Protocol:      proto,
			PublishMode:   "ingress",
		}, nil
	default:
		return ServicePort{}, fmt.Errorf("unrecognised port %q", raw)
	}
}

// parseVolume reads a Compose volume entry in short ("src:/dst:ro") or long
// (mapping) form. The stack name is needed to namespace a named volume the
// same way the CLI does.
func parseVolume(n yaml.Node, stack string, named map[string]struct{}) (ServiceMount, error) {
	if n.Kind == yaml.MappingNode {
		long := struct {
			Type     string `yaml:"type"`
			Source   string `yaml:"source"`
			Target   string `yaml:"target"`
			ReadOnly bool   `yaml:"read_only"`
		}{}
		if err := n.Decode(&long); err != nil {
			return ServiceMount{}, fmt.Errorf("volume entry: %w", err)
		}
		mountType := defaultString(long.Type, "volume")
		source := long.Source
		if mountType == "volume" && source != "" {
			if _, declared := named[source]; declared {
				source = stack + "_" + source
			}
		}
		return ServiceMount{
			Type:     mountType,
			Source:   source,
			Target:   long.Target,
			ReadOnly: long.ReadOnly,
		}, nil
	}

	raw := strings.TrimSpace(n.Value)
	if raw == "" {
		return ServiceMount{}, fmt.Errorf("empty volume entry")
	}

	parts := strings.Split(raw, ":")
	switch len(parts) {
	case 1:
		// An anonymous volume: a target with no source.
		return ServiceMount{Type: "volume", Target: parts[0]}, nil
	case 2, 3:
		source, target := parts[0], parts[1]
		readOnly := len(parts) == 3 && strings.Contains(parts[2], "ro")

		mountType := "volume"
		if strings.HasPrefix(source, "/") || strings.HasPrefix(source, ".") ||
			strings.HasPrefix(source, "~") {
			mountType = "bind"
		} else if _, declared := named[source]; declared {
			source = stack + "_" + source
		}
		return ServiceMount{Type: mountType, Source: source, Target: target, ReadOnly: readOnly}, nil
	default:
		return ServiceMount{}, fmt.Errorf("unrecognised volume %q", raw)
	}
}

// parseFileRef reads a secret or config reference in short (name) or long
// ({source, target}) form.
func parseFileRef(n yaml.Node) (source, target string) {
	if n.Kind == yaml.MappingNode {
		long := struct {
			Source string `yaml:"source"`
			Target string `yaml:"target"`
		}{}
		if err := n.Decode(&long); err != nil {
			return "", ""
		}
		return long.Source, long.Target
	}
	name := strings.TrimSpace(n.Value)
	return name, name
}

// parseMemory reads Compose's memory strings ("512M", "2g", "1073741824").
func parseMemory(raw string) (int64, error) {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" {
		return 0, nil
	}
	s = strings.TrimSuffix(s, "b")

	multiplier := int64(1)
	switch {
	case strings.HasSuffix(s, "k"):
		multiplier, s = 1024, strings.TrimSuffix(s, "k")
	case strings.HasSuffix(s, "m"):
		multiplier, s = 1024*1024, strings.TrimSuffix(s, "m")
	case strings.HasSuffix(s, "g"):
		multiplier, s = 1024*1024*1024, strings.TrimSuffix(s, "g")
	}

	value, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, fmt.Errorf("memory %q: %w", raw, err)
	}
	return int64(value * float64(multiplier)), nil
}

// parseCPUs reads Compose's fractional CPU strings into SwarmKit's nanoCPUs.
func parseCPUs(raw string) (int64, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, nil
	}
	value, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("cpus %q: %w", raw, err)
	}
	return int64(value * 1e9), nil
}

// parseDuration reads Compose's duration strings, tolerating a bare number of
// seconds, which older files use.
func parseDuration(raw string) (time.Duration, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, nil
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	secs, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("duration %q: %w", raw, err)
	}
	return time.Duration(secs * float64(time.Second)), nil
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
