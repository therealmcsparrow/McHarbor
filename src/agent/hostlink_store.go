// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/netconf"
)

// hostLinkDirName is the folder under the agent's data dir that holds one
// desired-state file per host link, replayed at startup because a macvlan
// child is kernel state and does not survive a reboot.
const hostLinkDirName = "hostlinks"

// defaultAgentDataDir is used when MCHARBOR_DATA_DIR is set to empty.
const defaultAgentDataDir = "/var/lib/mcharbor-agent"

// hostLinkOwnerIDPattern bounds an owner id to something that can never climb
// out of the store directory or split the ifalias it is written into.
var hostLinkOwnerIDPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

// hostLinkApplyMu serialises every host link mutation: a replay retry, a
// host_link_request and a compute network ensure may all name the same link.
var hostLinkApplyMu sync.Mutex

// hostLinkRecord is the desired state of one host link as last applied. The
// addresses double as the stale set of the next ensure, which is how a changed
// address is removed without touching any the operator set by hand.
type hostLinkRecord struct {
	Owner   string `json:"owner"`
	OwnerID string `json:"ownerId"`
	Name    string `json:"name"`
	// Parent is the interface as the request named it; for a compute network
	// it is the uplink, and VLANTag says which sub-interface to resolve.
	Parent      string    `json:"parent"`
	VLANTag     int       `json:"vlanTag,omitempty"`
	IPv4CIDR    string    `json:"ipv4Cidr,omitempty"`
	IPv6Enabled bool      `json:"ipv6Enabled,omitempty"`
	IPv6CIDR    string    `json:"ipv6Cidr,omitempty"`
	SavedAt     time.Time `json:"savedAt"`
}

// staleCIDRs lists the addresses this record applied.
func (r *hostLinkRecord) staleCIDRs() []string {
	if r == nil {
		return nil
	}
	var out []string
	for _, cidr := range []string{r.IPv4CIDR, r.IPv6CIDR} {
		if cidr != "" {
			out = append(out, cidr)
		}
	}
	return out
}

// hostLinkStore keeps hostLinkRecords as one JSON file each.
type hostLinkStore struct {
	dir string
	mu  sync.Mutex
}

// newHostLinkStore returns a store rooted at dataDir/hostlinks.
func newHostLinkStore(dataDir string) *hostLinkStore {
	if strings.TrimSpace(dataDir) == "" {
		dataDir = defaultAgentDataDir
	}
	return &hostLinkStore{dir: filepath.Join(dataDir, hostLinkDirName)}
}

// path names the record file for owner/ownerID, refusing anything that is not
// a known owner or a plain id.
func (s *hostLinkStore) path(owner, ownerID string) (string, error) {
	switch owner {
	case HostLinkOwnerCompute, HostLinkOwnerDocker:
	default:
		return "", fmt.Errorf("%w: unknown owner %q", netconf.ErrHostLinkInvalid, owner)
	}
	if !hostLinkOwnerIDPattern.MatchString(ownerID) || strings.Trim(ownerID, ".") == "" {
		return "", fmt.Errorf("%w: owner id %q", netconf.ErrHostLinkInvalid, ownerID)
	}
	return filepath.Join(s.dir, owner+"-"+ownerID+".json"), nil
}

// save writes rec through a temporary file, so a crash mid-write leaves the
// previous record rather than a truncated one the replay cannot read.
func (s *hostLinkStore) save(rec hostLinkRecord) error {
	path, err := s.path(rec.Owner, rec.OwnerID)
	if err != nil {
		return err
	}
	if rec.SavedAt.IsZero() {
		rec.SavedAt = time.Now().UTC()
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("host links: encode %s: %w", path, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("host links: create %s: %w", s.dir, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("host links: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return errors.Join(fmt.Errorf("host links: replace %s: %w", path, err), os.Remove(tmp))
	}
	return nil
}

// load returns owner/ownerID's record, or nil when there is none.
func (s *hostLinkStore) load(owner, ownerID string) (*hostLinkRecord, error) {
	path, err := s.path(owner, ownerID)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("host links: read %s: %w", path, err)
	}
	var rec hostLinkRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("host links: decode %s: %w", path, err)
	}
	return &rec, nil
}

// forget removes owner/ownerID's record; a missing one is already forgotten.
func (s *hostLinkStore) forget(owner, ownerID string) error {
	path, err := s.path(owner, ownerID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("host links: remove %s: %w", path, err)
	}
	return nil
}

// list returns every readable record, sorted for a stable replay order. A file
// that does not decode is reported in the error and skipped, not fatal.
func (s *hostLinkStore) list() ([]hostLinkRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("host links: list %s: %w", s.dir, err)
	}
	var (
		out  []hostLinkRecord
		errs []error
	)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(s.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("host links: read %s: %w", path, err))
			continue
		}
		var rec hostLinkRecord
		if err := json.Unmarshal(data, &rec); err != nil || rec.Owner == "" || rec.OwnerID == "" {
			errs = append(errs, fmt.Errorf("host links: %s is not a host link record", path))
			continue
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Owner != out[j].Owner {
			return out[i].Owner < out[j].Owner
		}
		return out[i].OwnerID < out[j].OwnerID
	})
	return out, errors.Join(errs...)
}

