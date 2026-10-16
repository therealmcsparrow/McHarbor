// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package swarmsim

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/moby/moby/api/types/swarm"
)

func (c *cluster) serviceByID(id string) *service {
	for _, s := range c.services {
		if s.ID == id {
			return s
		}
	}
	return nil
}

func (c *cluster) findService(ref string) *service {
	for _, s := range c.services {
		if s.ID == ref || s.Spec.Name == ref {
			return s
		}
	}
	for _, s := range c.services {
		if ref != "" && strings.HasPrefix(s.ID, ref) {
			return s
		}
	}
	return nil
}

func (b *backend) services(w http.ResponseWriter, r *http.Request, rest string) {
	if strings.HasSuffix(rest, "/logs") && r.Method == http.MethodGet {
		b.serviceLogs(w, r, strings.TrimSuffix(strings.TrimPrefix(rest, "/"), "/logs"))
		return
	}
	now, unlock := b.lock()
	defer unlock()
	c := b.c
	switch {
	case rest == "" && r.Method == http.MethodGet:
		f := parseFilters(r)
		withStatus := queryBool(r, "status")
		out := []swarm.Service{}
		for _, s := range c.services {
			if b.matchService(f, s) {
				out = append(out, c.serviceDoc(s, withStatus, false))
			}
		}
		writeJSON(w, http.StatusOK, out)
		return
	case rest == "/create" && r.Method == http.MethodPost:
		b.createService(w, r, now)
		return
	}
	ref, action, _ := strings.Cut(strings.TrimPrefix(rest, "/"), "/")
	svc := c.findService(ref)
	if svc == nil {
		writeError(w, http.StatusNotFound, "service "+ref+" not found")
		return
	}
	switch {
	case action == "" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, c.serviceDoc(svc, false, queryBool(r, "insertDefaults")))
	case action == "update" && r.Method == http.MethodPost:
		b.updateService(w, r, svc, now)
	case action == "" && r.Method == http.MethodDelete:
		kept := c.services[:0]
		for _, s := range c.services {
			if s.ID != svc.ID {
				kept = append(kept, s)
			}
		}
		c.services = kept
		tasks := c.tasks[:0]
		for _, t := range c.tasks {
			if t.ServiceID != svc.ID {
				tasks = append(tasks, t)
			}
		}
		c.tasks = tasks
		w.WriteHeader(http.StatusOK)
	default:
		notFound(w)
	}
}

func (c *cluster) serviceDoc(svc *service, withStatus, defaults bool) swarm.Service {
	doc := svc.Service
	if withStatus {
		doc.ServiceStatus = c.status(svc)
	}
	if defaults {
		doc = clone(doc)
		if doc.Spec.UpdateConfig == nil {
			doc.Spec.UpdateConfig = &swarm.UpdateConfig{Parallelism: 1, FailureAction: swarm.UpdateFailureActionPause, Monitor: 5 * time.Second, Order: swarm.UpdateOrderStopFirst}
		}
		if doc.Spec.RollbackConfig == nil {
			doc.Spec.RollbackConfig = &swarm.UpdateConfig{Parallelism: 1, FailureAction: swarm.UpdateFailureActionPause, Monitor: 5 * time.Second, Order: swarm.UpdateOrderStopFirst}
		}
		if doc.Spec.TaskTemplate.RestartPolicy == nil {
			delay := 5 * time.Second
			doc.Spec.TaskTemplate.RestartPolicy = &swarm.RestartPolicy{Condition: swarm.RestartPolicyConditionAny, Delay: &delay}
		}
		if doc.Spec.EndpointSpec == nil {
			doc.Spec.EndpointSpec = &swarm.EndpointSpec{Mode: swarm.ResolutionModeVIP}
		}
	}
	return doc
}

type specError struct {
	status int
	msg    string
}

