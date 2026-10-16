// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Kinds of desired state. A container is one named container; a stack is a
// compose project; an environment declares that everything on it is accounted for.
const (
	KindContainer   = "container"
	KindStack       = "stack"
	KindEnvironment = "environment"
)

// ErrInvalidConfig is returned for a desired config that cannot be compared.
var ErrInvalidConfig = errors.New("reconciler: invalid desired config")

// DesiredConfig is the part of a container's configuration drift is checked
// against. Env and labels are a subset: keys not listed are not the loop's business.
type DesiredConfig struct {
	Env              map[string]string `json:"env,omitempty"`
	Labels           map[string]string `json:"labels,omitempty"`
	Ports            []string          `json:"ports,omitempty"`
	Mounts           []string          `json:"mounts,omitempty"`
	MemoryBytes      int64             `json:"memoryBytes,omitempty"`
	NanoCPUs         int64             `json:"nanoCpus,omitempty"`
	CheckImageDigest bool              `json:"checkImageDigest,omitempty"`
	Networks         []string          `json:"networks,omitempty"`
	CapAdd           []string          `json:"capAdd,omitempty"`
	CapDrop          []string          `json:"capDrop,omitempty"`
	Healthcheck      *HealthcheckSpec  `json:"healthcheck,omitempty"`
}

// HealthcheckSpec is the healthcheck a container should carry. Test is compared
// exactly; the durations only when set, as Go duration strings such as "30s".
type HealthcheckSpec struct {
	Test     []string `json:"test"`
	Interval string   `json:"interval,omitempty"`
	Timeout  string   `json:"timeout,omitempty"`
	Retries  int      `json:"retries,omitempty"`
}

// Modes a desired state answers drift with. Observe records it; propose also
// writes a reviewable plan; enforce applies the plan within the global limits.
const (
	ModeObserve = "observe"
	ModePropose = "propose"
	ModeEnforce = "enforce"
)

// ParseConfig decodes a stored or submitted config. Strict refuses unknown
// keys, which is what create and update use; rows written before the config was
// typed are read leniently so an old free-form blob cannot break the loop.
func ParseConfig(raw string, strict bool) (DesiredConfig, error) {
	var cfg DesiredConfig
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" || raw == "null" {
		return cfg, nil
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	if strict {
		dec.DisallowUnknownFields()
	}
	if err := dec.Decode(&cfg); err != nil {
		return DesiredConfig{}, fmt.Errorf("%w: %w", ErrInvalidConfig, err)
	}
	for _, p := range cfg.Ports {
		if _, err := parsePort(p); err != nil {
			return DesiredConfig{}, err
		}
	}
	for _, m := range cfg.Mounts {
		if _, _, ok := splitMount(m); !ok {
			return DesiredConfig{}, fmt.Errorf("%w: mount %q is not source:target", ErrInvalidConfig, m)
		}
	}
	if h := cfg.Healthcheck; h != nil {
		if len(h.Test) == 0 {
			return DesiredConfig{}, fmt.Errorf("%w: healthcheck needs a test", ErrInvalidConfig)
		}
		for _, d := range []string{h.Interval, h.Timeout} {
			if d == "" {
				continue
			}
			if v, err := time.ParseDuration(d); err != nil || v <= 0 {
				return DesiredConfig{}, fmt.Errorf("%w: healthcheck duration %q", ErrInvalidConfig, d)
			}
		}
		if h.Retries < 0 {
			return DesiredConfig{}, fmt.Errorf("%w: healthcheck retries cannot be negative", ErrInvalidConfig)
		}
	}
	for _, list := range [][]string{cfg.Networks, cfg.CapAdd, cfg.CapDrop} {
		for _, v := range list {
			if strings.TrimSpace(v) == "" {
				return DesiredConfig{}, fmt.Errorf("%w: empty network or capability", ErrInvalidConfig)
			}
		}
	}
	if cfg.MemoryBytes < 0 || cfg.NanoCPUs < 0 {
		return DesiredConfig{}, fmt.Errorf("%w: limits cannot be negative", ErrInvalidConfig)
	}
	return cfg, nil
}

// portSpec is a published port: host port, container port and protocol.
type portSpec struct {
	host, container, proto string
}

func (p portSpec) String() string { return p.host + ":" + p.container + "/" + p.proto }

// parsePort reads "8080:80", "8080:80/udp" or "0.0.0.0:8080:80/tcp"; the host
// address is ignored because Docker reports every binding separately anyway.
func parsePort(s string) (portSpec, error) {
	proto := "tcp"
	if i := strings.LastIndex(s, "/"); i >= 0 {
		proto = strings.ToLower(s[i+1:])
		s = s[:i]
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || parts[len(parts)-1] == "" || parts[len(parts)-2] == "" || (proto != "tcp" && proto != "udp" && proto != "sctp") {
		return portSpec{}, fmt.Errorf("%w: port %q is not host:container[/proto]", ErrInvalidConfig, s)
	}
	return portSpec{host: parts[len(parts)-2], container: parts[len(parts)-1], proto: proto}, nil
}

func splitMount(s string) (source, target string, ok bool) {
	i := strings.LastIndex(s, ":")
	if i <= 0 || i == len(s)-1 {
		return "", "", false
	}
	source, target = s[:i], s[i+1:]
	if !strings.HasPrefix(target, "/") {
		if j := strings.LastIndex(source, ":"); j > 0 && strings.HasPrefix(source[j+1:], "/") {
			return source[:j], source[j+1:], true
		}
		return "", "", false
	}
	return source, target, true
}
