// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/moby/moby/client"
)

// ListTasks returns tasks, optionally narrowed to one service or node.
//
// Service and node names are resolved from one ServiceList plus one NodeList,
// not from an inspect per task. A cluster with 200 tasks would otherwise cost
// 200 round trips, and over the agent transport each one is measured in
// hundreds of milliseconds.
func (s *Service) ListTasks(ctx context.Context, envID string, f TaskFilter) ([]Task, error) {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	args := make(client.Filters)
	if f.ServiceID != "" {
		args.Add("service", f.ServiceID)
	}
	if f.NodeID != "" {
		args.Add("node", f.NodeID)
	}
	if f.DesiredState != "" {
		args.Add("desired-state", f.DesiredState)
	}

	tasksRes, err := cli.TaskList(ctx, client.TaskListOptions{Filters: args})
	if err != nil {
		return nil, fmt.Errorf("task list: %w", err)
	}
	tasks := tasksRes.Items

	serviceNames := map[string]string{}
	if services, sErr := cli.ServiceList(ctx, client.ServiceListOptions{}); sErr == nil {
		for _, svc := range services.Items {
			serviceNames[svc.ID] = svc.Spec.Name
		}
	}
	nodeNames := map[string]string{}
	if nodes, nErr := cli.NodeList(ctx, client.NodeListOptions{}); nErr == nil {
		for _, n := range nodes.Items {
			name := n.Description.Hostname
			if name == "" {
				name = n.ID
			}
			nodeNames[n.ID] = name
		}
	}

	out := make([]Task, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, mapTask(t, serviceNames[t.ServiceID], nodeNames[t.NodeID]))
	}

	// Newest first: an operator opening the tasks page is looking for what
	// just happened, not for the task that has been up for three weeks.
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].Name < out[j].Name
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

// InspectTask returns one task.
func (s *Service) InspectTask(ctx context.Context, envID, id string) (Task, error) {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return Task{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	tRes, err := cli.TaskInspect(ctx, id, client.TaskInspectOptions{})
	if err != nil {
		return Task{}, fmt.Errorf("task inspect %s: %w", id, err)
	}
	t := tRes.Task

	serviceName := ""
	if t.ServiceID != "" {
		if svc, sErr := cli.ServiceInspect(ctx, t.ServiceID, client.ServiceInspectOptions{}); sErr == nil {
			serviceName = svc.Service.Spec.Name
		}
	}
	nodeName := ""
	if t.NodeID != "" {
		if n, nErr := cli.NodeInspect(ctx, t.NodeID, client.NodeInspectOptions{}); nErr == nil {
			nodeName = n.Node.Description.Hostname
		}
	}

	return mapTask(t, serviceName, nodeName), nil
}
