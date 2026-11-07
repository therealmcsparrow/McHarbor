// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/updatechannel"
	"github.com/therealmcsparrow/mcharbor/core/versionorder"
)

// channelOverridesKey holds every per-plugin override as one JSON list, the
// shape AgentExcludedEnvIDs uses, so a save is one transaction and two saves
// cannot lose each other. The category keeps it beside the global switches.
const (
	channelOverridesKey      = updatechannel.PluginOverridesKey
	channelOverridesCategory = updatechannel.Category
	maxChannelOverrides      = 500
	maxChannelBody           = 4 << 10
)

// slugPattern is the allow-list a plugin slug must match before it reaches a
// settings row or a log line.
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// ErrInvalidSlug is returned when a slug does not match the plugin slug shape.
var ErrInvalidSlug = errors.New("invalid plugin slug")

// ErrTooManyOverrides is returned when the override list is full.
var ErrTooManyOverrides = errors.New("too many plugin channel overrides")

type channelOverride struct {
	Slug        string `json:"slug"`
	Prereleases bool   `json:"prereleases"`
}

// channelState is the global plugins switch plus every per-plugin override.
type channelState struct {
	global    bool
	overrides map[string]bool
}

// prereleases reports the effective setting for slug: on when the global switch
// is on or the plugin's own override is on. An override only adds a plugin to the
// channel, so a stored false never beats the global switch.
func (c channelState) prereleases(slug string) bool {
	return c.global || c.overrides[slug]
}

func decodeOverrides(raw string) ([]channelOverride, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var list []channelOverride
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, fmt.Errorf("decoding plugin channel overrides: %w", err)
	}
	return list, nil
}

// channelState reads the switch and the overrides. A service without a database
// reads as everything off.
func (s *Service) channelState(ctx context.Context) (channelState, error) {
	st := channelState{overrides: map[string]bool{}}
	if s == nil || s.db == nil {
		return st, nil
	}
	global, err := updatechannel.Load(ctx, s.db)
	if err != nil {
		return st, fmt.Errorf("loading the plugin channel switch: %w", err)
	}
	st.global = global.PluginPrereleases
	var raw sql.NullString
	err = s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, channelOverridesKey).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return st, fmt.Errorf("loading plugin channel overrides: %w", err)
	}
	list, err := decodeOverrides(raw.String)
	if err != nil {
		return st, err
	}
	for _, o := range list {
		st.overrides[o.Slug] = o.Prereleases
	}
	return st, nil
}

// channelStateOrOff fails closed: a store that cannot be read offers no
// prerelease, because a stable host must never be moved to one by accident.
func (s *Service) channelStateOrOff(ctx context.Context) channelState {
	st, err := s.channelState(ctx)
	if err != nil {
		slog.Warn("plugin channels: reading the setting failed, offering stable only", "error", err)
		return channelState{overrides: map[string]bool{}}
	}
	return st
}

