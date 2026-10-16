// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"
	"gopkg.in/yaml.v3"
)

// errNoServices is returned when a compose document declares no services.
var errNoServices = errors.New("compose file declares no services")

// ErrStackRejected means the compose file used keys that have no meaning
// under Swarm. The per-key detail travels in the result, not in the error.
var ErrStackRejected = errors.New("compose file contains keys Swarm cannot honour")

// ListStacks groups services by their com.docker.stack.namespace label.
//
// One ServiceList with Status:true answers the whole page — service counts and
// task counts included. Services without the label are standalone and are not
// stacks, so they do not appear here.
func (s *Service) ListStacks(ctx context.Context, envID string) ([]Stack, error) {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	servicesRes, err := cli.ServiceList(ctx, client.ServiceListOptions{Status: true})
	if err != nil {
		return nil, fmt.Errorf("service list: %w", err)
	}
	services := servicesRes.Items

	names := networkNameMap(ctx, cli)

	grouped := map[string]*Stack{}
	networksByStack := map[string]map[string]struct{}{}
	secretsByStack := map[string]map[string]struct{}{}
	configsByStack := map[string]map[string]struct{}{}

	for _, svc := range services {
		name := stackOf(svc.Spec.Labels)
		if name == "" {
			continue
		}

		st, ok := grouped[name]
		if !ok {
			st = &Stack{Name: name, CreatedAt: svc.CreatedAt, UpdatedAt: svc.UpdatedAt}
			grouped[name] = st
			networksByStack[name] = map[string]struct{}{}
			secretsByStack[name] = map[string]struct{}{}
			configsByStack[name] = map[string]struct{}{}
		}

		st.Services++
		if svc.ServiceStatus != nil {
			st.RunningTasks += svc.ServiceStatus.RunningTasks
			st.DesiredTasks += svc.ServiceStatus.DesiredTasks
		}
		if svc.CreatedAt.Before(st.CreatedAt) {
			st.CreatedAt = svc.CreatedAt
		}
		if svc.UpdatedAt.After(st.UpdatedAt) {
			st.UpdatedAt = svc.UpdatedAt
		}

		for _, n := range svc.Spec.TaskTemplate.Networks {
			networksByStack[name][networkLabel(n.Target, names)] = struct{}{}
		}
		if cs := svc.Spec.TaskTemplate.ContainerSpec; cs != nil {
			for _, ref := range cs.Secrets {
				secretsByStack[name][ref.SecretName] = struct{}{}
			}
			for _, ref := range cs.Configs {
				configsByStack[name][ref.ConfigName] = struct{}{}
			}
		}
	}

	out := make([]Stack, 0, len(grouped))
	for name, st := range grouped {
		st.Networks = sortedNames(networksByStack[name])
		st.Secrets = sortedNames(secretsByStack[name])
		st.Configs = sortedNames(configsByStack[name])
		out = append(out, *st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// InspectStack returns one stack plus its member services.
func (s *Service) InspectStack(ctx context.Context, envID, name string) (StackDetail, error) {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return StackDetail{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	args := make(client.Filters)
	args.Add("label", StackNamespaceLabel+"="+name)

	servicesRes, err := cli.ServiceList(ctx, client.ServiceListOptions{Filters: args, Status: true})
	if err != nil {
		return StackDetail{}, fmt.Errorf("service list for stack %s: %w", name, err)
	}
	services := servicesRes.Items
	if len(services) == 0 {
		return StackDetail{}, ErrStackNotFound
	}

	names := networkNameMap(ctx, cli)

	detail := StackDetail{Stack: Stack{Name: name}}
	networks := map[string]struct{}{}
	secrets := map[string]struct{}{}
	configs := map[string]struct{}{}

	detail.CreatedAt = services[0].CreatedAt
	for _, svc := range services {
		summary := mapService(svc)
		resolveNetworkNames(&summary, names)
		detail.ServiceList = append(detail.ServiceList, summary)
		detail.Services++
		detail.RunningTasks += summary.Running
		detail.DesiredTasks += summary.Desired

		if svc.CreatedAt.Before(detail.CreatedAt) {
			detail.CreatedAt = svc.CreatedAt
		}
		if svc.UpdatedAt.After(detail.UpdatedAt) {
			detail.UpdatedAt = svc.UpdatedAt
		}

		for _, n := range svc.Spec.TaskTemplate.Networks {
			networks[networkLabel(n.Target, names)] = struct{}{}
		}
		if cs := svc.Spec.TaskTemplate.ContainerSpec; cs != nil {
			for _, ref := range cs.Secrets {
				secrets[ref.SecretName] = struct{}{}
			}
			for _, ref := range cs.Configs {
				configs[ref.ConfigName] = struct{}{}
			}
		}
	}

	detail.Networks = sortedNames(networks)
	detail.Secrets = sortedNames(secrets)
	detail.Configs = sortedNames(configs)
	sort.Slice(detail.ServiceList, func(i, j int) bool {
		return detail.ServiceList[i].Name < detail.ServiceList[j].Name
	})
	return detail, nil
}

// ErrStackNotFound is returned when no service carries the stack's label.
var ErrStackNotFound = errors.New("stack not found")

// networkLabel resolves a network id to its name, falling back to the id when
// the network is gone or the listing failed.
func networkLabel(target string, names map[string]string) string {
	if name, ok := names[target]; ok && name != "" {
		return name
	}
	return target
}

// DeployStack applies a compose document to the cluster.
//
// The sequence mirrors what `docker stack deploy` does client-side: reject
// what Swarm cannot honour, make the declared networks exist, then create or
// update each service. A mid-deploy failure is reported per service rather
// than rolled back — a partial deploy the operator can see and finish beats a
// silent revert that loses the services that did land.
func (s *Service) DeployStack(ctx context.Context, envID, name, composeYAML string, prune bool) (DeployStackResult, error) {
	result := DeployStackResult{Name: name}

	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return result, err
	}

	file, rejected, err := parseCompose(composeYAML)
	if err != nil {
		return result, err
	}
	if len(rejected) > 0 {
		result.Rejected = rejected
		return result, ErrStackRejected
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// --- networks ---------------------------------------------------------
	networkTargets, err := s.ensureStackNetworks(ctx, cli, name, file)
	if err != nil {
		return result, err
	}
	for _, target := range networkTargets {
		result.Networks = append(result.Networks, target)
	}

	namedVolumes := map[string]struct{}{}
	for volName := range file.Volumes {
		namedVolumes[volName] = struct{}{}
	}

	// --- existing services ------------------------------------------------
	args := make(client.Filters)
	args.Add("label", StackNamespaceLabel+"="+name)
	existingRes, err := cli.ServiceList(ctx, client.ServiceListOptions{Filters: args})
	if err != nil {
		return result, fmt.Errorf("service list for stack %s: %w", name, err)
	}
	existing := existingRes.Items
	byName := map[string]swarm.Service{}
	for _, svc := range existing {
		byName[svc.Spec.Name] = svc
	}

	// --- services ---------------------------------------------------------
	serviceNames := make([]string, 0, len(file.Services))
	for svcName := range file.Services {
		serviceNames = append(serviceNames, svcName)
	}
	sort.Strings(serviceNames)

	deployed := map[string]struct{}{}
	for _, svcName := range serviceNames {
		fullName := name + "_" + svcName
		deployed[fullName] = struct{}{}

		spec, specErr := s.specFromCompose(name, svcName, file.Services[svcName], file, namedVolumes, networkTargets)
		if specErr != nil {
			result.Rejected = append(result.Rejected, StackServiceError{Service: svcName, Reason: specErr.Error()})
			continue
		}

		if prev, found := byName[fullName]; found {
			resp, updErr := cli.ServiceUpdate(ctx, prev.ID, client.ServiceUpdateOptions{Version: prev.Version, Spec: spec, QueryRegistry: true})
			if updErr != nil {
				result.Rejected = append(result.Rejected, StackServiceError{Service: svcName, Reason: updErr.Error()})
				continue
			}
			s.logWarnings(resp.Warnings, "stack-update", fullName)
			result.Updated = append(result.Updated, fullName)
			continue
		}

		resp, createErr := cli.ServiceCreate(ctx, client.ServiceCreateOptions{Spec: spec, QueryRegistry: true})
		if createErr != nil {
			result.Rejected = append(result.Rejected, StackServiceError{Service: svcName, Reason: createErr.Error()})
			continue
		}
		s.logWarnings(resp.Warnings, "stack-create", fullName)
		result.Created = append(result.Created, fullName)
	}

	// --- prune ------------------------------------------------------------
	if prune {
		for _, svc := range existing {
			if _, kept := deployed[svc.Spec.Name]; kept {
				continue
			}
			if _, rmErr := cli.ServiceRemove(ctx, svc.ID, client.ServiceRemoveOptions{}); rmErr != nil {
				result.Rejected = append(result.Rejected, StackServiceError{
					Service: svc.Spec.Name,
					Reason:  rmErr.Error(),
				})
				continue
			}
			result.Removed = append(result.Removed, svc.Spec.Name)
		}
	}

	if len(result.Rejected) > 0 {
		return result, ErrStackRejected
	}
	return result, nil
}

// ensureStackNetworks creates the overlay networks a stack declares and
// returns the map from compose-local name to the real network name.
func (s *Service) ensureStackNetworks(
	ctx context.Context,
	cli dockerNetworkClient,
	stack string,
	file composeFile,
) (map[string]string, error) {
	targets := map[string]string{}

	for netName, netSpec := range file.Networks {
		if isExternal(netSpec.External) {
			// An external network is somebody else's object; the stack only
			// attaches to it and must never create or relabel it.
			real := netSpec.Name
			if real == "" {
				real = externalName(netSpec.External)
			}
			if real == "" {
				real = netName
			}
			targets[netName] = real
			continue
		}

		real := netSpec.Name
		if real == "" {
			real = stack + "_" + netName
		}
		targets[netName] = real

		if _, err := cli.NetworkInspect(ctx, real, client.NetworkInspectOptions{}); err == nil {
			continue
		}

		labels := keyValues(netSpec.Labels)
		if labels == nil {
			labels = map[string]string{}
		}
		labels[StackNamespaceLabel] = stack

		driver := defaultString(netSpec.Driver, "overlay")
		if _, err := cli.NetworkCreate(ctx, real, client.NetworkCreateOptions{
			Driver:     driver,
			Attachable: netSpec.Attachable,
			Internal:   netSpec.Internal,
			Labels:     labels,
			Options:    netSpec.DriverOpts,
		}); err != nil {
			return nil, fmt.Errorf("create network %s: %w", real, err)
		}
	}

	return targets, nil
}

// dockerNetworkClient is the network slice of the Docker client used by the
// stack deploy path.
type dockerNetworkClient interface {
	NetworkInspect(ctx context.Context, id string, options client.NetworkInspectOptions) (client.NetworkInspectResult, error)
	NetworkCreate(ctx context.Context, name string, options client.NetworkCreateOptions) (client.NetworkCreateResult, error)
}

// specFromCompose converts one compose service into a SwarmKit ServiceSpec.
func (s *Service) specFromCompose(
	stack, svcName string,
	svc composeService,
	file composeFile,
	namedVolumes map[string]struct{},
	networkTargets map[string]string,
) (swarm.ServiceSpec, error) {
	fullName := stack + "_" + svcName

	labels := keyValues(svc.Deploy.Labels)
	if labels == nil {
		labels = map[string]string{}
	}
	labels[StackNamespaceLabel] = stack

	containerLabels := keyValues(svc.Labels)
	if containerLabels == nil {
		containerLabels = map[string]string{}
	}
	containerLabels[StackNamespaceLabel] = stack

	container := &swarm.ContainerSpec{
		Image:      svc.Image,
		Labels:     containerLabels,
		Command:    stringList(svc.Entrypoint),
		Args:       stringList(svc.Command),
		Env:        envList(svc.Environment),
		Hostname:   svc.Hostname,
		User:       svc.User,
		Dir:        svc.WorkingDir,
		StopSignal: svc.StopSignal,
		TTY:        svc.TTY,
		OpenStdin:  svc.StdinOpen,
		ReadOnly:   svc.ReadOnly,
	}

	for _, volNode := range svc.Volumes {
		m, err := parseVolume(volNode, stack, namedVolumes)
		if err != nil {
			return swarm.ServiceSpec{}, err
		}
		container.Mounts = append(container.Mounts, mount.Mount{
			Type:     mount.Type(m.Type),
			Source:   m.Source,
			Target:   m.Target,
			ReadOnly: m.ReadOnly,
		})
	}

	for _, refNode := range svc.Secrets {
		source, target := parseFileRef(refNode)
		if source == "" {
			continue
		}
		container.Secrets = append(container.Secrets, &swarm.SecretReference{
			SecretName: externalObjectName(stack, source, file.Secrets),
			File: &swarm.SecretReferenceFileTarget{
				Name: defaultString(target, source),
				UID:  "0",
				GID:  "0",
				Mode: 0o444,
			},
		})
	}
	for _, refNode := range svc.Configs {
		source, target := parseFileRef(refNode)
		if source == "" {
			continue
		}
		container.Configs = append(container.Configs, &swarm.ConfigReference{
			ConfigName: externalObjectName(stack, source, file.Configs),
			File: &swarm.ConfigReferenceFileTarget{
				Name: defaultString(target, source),
				UID:  "0",
				GID:  "0",
				Mode: 0o444,
			},
		})
	}

	spec := swarm.ServiceSpec{TaskTemplate: swarm.TaskSpec{ContainerSpec: container}}
	spec.Name = fullName
	spec.Labels = labels

	// --- mode -------------------------------------------------------------
	if strings.EqualFold(svc.Deploy.Mode, "global") {
		spec.Mode = swarm.ServiceMode{Global: &swarm.GlobalService{}}
	} else {
		replicas := uint64(1)
		if svc.Deploy.Replicas != nil {
			replicas = *svc.Deploy.Replicas
		}
		spec.Mode = swarm.ServiceMode{Replicated: &swarm.ReplicatedService{Replicas: &replicas}}
	}

	// --- placement --------------------------------------------------------
	if len(svc.Deploy.Placement.Constraints) > 0 || svc.Deploy.Placement.MaxReplicas > 0 {
		spec.TaskTemplate.Placement = &swarm.Placement{
			Constraints: svc.Deploy.Placement.Constraints,
			MaxReplicas: svc.Deploy.Placement.MaxReplicas,
		}
	}

	// --- resources --------------------------------------------------------
	resources := &swarm.ResourceRequirements{}
	hasResources := false
	if limitCPU, limitMem, err := resourceEntry(svc.Deploy.Resources.Limits); err != nil {
		return swarm.ServiceSpec{}, err
	} else if limitCPU > 0 || limitMem > 0 {
		resources.Limits = &swarm.Limit{NanoCPUs: limitCPU, MemoryBytes: limitMem}
		hasResources = true
	}
	if resCPU, resMem, err := resourceEntry(svc.Deploy.Resources.Reservations); err != nil {
		return swarm.ServiceSpec{}, err
	} else if resCPU > 0 || resMem > 0 {
		resources.Reservations = &swarm.Resources{NanoCPUs: resCPU, MemoryBytes: resMem}
		hasResources = true
	}
	if hasResources {
		spec.TaskTemplate.Resources = resources
	}

	// --- restart policy ---------------------------------------------------
	if rp := svc.Deploy.RestartPolicy; rp.Condition != "" || rp.Delay != "" || rp.MaxAttempts != nil {
		policy := &swarm.RestartPolicy{
			Condition: swarm.RestartPolicyCondition(defaultString(rp.Condition, "any")),
		}
		if rp.Delay != "" {
			d, err := parseDuration(rp.Delay)
			if err != nil {
				return swarm.ServiceSpec{}, err
			}
			policy.Delay = &d
		}
		if rp.MaxAttempts != nil {
			attempts := *rp.MaxAttempts
			policy.MaxAttempts = &attempts
		}
		if rp.Window != "" {
			w, err := parseDuration(rp.Window)
			if err != nil {
				return swarm.ServiceSpec{}, err
			}
			policy.Window = &w
		}
		spec.TaskTemplate.RestartPolicy = policy
	}

	// --- rollout ----------------------------------------------------------
	if uc := svc.Deploy.UpdateConfig; uc.Parallelism != nil || uc.Delay != "" ||
		uc.FailureAction != "" || uc.Order != "" {
		update := &swarm.UpdateConfig{
			FailureAction: swarm.FailureAction(uc.FailureAction),
			Order:         swarm.UpdateOrder(uc.Order),
		}
		if uc.Parallelism != nil {
			update.Parallelism = *uc.Parallelism
		}
		if uc.Delay != "" {
			d, err := parseDuration(uc.Delay)
			if err != nil {
				return swarm.ServiceSpec{}, err
			}
			update.Delay = d
		}
		spec.UpdateConfig = update
	}

	// --- networks ---------------------------------------------------------
	for _, netName := range networkNames(svc.Networks) {
		target, known := networkTargets[netName]
		if !known {
			// A network used by a service but never declared at the top
			// level: Compose treats that as an error, and so does this.
			return swarm.ServiceSpec{}, fmt.Errorf("service %s attaches to undeclared network %q", svcName, netName)
		}
		spec.TaskTemplate.Networks = append(spec.TaskTemplate.Networks,
			swarm.NetworkAttachmentConfig{Target: target})
	}

	// --- ports ------------------------------------------------------------
	if len(svc.Ports) > 0 {
		endpoint := &swarm.EndpointSpec{}
		if svc.Deploy.EndpointMode != "" {
			endpoint.Mode = swarm.ResolutionMode(svc.Deploy.EndpointMode)
		}
		for _, portNode := range svc.Ports {
			p, err := parsePort(portNode)
			if err != nil {
				return swarm.ServiceSpec{}, err
			}
			endpoint.Ports = append(endpoint.Ports, swarm.PortConfig{
				Protocol:      network.IPProtocol(p.Protocol),
				TargetPort:    p.TargetPort,
				PublishedPort: p.PublishedPort,
				PublishMode:   swarm.PortConfigPublishMode(p.PublishMode),
			})
		}
		spec.EndpointSpec = endpoint
	}

	return spec, nil
}

// resourceEntry converts one limits/reservations block.
func resourceEntry(entry composeResourceEntry) (nanoCPUs, memoryBytes int64, err error) {
	nanoCPUs, err = parseCPUs(entry.CPUs)
	if err != nil {
		return 0, 0, err
	}
	memoryBytes, err = parseMemory(entry.Memory)
	if err != nil {
		return 0, 0, err
	}
	return nanoCPUs, memoryBytes, nil
}

// externalObjectName resolves a secret/config reference to the name that
// actually exists in the cluster. A stack-owned object is namespaced; an
// external one keeps the name it was created with.
func externalObjectName(stack, ref string, declared map[string]composeExtern) string {
	spec, found := declared[ref]
	if !found {
		return ref
	}
	if isExternal(spec.External) {
		if spec.Name != "" {
			return spec.Name
		}
		if name := externalName(spec.External); name != "" {
			return name
		}
		return ref
	}
	if spec.Name != "" {
		return spec.Name
	}
	return stack + "_" + ref
}

// RemoveStack deletes a stack's services, then its networks, then its secrets
// and configs.
//
// The order is load-bearing: Docker refuses to remove a network while a
// service is still attached, and refuses to remove a secret while a service
// still references it. Reversing this leaves half the stack behind with an
// error that reads like a permissions problem.
func (s *Service) RemoveStack(ctx context.Context, envID, name string) error {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	label := make(client.Filters)
	label.Add("label", StackNamespaceLabel+"="+name)

	servicesRes, err := cli.ServiceList(ctx, client.ServiceListOptions{Filters: label})
	if err != nil {
		return fmt.Errorf("service list for stack %s: %w", name, err)
	}
	services := servicesRes.Items
	if len(services) == 0 {
		return ErrStackNotFound
	}
	for _, svc := range services {
		if _, rmErr := cli.ServiceRemove(ctx, svc.ID, client.ServiceRemoveOptions{}); rmErr != nil {
			return fmt.Errorf("remove service %s: %w", svc.Spec.Name, rmErr)
		}
	}

	// Services are gone from the store immediately but their tasks take a
	// moment to release the network attachments.
	if networks, nErr := cli.NetworkList(ctx, client.NetworkListOptions{Filters: label}); nErr == nil {
		for _, n := range networks.Items {
			if err := s.removeNetworkWithRetry(ctx, cli, n.ID); err != nil {
				s.logger.Warn("swarm stack network not removed", "stack", name, "network", n.Name, "error", err)
			}
		}
	}

	if secrets, sErr := cli.SecretList(ctx, client.SecretListOptions{Filters: label}); sErr == nil {
		for _, sec := range secrets.Items {
			if _, err := cli.SecretRemove(ctx, sec.ID, client.SecretRemoveOptions{}); err != nil {
				s.logger.Warn("swarm stack secret not removed", "stack", name, "secret", sec.Spec.Name, "error", err)
			}
		}
	}
	if configs, cErr := cli.ConfigList(ctx, client.ConfigListOptions{Filters: label}); cErr == nil {
		for _, cfg := range configs.Items {
			if _, err := cli.ConfigRemove(ctx, cfg.ID, client.ConfigRemoveOptions{}); err != nil {
				s.logger.Warn("swarm stack config not removed", "stack", name, "config", cfg.Spec.Name, "error", err)
			}
		}
	}

	return nil
}

// removeNetworkWithRetry waits out the window in which a just-removed
// service's tasks still hold their endpoints.
func (s *Service) removeNetworkWithRetry(ctx context.Context, cli dockerNetworkRemover, id string) error {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if _, err := cli.NetworkRemove(ctx, id, client.NetworkRemoveOptions{}); err == nil {
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * time.Second):
		}
	}
	return lastErr
}

// dockerNetworkRemover is the one call removeNetworkWithRetry needs.
type dockerNetworkRemover interface {
	NetworkRemove(ctx context.Context, id string, options client.NetworkRemoveOptions) (client.NetworkRemoveResult, error)
}

// ListOverlayNetworks returns the swarm-scoped networks.
func (s *Service) ListOverlayNetworks(ctx context.Context, envID string) ([]OverlayNetwork, error) {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	args := make(client.Filters).Add("scope", "swarm")
	networksRes, err := cli.NetworkList(ctx, client.NetworkListOptions{Filters: args})
	if err != nil {
		return nil, fmt.Errorf("network list: %w", err)
	}
	networks := networksRes.Items

	out := make([]OverlayNetwork, 0, len(networks))
	for _, n := range networks {
		out = append(out, mapOverlayNetwork(n))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// composeDocumentName is a small convenience for the handler: a compose file
// may name the stack itself, and a request that omits the name should still
// work when the document carries one.
func composeDocumentName(src string) string {
	var doc struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		return ""
	}
	return strings.TrimSpace(doc.Name)
}
