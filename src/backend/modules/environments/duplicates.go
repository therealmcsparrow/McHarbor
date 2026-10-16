// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package environments

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/moby/moby/client"
)

// duplicateProbeTimeout bounds one environment's identity probe.
//
// The scan is fanned out, so this is the wall-clock cost of the whole sweep
// rather than of each host, and an unreachable environment must not hold the
// dialog open.
const duplicateProbeTimeout = 10 * time.Second

// DuplicateMember is one environment in a duplicate group.
type DuplicateMember struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	ConnectionType string `json:"connectionType"`
	IsDefault      bool   `json:"isDefault"`
}

// DuplicateGroup is a set of environments that are the same machine.
//
// Reason names the evidence — the operator is being asked to merge two rows and
// delete one, so "because they report the same Docker daemon id" is the
// difference between a decision and a leap of faith.
type DuplicateGroup struct {
	Reason   string            `json:"reason"`
	Hostname string            `json:"hostname,omitempty"`
	Members  []DuplicateMember `json:"members"`
}

// Duplicates finds environments that point at the same machine.
//
// The Docker daemon's own id is the evidence, because it is the one signal that
// survives the same host being reached over a socket in one environment and an
// agent in another — which is exactly the pair this exists to surface.
func (s *Service) Duplicates(ctx context.Context) ([]DuplicateGroup, error) {
	envs, err := s.ListContext(ctx)
	if err != nil {
		return nil, err
	}

	type identity struct {
		daemonID string
		hostname string
	}

	identities := make([]identity, len(envs))
	var wg sync.WaitGroup
	for i := range envs {
		wg.Add(1)
		go func(index int, envID string) {
			defer wg.Done()
			probeCtx, cancel := context.WithTimeout(ctx, duplicateProbeTimeout)
			defer cancel()

			cli, clientErr := s.dockerPool.GetContext(ctx, envID)
			if clientErr != nil {
				return
			}
			infoRes, infoErr := cli.Info(probeCtx, client.InfoOptions{})
			if infoErr != nil {
				return
			}
			info := infoRes.Info
			identities[index] = identity{daemonID: info.ID, hostname: info.Name}
		}(i, envs[i].ID)
	}
	wg.Wait()

	grouped := map[string][]DuplicateMember{}
	hostnames := map[string]string{}
	for i, env := range envs {
		id := identities[i].daemonID
		if id == "" {
			continue
		}
		grouped[id] = append(grouped[id], DuplicateMember{
			ID:             env.ID,
			Name:           env.Name,
			ConnectionType: env.ConnectionType,
			IsDefault:      env.IsDefault,
		})
		if hostnames[id] == "" {
			hostnames[id] = identities[i].hostname
		}
	}

	groups := make([]DuplicateGroup, 0, len(grouped))
	for id, members := range grouped {
		if len(members) < 2 {
			continue
		}
		sort.Slice(members, func(a, b int) bool { return members[a].Name < members[b].Name })
		groups = append(groups, DuplicateGroup{
			Reason:   "same_daemon_id",
			Hostname: hostnames[id],
			Members:  members,
		})
	}
	sort.Slice(groups, func(a, b int) bool { return groups[a].Hostname < groups[b].Hostname })
	return groups, nil
}