// SetChannelOverride stores prereleases for slug, or clears the override when it
// is nil, in one transaction. It returns the override before the save.
func (s *Service) SetChannelOverride(ctx context.Context, slug string, prereleases *bool) (old *bool, err error) {
	if !slugPattern.MatchString(slug) {
		return nil, ErrInvalidSlug
	}
	s.overrideMu.Lock()
	defer s.overrideMu.Unlock()
	err = coredb.RetryOnBusy(ctx, 8, func() error {
		tx, txErr := s.db.BeginTx(ctx, nil)
		if txErr != nil {
			return fmt.Errorf("beginning plugin channel save: %w", txErr)
		}
		defer func() { _ = tx.Rollback() }() //nolint:errcheck // rollback after a commit returns ErrTxDone by design
		var raw sql.NullString
		qErr := tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, channelOverridesKey).Scan(&raw)
		if qErr != nil && !errors.Is(qErr, sql.ErrNoRows) {
			return fmt.Errorf("reading plugin channel overrides: %w", qErr)
		}
		list, dErr := decodeOverrides(raw.String)
		if dErr != nil {
			return dErr
		}
		old = nil
		next := make([]channelOverride, 0, len(list)+1)
		for _, o := range list {
			if o.Slug == slug {
				v := o.Prereleases
				old = &v
				continue
			}
			next = append(next, o)
		}
		if prereleases != nil {
			next = append(next, channelOverride{Slug: slug, Prereleases: *prereleases})
		}
		if len(next) > maxChannelOverrides {
			return ErrTooManyOverrides
		}
		encoded, mErr := json.Marshal(next)
		if mErr != nil {
			return fmt.Errorf("encoding plugin channel overrides: %w", mErr)
		}
		now := time.Now().UTC().Format(time.RFC3339)
		if _, eErr := tx.ExecContext(ctx,
			`INSERT INTO settings (id, category, key, value, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			xid.New().String(), channelOverridesCategory, channelOverridesKey, string(encoded), now, now); eErr != nil {
			return fmt.Errorf("saving plugin channel overrides: %w", eErr)
		}
		if cErr := tx.Commit(); cErr != nil {
			return fmt.Errorf("committing plugin channel save: %w", cErr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return old, nil
}

// hostMeets reports whether host satisfies a catalog minimum, given as a
// parameter so the choice is a pure function of its inputs.
func hostMeets(host, minimum string) bool {
	if malformedFloor(minimum) {
		return false
	}
	floor := floorOf(minimum)
	return floor == "" || releaseAtLeast(host, floor)
}

// newestEligible returns the newest version host can take that the channel
// admits, ordered by the shared version order and never by list position.
func newestEligible(versions []PluginVersion, host string, wantPre, anyKind bool) (PluginVersion, bool) {
	var best PluginVersion
	found := false
	for _, v := range versions {
		if !hostMeets(host, v.MinMcharbor) {
			continue
		}
		if !anyKind && versionorder.IsPrerelease(v.Version) != wantPre {
			continue
		}
		if !found || versionorder.Compare(v.Version, best.Version) > 0 {
			best, found = v, true
		}
	}
	return best, found
}

// pickRepair chooses the catalog artifact a repair installs.
//
// A prerelease host takes the prerelease build meeting the floor after a release
// change, else the newest stable. A stable host with the plugin on takes the
// exact version, else after a release change the newest meeting the floor,
// prerelease included. With it off a stable installed plugin is as before; an
// installed prerelease moves only on a release change, to the newest stable, and
// otherwise has no target.
func pickRepair(versions []PluginVersion, installed, host string, optedIn, releaseChanged bool) (PluginVersion, bool) {
	hostPre := versionorder.IsPrerelease(host)
	allow := optedIn || hostPre
	if hostPre && releaseChanged {
		if v, ok := newestEligible(versions, host, true, false); ok {
			return v, true
		}
	}
	if !allow && versionorder.IsPrerelease(installed) {
		if !releaseChanged {
			return PluginVersion{}, false
		}
		return newestEligible(versions, host, false, false)
	}
	for _, v := range versions {
		if v.Version == installed {
			return v, true
		}
	}
	if !releaseChanged {
		return PluginVersion{}, false
	}
	return newestEligible(versions, host, false, allow)
}

// pickLatest chooses what a fresh install or a Store update takes: the newest
// eligible version of the kinds the channel admits, else the newest stable even
// when the host is below its floor, so the installer can say so by name.
func pickLatest(versions []PluginVersion, host string, optedIn bool) (PluginVersion, bool) {
	allow := optedIn || versionorder.IsPrerelease(host)
	if v, ok := newestEligible(versions, host, false, allow); ok {
		return v, true
	}
	return newestEligible(versions, "", false, false)
}

// setMirror points the legacy top-level fields at Versions[0], the newest stable.
func setMirror(e *PluginEntry) {
	if len(e.Versions) == 0 {
		return
	}
	top := e.Versions[0]
	e.Version = top.Version
	e.Build = top.Build
	e.DownloadURL = top.DownloadURL
	e.Checksum = top.Checksum
	e.Signature = top.Signature
	e.MinMcharbor = top.MinMcharbor
}

// filterChannel drops prerelease rows for every plugin the stable host has not
// opted in, drops any row whose version does not parse (versionorder reads one as
// stable, and catalog metadata is unsigned), and hides an entry left with nothing.
func filterChannel(entries []PluginEntry, st channelState, host string) []PluginEntry {
	if versionorder.IsPrerelease(host) {
		return entries
	}
	out := make([]PluginEntry, 0, len(entries))
	for _, e := range entries {
		opted := st.prereleases(e.Slug)
		offered := func(version string) bool {
			return usableVersion(version) && (opted || !versionorder.IsPrerelease(version))
		}
		if len(e.Versions) == 0 {
			if offered(e.Version) {
				out = append(out, e)
			}
			continue
		}
		kept := make([]PluginVersion, 0, len(e.Versions))
		for _, v := range e.Versions {
			if offered(v.Version) {
				kept = append(kept, v)
			}
		}
		if len(kept) == len(e.Versions) {
			out = append(out, e)
			continue
		}
		if len(kept) == 0 {
			continue
		}
		e.Versions = kept
		setMirror(&e)
		out = append(out, e)
	}
	return out
}

// ListCatalog returns the catalog a plugin may be offered from: a prerelease row
// is listed only for a plugin whose effective setting is on, or on a prerelease host.
func (s *Service) ListCatalog(ctx context.Context) ([]PluginEntry, error) {
	all, err := s.listCatalogAll(ctx)
	if err != nil {
		return nil, err
	}
	st := s.channelStateOrOff(ctx)
	out := filterChannel(all, st, hostVersion())
	for i := range out {
		if v, ok := st.overrides[out[i].Slug]; ok {
			out[i].PrereleaseOverride = &v
		}
	}
	return out, nil
}

// catalogSourceExact resolves the artifact of exactly one version, ignoring the
// channel, because a restore asks for the version it recorded.
func (h *Handler) catalogSourceExact(ctx context.Context, slug, version string) (Source, error) {
	entries, err := h.svc.listCatalogAll(ctx)
	if err != nil {
		return Source{}, fmt.Errorf("reading plugin catalog: %w", err)
	}
	for _, entry := range entries {
		if entry.Slug != slug {
			continue
		}
		for _, v := range entry.Versions {
			if v.Version != version {
				continue
			}
			if strings.TrimSpace(v.DownloadURL) == "" {
				return Source{}, fmt.Errorf("catalog entry %s@%s has no download", slug, v.Version)
			}
			return Source{URL: v.DownloadURL, Checksum: v.Checksum, Signature: v.Signature, Build: v.Build, MinMcharbor: v.MinMcharbor}, nil
		}
		return Source{}, fmt.Errorf("catalog has no %s@%s", slug, version)
	}
	return Source{}, fmt.Errorf("%s is not in any configured plugin catalog", slug)
}

// ErrPrereleaseNotEnabled refuses a prerelease build on a stable host whose setting for
// that plugin is off; the operator turns the plugin's override on first.
var ErrPrereleaseNotEnabled = errors.New("this is a prerelease and prerelease versions are off for the plugin; turn on its prerelease override first")

// refuseOffChannel reports ErrPrereleaseNotEnabled when version is a prerelease, the
// host is stable and the plugin's effective setting is off. A prerelease host takes
// prereleases, and an in-process restore, which asks for what a backup recorded,
// does not call it.
func (h *Handler) refuseOffChannel(ctx context.Context, slug, version string) error {
	if versionorder.IsPrerelease(hostVersion()) || !versionorder.IsPrerelease(version) {
		return nil
	}
	if h.svc.channelStateOrOff(ctx).prereleases(slug) {
		return nil
	}
	return ErrPrereleaseNotEnabled
}

// ChannelRequest documents the body of PUT /appstore/plugins/{slug}/channel for
// the OpenAPI spec: the key is required and null clears the override.
type ChannelRequest struct {
	Prereleases *bool `json:"prereleases" openapi:"nullable"`
}

// channelBody is what the handler decodes, because a *bool cannot tell an absent
// key from null and the route refuses a body without the key.
type channelBody struct {
	Prereleases json.RawMessage `json:"prereleases"`
}

// ChannelResult is the override after the save; Prereleases is null when cleared.
type ChannelResult struct {
	Slug        string `json:"slug"`
	Prereleases *bool  `json:"prereleases"`
}

func parseOverride(raw json.RawMessage) (*bool, bool) {
	switch strings.TrimSpace(string(raw)) {
	case "true":
		v := true
		return &v, true
	case "false":
		v := false
		return &v, true
	case "null":
		return nil, true
	}
	return nil, false
}

func overrideText(v *bool) string {
	if v == nil {
		return "null"
	}
	if *v {
		return "true"
	}
	return "false"
}

// handleSetChannel serves PUT /api/appstore/plugins/{slug}/channel: set or clear
// one plugin's prerelease override. The audit row carries the old and new value.
func (h *Handler) handleSetChannel(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	slug := chi.URLParam(r, "slug")
	if !slugPattern.MatchString(slug) {
		response.BadRequestCode(w, r, i18n.ErrPluginSlugRequired)
		return
	}
	var req channelBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxChannelBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	next, ok := parseOverride(req.Prereleases)
	if !ok {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	old, err := h.svc.SetChannelOverride(r.Context(), slug, next)
	if err != nil {
		if errors.Is(err, ErrTooManyOverrides) {
			response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
			return
		}
		slog.Error("plugin channel: save failed", "slug", slug, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if overrideText(old) != overrideText(next) && h.audit != nil {
		h.audit.Log(r, audit.Entry{
			Action:      "plugin_channel_change",
			EntityType:  "plugin",
			EntityID:    slug,
			EntityName:  slug,
			Details:     fmt.Sprintf("Plugin %s prerelease override %s -> %s by %s", slug, overrideText(old), overrideText(next), user.Username),
			StateBefore: `{"prereleases":` + overrideText(old) + `}`,
			StateAfter:  `{"prereleases":` + overrideText(next) + `}`,
			ChangedKeys: "prereleases",
		})
	}
	response.OK(w, ChannelResult{Slug: slug, Prereleases: next})
}
