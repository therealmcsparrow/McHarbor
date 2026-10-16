// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/envredact"
)

// stackNamespaceLabel is the label `docker stack deploy` puts on every service it creates.
const stackNamespaceLabel = "com.docker.stack.namespace"

// isMcHarborService reports whether a Swarm service runs McHarbor, whose own keys stay
// withheld even from a permitted reveal. It asks core/docker's detector, offering the
// service as the compose service it was deployed from: a stack service is named
// <stack>_<service>, so an exact match on "mcharbor" missed every stack deployment.
func isMcHarborService(name, image string, labels map[string]string) bool {
	name = strings.TrimSpace(name)
	short := name
	ns := labels[stackNamespaceLabel]
	if ns != "" {
		short = strings.TrimPrefix(name, ns+"_")
	}
	probe := maps.Clone(labels)
	if probe == nil {
		probe = map[string]string{}
	}
	if ns != "" {
		probe["com.docker.compose.project"] = ns
		probe["com.docker.compose.service"] = short
	}
	return docker.IsMcHarborContainer([]string{name, short}, image, probe)
}

// servicePolicy is the redaction rule for one service: reveal is the caller's manage-level
// request, which the route has already authorised. Values carrying the keys the host
// registered as pinned stay withheld through envredact's process-wide set, which this
// plugin shares with the host without a handle.
func servicePolicy(name, image string, labels map[string]string, reveal bool) envredact.Policy {
	return envredact.Policy{Reveal: reveal, McHarbor: isMcHarborService(name, image, labels)}
}

// ErrMaskedValueMoved is returned when an update keeps a value nobody may reveal by its
// marker while changing the image, command, arguments or labels it is handed to.
var ErrMaskedValueMoved = errors.New("service update keeps a withheld value while changing what runs with it")

// guardMovedWithheld refuses an update that keeps an unrevealable value (a pinned key, or a
// McHarbor service's own keys) while changing what the service runs: the editor cannot read
// that value, so they must not choose the program it goes to.
func guardMovedWithheld(req ServiceWriteRequest, name string, labels map[string]string, cs *swarm.ContainerSpec) error {
	policy := servicePolicy(name, cs.Image, serviceLabels(labels, cs.Labels), false)
	moved := policy.MovedWithheld(req.Env, cs.Env)
	if len(moved) == 0 {
		return nil
	}
	changed := (req.Image != "" && req.Image != cs.Image) ||
		(req.Command != nil && !slices.Equal(req.Command, cs.Command)) ||
		(req.Args != nil && !slices.Equal(req.Args, cs.Args)) ||
		(req.Labels != nil && !maps.Equal(req.Labels, labels))
	if !changed {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrMaskedValueMoved, strings.Join(moved, ","))
}

// serviceLabels merges a service's own labels with its container labels, which is where an
// image's role label reaches a Swarm task.
func serviceLabels(service, container map[string]string) map[string]string {
	out := maps.Clone(service)
	if out == nil {
		out = map[string]string{}
	}
	maps.Copy(out, container)
	return out
}

// ServiceEnvResult is one service's revealed environment, its name for the audit row, and
// the names that stayed withheld.
type ServiceEnvResult struct {
	Env      []string
	Name     string
	Withheld []string
}

// ServiceEnv returns one service's environment with credential-looking values in the clear,
// for the swarm_services.manage route the manifest audits. McHarbor's own keys stay withheld.
func (s *Service) ServiceEnv(ctx context.Context, envID, id string) (ServiceEnvResult, error) {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return ServiceEnvResult{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	svcRes, err := cli.ServiceInspect(ctx, id, client.ServiceInspectOptions{})
	if err != nil {
		return ServiceEnvResult{}, fmt.Errorf("service inspect %s: %w", id, err)
	}
	spec := svcRes.Service.Spec
	cs := spec.TaskTemplate.ContainerSpec
	if cs == nil {
		return ServiceEnvResult{Env: []string{}, Name: spec.Name}, nil
	}
	env, withheld := servicePolicy(spec.Name, cs.Image, serviceLabels(spec.Labels, cs.Labels), true).EnvList(cs.Env)
	if env == nil {
		env = []string{}
	}
	return ServiceEnvResult{Env: env, Name: spec.Name, Withheld: withheld}, nil
}