// hostLinkErrorCode classifies a host link failure for HostLinkPayload.ErrorCode.
// Empty means a failure with no code of its own; Error still carries the text.
func hostLinkErrorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, netconf.ErrHostLinkNameTaken):
		return HostLinkErrNameTaken
	case errors.Is(err, netconf.ErrParentNotFound):
		return HostLinkErrParentNotFound
	case errors.Is(err, netconf.ErrHostLinkInvalid):
		return HostLinkErrInvalid
	case errors.Is(err, netconf.ErrNotLinux):
		return HostLinkErrNotLinux
	case errors.Is(err, os.ErrPermission):
		return HostLinkErrPermissionDenied
	}
	return ""
}

// hostLinkRetryDelay is the wait before replay attempt n (from 0) of a link
// whose parent is not there yet: 5s doubling to a 60s ceiling.
func hostLinkRetryDelay(attempt int) time.Duration {
	delay := hostLinkRetryFirst
	for i := 0; i < attempt && delay < hostLinkRetryMax; i++ {
		delay *= 2
	}
	return min(delay, hostLinkRetryMax)
}

// Replay retry bounds. A parent that is still missing after the window is a
// host configuration problem, not a slow boot, and retrying forever hides it.
const (
	hostLinkRetryFirst  = 5 * time.Second
	hostLinkRetryMax    = 60 * time.Second
	hostLinkRetryWindow = 10 * time.Minute
)

// hostLinkReplayer restores persisted host links at startup. Its parts are
// fields so the retry schedule can be exercised without a kernel or a clock.
type hostLinkReplayer struct {
	store  *hostLinkStore
	logger *slog.Logger
	apply  func(hostLinkRecord) error
	delay  func(attempt int) time.Duration
	window time.Duration
}

// run applies records, retrying the ones whose parent is not there yet: at boot
// the agent can start before the NIC, its VLAN or Docker's own sub-interface
// exists. Any other failure is logged once and not retried.
func (r hostLinkReplayer) run(ctx context.Context, records []hostLinkRecord) {
	deadline := time.Now().Add(r.window)
	pending := records
	for attempt := 0; len(pending) > 0; attempt++ {
		var retry []hostLinkRecord
		for _, rec := range pending {
			hostLinkApplyMu.Lock()
			// A request may have removed or replaced the record since startup;
			// the store, not the startup snapshot, is the truth.
			current, err := r.store.load(rec.Owner, rec.OwnerID)
			if err != nil || current == nil {
				hostLinkApplyMu.Unlock()
				continue
			}
			err = r.apply(*current)
			hostLinkApplyMu.Unlock()
			switch {
			case err == nil:
				r.logger.Info("host links: restored", "owner", current.Owner, "ownerId", current.OwnerID, "name", current.Name)
			case errors.Is(err, netconf.ErrParentNotFound):
				retry = append(retry, *current)
			default:
				r.logger.Error("host links: restore failed", "owner", current.Owner, "ownerId", current.OwnerID, "name", current.Name, "error", err)
			}
		}
		if len(retry) == 0 {
			return
		}
		wait := r.delay(attempt)
		if time.Now().Add(wait).After(deadline) {
			for _, rec := range retry {
				r.logger.Error("host links: parent never appeared, giving up", "owner", rec.Owner, "ownerId", rec.OwnerID, "parent", rec.Parent)
			}
			return
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		pending = retry
	}
}
