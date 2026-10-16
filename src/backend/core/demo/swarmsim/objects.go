// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package swarmsim

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/moby/moby/api/types/swarm"
)

func (c *cluster) findSecret(ref string) *swarm.Secret {
	if ref == "" {
		return nil
	}
	for _, s := range c.secrets {
		if s.ID == ref || s.Spec.Name == ref {
			return s
		}
	}
	for _, s := range c.secrets {
		if strings.HasPrefix(s.ID, ref) {
			return s
		}
	}
	return nil
}

func (c *cluster) findConfig(ref string) *swarm.Config {
	if ref == "" {
		return nil
	}
	for _, s := range c.configs {
		if s.ID == ref || s.Spec.Name == ref {
			return s
		}
	}
	for _, s := range c.configs {
		if strings.HasPrefix(s.ID, ref) {
			return s
		}
	}
	return nil
}

// nameTaken checks names exactly; the id-prefix resolution used for lookups
// would otherwise make a short new name collide with an unrelated object's id.
func (c *cluster) nameTaken(name, kind string) bool {
	switch kind {
	case "secret":
		for _, s := range c.secrets {
			if s.Spec.Name == name {
				return true
			}
		}
	case "config":
		for _, s := range c.configs {
			if s.Spec.Name == name {
				return true
			}
		}
	case "service":
		for _, s := range c.services {
			if s.Spec.Name == name {
				return true
			}
		}
	}
	return false
}

// usersOf names the services that mount a secret or config; Docker refuses to
// remove either while anything still references it.
func (c *cluster) usersOf(secretID, configID string) []string {
	var out []string
	for _, svc := range c.services {
		cs := svc.Spec.TaskTemplate.ContainerSpec
		if cs == nil {
			continue
		}
		for _, ref := range cs.Secrets {
			if secretID != "" && ref.SecretID == secretID {
				out = append(out, svc.Spec.Name)
			}
		}
		for _, ref := range cs.Configs {
			if configID != "" && ref.ConfigID == configID {
				out = append(out, svc.Spec.Name)
			}
		}
	}
	sort.Strings(out)
	return out
}

func inUse(kind, name string, users []string) string {
	noun := "service"
	if len(users) > 1 {
		noun = "services"
	}
	return "rpc error: code = InvalidArgument desc = " + kind + " '" + name + "' is in use by the following " + noun + ": " + strings.Join(users, ", ")
}

func (b *backend) secretsRoute(w http.ResponseWriter, r *http.Request, rest string) {
	now, unlock := b.lock()
	defer unlock()
	c := b.c
	switch {
	case rest == "" && r.Method == http.MethodGet:
		f := parseFilters(r)
		out := []swarm.Secret{}
		for _, s := range c.secrets {
			if matchObject(f, s.ID, s.Spec.Name, s.Spec.Labels) {
				out = append(out, *s)
			}
		}
		writeJSON(w, http.StatusOK, out)
		return
	case rest == "/create" && r.Method == http.MethodPost:
		var spec swarm.SecretSpec
		if err := json.NewDecoder(r.Body).Decode(&spec); err != nil || strings.TrimSpace(spec.Name) == "" {
			writeError(w, http.StatusBadRequest, "rpc error: code = InvalidArgument desc = name must be valid as a DNS name component")
			return
		}
		if len(spec.Data) == 0 || len(spec.Data) > 500*1024 {
			writeError(w, http.StatusBadRequest, "rpc error: code = InvalidArgument desc = secret data must be larger than 0 and less than 500KB")
			return
		}
		if c.nameTaken(spec.Name, "secret") {
			writeError(w, http.StatusConflict, "rpc error: code = AlreadyExists desc = secret "+spec.Name+" already exists")
			return
		}
		s := c.addSecret(spec.Name, spec.Labels, now)
		s.Spec.Driver = spec.Driver
		writeJSON(w, http.StatusCreated, swarm.SecretCreateResponse{ID: s.ID})
		return
	}
	ref, action, _ := strings.Cut(strings.TrimPrefix(rest, "/"), "/")
	s := c.findSecret(ref)
	if s == nil {
		writeError(w, http.StatusNotFound, "secret "+ref+" not found")
		return
	}
	switch {
	case action == "" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, s)
	case action == "update" && r.Method == http.MethodPost:
		if !checkVersion(w, r, s.Version) {
			return
		}
		var spec swarm.SecretSpec
		if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
			writeError(w, http.StatusBadRequest, "invalid secret spec")
			return
		}
		if (spec.Name != "" && spec.Name != s.Spec.Name) || len(spec.Data) > 0 {
			writeError(w, http.StatusBadRequest, "rpc error: code = InvalidArgument desc = only updates to Labels are allowed")
			return
		}
		if spec.Labels == nil {
			spec.Labels = map[string]string{}
		}
		s.Spec.Labels = spec.Labels
		s.Version = c.bump()
		s.UpdatedAt = now
		w.WriteHeader(http.StatusOK)
	case action == "" && r.Method == http.MethodDelete:
		if users := c.usersOf(s.ID, ""); len(users) > 0 {
			writeError(w, http.StatusBadRequest, inUse("secret", s.Spec.Name, users))
			return
		}
		kept := c.secrets[:0]
		for _, x := range c.secrets {
			if x.ID != s.ID {
				kept = append(kept, x)
			}
		}
		c.secrets = kept
		w.WriteHeader(http.StatusNoContent)
	default:
		notFound(w)
	}
}