// normalize validates a submitted spec and resolves networks, secrets and
// configs named by the client to ids, the way the daemon's allocator does.
func (c *cluster) normalize(spec *swarm.ServiceSpec, self string) *specError {
	if strings.TrimSpace(spec.Name) == "" {
		return &specError{http.StatusBadRequest, "rpc error: code = InvalidArgument desc = name must be valid as a DNS name component"}
	}
	cs := spec.TaskTemplate.ContainerSpec
	if cs == nil || cs.Image == "" {
		return &specError{http.StatusBadRequest, "rpc error: code = InvalidArgument desc = ContainerSpec: image reference must be provided"}
	}
	if spec.Mode.ReplicatedJob != nil || spec.Mode.GlobalJob != nil {
		return &specError{http.StatusBadRequest, "Job services are not available in demo mode."}
	}
	if spec.Mode.Global == nil && spec.Mode.Replicated == nil {
		spec.Mode.Replicated = &swarm.ReplicatedService{}
	}
	if spec.Mode.Replicated != nil && spec.Mode.Replicated.Replicas == nil {
		one := uint64(1)
		spec.Mode.Replicated.Replicas = &one
	}
	if spec.Labels == nil {
		spec.Labels = map[string]string{}
	}
	for i, n := range spec.TaskTemplate.Networks {
		o := c.networkByName(n.Target)
		if o == nil {
			return &specError{http.StatusNotFound, "network " + n.Target + " not found"}
		}
		spec.TaskTemplate.Networks[i].Target = o.ID
	}
	for _, ref := range cs.Secrets {
		s := c.findSecret(ref.SecretID)
		if s == nil {
			s = c.findSecret(ref.SecretName)
		}
		if s == nil {
			return &specError{http.StatusBadRequest, "rpc error: code = InvalidArgument desc = secret not found: " + ref.SecretName + ref.SecretID}
		}
		ref.SecretID, ref.SecretName = s.ID, s.Spec.Name
	}
	for _, ref := range cs.Configs {
		cfg := c.findConfig(ref.ConfigID)
		if cfg == nil {
			cfg = c.findConfig(ref.ConfigName)
		}
		if cfg == nil {
			return &specError{http.StatusBadRequest, "rpc error: code = InvalidArgument desc = config not found: " + ref.ConfigName + ref.ConfigID}
		}
		ref.ConfigID, ref.ConfigName = cfg.ID, cfg.Spec.Name
	}
	if spec.EndpointSpec != nil {
		for _, p := range spec.EndpointSpec.Ports {
			if p.PublishedPort == 0 {
				continue
			}
			for _, other := range c.services {
				if other.ID == self {
					continue
				}
				for _, op := range other.Endpoint.Ports {
					if op.PublishedPort == p.PublishedPort && (p.Protocol == "" || op.Protocol == p.Protocol) {
						return &specError{http.StatusBadRequest, fmt.Sprintf("rpc error: code = InvalidArgument desc = port '%d' is already in use by service '%s' (%s) as an ingress port", p.PublishedPort, other.Spec.Name, other.ID)}
					}
				}
			}
		}
	}
	return nil
}

func (b *backend) createService(w http.ResponseWriter, r *http.Request, now time.Time) {
	c := b.c
	var spec swarm.ServiceSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		writeError(w, http.StatusBadRequest, "invalid service spec")
		return
	}
	if e := c.normalize(&spec, ""); e != nil {
		writeError(w, e.status, e.msg)
		return
	}
	if c.nameTaken(spec.Name, "service") {
		writeError(w, http.StatusConflict, "rpc error: code = AlreadyExists desc = name conflicts with an existing object: service "+spec.Name+" already exists")
		return
	}
	svc := &service{profile: profileFor(spec.TaskTemplate.ContainerSpec.Image)}
	svc.ID = c.r.id(25)
	svc.CreatedAt, svc.UpdatedAt = now, now
	svc.Spec = spec
	svc.Version = c.bump()
	c.endpoint(svc)
	c.services = append(c.services, svc)
	c.reconcile(svc, now)
	writeJSON(w, http.StatusCreated, swarm.ServiceCreateResponse{ID: svc.ID})
}

func (b *backend) updateService(w http.ResponseWriter, r *http.Request, svc *service, now time.Time) {
	c := b.c
	if !checkVersion(w, r, svc.Version) {
		return
	}
	var spec swarm.ServiceSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		writeError(w, http.StatusBadRequest, "invalid service spec")
		return
	}
	rollback := r.URL.Query().Get("rollback") == "previous"
	if rollback {
		if svc.PreviousSpec == nil {
			writeError(w, http.StatusBadRequest, "rpc error: code = FailedPrecondition desc = service "+svc.ID+" does not have a previous spec")
			return
		}
		spec = clone(*svc.PreviousSpec)
	} else if e := c.normalize(&spec, svc.ID); e != nil {
		writeError(w, e.status, e.msg)
		return
	}
	if spec.Name != svc.Spec.Name {
		writeError(w, http.StatusBadRequest, "rpc error: code = Unimplemented desc = renaming services is not supported")
		return
	}
	if (spec.Mode.Global != nil) != (svc.Spec.Mode.Global != nil) {
		writeError(w, http.StatusBadRequest, "rpc error: code = Unimplemented desc = service mode change is not allowed")
		return
	}
	changed := specKey(spec.TaskTemplate) != specKey(svc.Spec.TaskTemplate)
	prev := clone(svc.Spec)
	svc.PreviousSpec = &prev
	svc.Spec = spec
	svc.Version = c.bump()
	svc.UpdatedAt = now
	c.endpoint(svc)
	c.reconcile(svc, now)
	if changed || rollback {
		state := swarm.UpdateStateUpdating
		if rollback {
			state = swarm.UpdateStateRollbackStarted
		}
		c.roll(svc, now, state)
	}
	writeJSON(w, http.StatusOK, swarm.ServiceUpdateResponse{})
}
