// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scim

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// maxSCIMResponseBytes caps one page read from a remote SCIM server so a
// misbehaving endpoint cannot exhaust memory.
const maxSCIMResponseBytes = 16 << 20

// remoteSCIMUser decodes `active` as optional: RFC 7643 lets a server omit it,
// and reading an absent flag as false would disable every linked account.
type remoteSCIMUser struct {
	User
	Active *bool `json:"active"`
}

// Puller reads users and groups from a remote SCIM 2.0 server. The scheduled
// poller and the dry-run preview share it so a preview reads exactly what a
// real sync would.
type Puller struct {
	client *http.Client
}

// NewPuller returns a Puller whose requests time out after 30s, because a
// hung identity provider would otherwise hold the sync lock indefinitely.
func NewPuller() *Puller {
	return &Puller{client: &http.Client{Timeout: 30 * time.Second}}
}

// Pull fetches every page of /Users and /Groups from the endpoint.
func (p *Puller) Pull(ctx context.Context, ep *Endpoint, token string) ([]User, []Group, error) {
	users, err := p.pullUsers(ctx, ep, token)
	if err != nil {
		return nil, nil, fmt.Errorf("pull users: %w", err)
	}
	groups, err := p.pullGroups(ctx, ep, token)
	if err != nil {
		return nil, nil, fmt.Errorf("pull groups: %w", err)
	}
	return users, groups, nil
}

func (p *Puller) pullUsers(ctx context.Context, ep *Endpoint, token string) ([]User, error) {
	var all []User
	startIndex := 1
	for {
		var page ListResponse[remoteSCIMUser]
		if err := p.fetchPage(ctx, ep, token, "Users", startIndex, &page); err != nil {
			return nil, err
		}
		for _, remote := range page.Resources {
			user := remote.User
			user.Active = remote.Active == nil || *remote.Active
			all = append(all, user)
		}
		if startIndex+len(page.Resources) > page.TotalResults || len(page.Resources) == 0 {
			break
		}
		startIndex += len(page.Resources)
	}
	return all, nil
}

func (p *Puller) pullGroups(ctx context.Context, ep *Endpoint, token string) ([]Group, error) {
	var all []Group
	startIndex := 1
	for {
		var page ListResponse[Group]
		if err := p.fetchPage(ctx, ep, token, "Groups", startIndex, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Resources...)
		if startIndex+len(page.Resources) > page.TotalResults || len(page.Resources) == 0 {
			break
		}
		startIndex += len(page.Resources)
	}
	return all, nil
}

func (p *Puller) fetchPage(ctx context.Context, ep *Endpoint, token, resource string, startIndex int, into any) error {
	url := strings.TrimRight(ep.BaseURL, "/") + "/" + resource + "?startIndex=" + strconv.Itoa(startIndex) + "&count=100"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	switch ep.AuthType {
	case "basic":
		req.SetBasicAuth(ep.Username, token)
	case "none":
	default:
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/scim+json")
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { closeutil.Report(resp.Body.Close(), "resp.Body") }()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("scim endpoint returned %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSCIMResponseBytes))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, into); err != nil {
		return fmt.Errorf("decode %s: %w", strings.ToLower(resource), err)
	}
	return nil
}