func (b *backend) configsRoute(w http.ResponseWriter, r *http.Request, rest string) {
	now, unlock := b.lock()
	defer unlock()
	c := b.c
	switch {
	case rest == "" && r.Method == http.MethodGet:
		f := parseFilters(r)
		out := []swarm.Config{}
		for _, s := range c.configs {
			if matchObject(f, s.ID, s.Spec.Name, s.Spec.Labels) {
				out = append(out, *s)
			}
		}
		writeJSON(w, http.StatusOK, out)
		return
	case rest == "/create" && r.Method == http.MethodPost:
		var spec swarm.ConfigSpec
		if err := json.NewDecoder(r.Body).Decode(&spec); err != nil || strings.TrimSpace(spec.Name) == "" {
			writeError(w, http.StatusBadRequest, "rpc error: code = InvalidArgument desc = name must be valid as a DNS name component")
			return
		}
		if len(spec.Data) == 0 || len(spec.Data) > 1000*1024 {
			writeError(w, http.StatusBadRequest, "rpc error: code = InvalidArgument desc = config data must be larger than 0 and less than 1000KB")
			return
		}
		if c.nameTaken(spec.Name, "config") {
			writeError(w, http.StatusConflict, "rpc error: code = AlreadyExists desc = config "+spec.Name+" already exists")
			return
		}
		cfg := c.addConfig(spec.Name, spec.Labels, spec.Data, now)
		writeJSON(w, http.StatusCreated, swarm.ConfigCreateResponse{ID: cfg.ID})
		return
	}
	ref, action, _ := strings.Cut(strings.TrimPrefix(rest, "/"), "/")
	cfg := c.findConfig(ref)
	if cfg == nil {
		writeError(w, http.StatusNotFound, "config "+ref+" not found")
		return
	}
	switch {
	case action == "" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, cfg)
	case action == "update" && r.Method == http.MethodPost:
		if !checkVersion(w, r, cfg.Version) {
			return
		}
		var spec swarm.ConfigSpec
		if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
			writeError(w, http.StatusBadRequest, "invalid config spec")
			return
		}
		if (spec.Name != "" && spec.Name != cfg.Spec.Name) || (len(spec.Data) > 0 && string(spec.Data) != string(cfg.Spec.Data)) {
			writeError(w, http.StatusBadRequest, "rpc error: code = InvalidArgument desc = only updates to Labels are allowed")
			return
		}
		if spec.Labels == nil {
			spec.Labels = map[string]string{}
		}
		cfg.Spec.Labels = spec.Labels
		cfg.Version = c.bump()
		cfg.UpdatedAt = now
		w.WriteHeader(http.StatusOK)
	case action == "" && r.Method == http.MethodDelete:
		if users := c.usersOf("", cfg.ID); len(users) > 0 {
			writeError(w, http.StatusBadRequest, inUse("config", cfg.Spec.Name, users))
			return
		}
		kept := c.configs[:0]
		for _, x := range c.configs {
			if x.ID != cfg.ID {
				kept = append(kept, x)
			}
		}
		c.configs = kept
		w.WriteHeader(http.StatusNoContent)
	default:
		notFound(w)
	}
}
