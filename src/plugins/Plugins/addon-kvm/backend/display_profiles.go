// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/xid"
)

// Display profiles: named sets of SPICE server options a guest launches with.
//
// The browser console is spice-html5, a JavaScript decoder. It draws raw
// bitmaps, LZ, QUIC and JPEG, streams MJPEG, and plays only Opus audio; it has
// no GLZ decoder at all. Which options a guest starts with therefore decides
// whether the browser spends its time decoding or simply painting.

// The built-in profiles. Their ids are fixed so a guest's choice survives a
// reinstall, and their options are re-asserted from these values at every Init.
const (
	ProfileFastLAN      = "builtin-fast-lan"
	ProfileBalanced     = "builtin-balanced"
	ProfileLowBandwidth = "builtin-low-bandwidth"
)

// ErrDisplayProfileBuiltin refuses an edit or delete of a built-in profile.
var ErrDisplayProfileBuiltin = errors.New("built-in display profiles cannot be changed")

// ErrDisplayProfileNotFound is a profile id that names nothing.
var ErrDisplayProfileNotFound = errors.New("display profile not found")

// ErrDisplayProfileNameTaken is a second profile with the same name.
var ErrDisplayProfileNameTaken = errors.New("a display profile with that name exists")

// ErrDisplayProfileInUse wraps the refusal to delete a profile guests still use.
var ErrDisplayProfileInUse = errors.New("display profile in use")

// DisplayProfileInUseError carries how many guests still use the profile, which
// is what the operator needs to decide whether to reassign them.
type DisplayProfileInUseError struct {
	Count int
}

// Error names the count so the log line is useful on its own.
func (e *DisplayProfileInUseError) Error() string {
	return fmt.Sprintf("display profile in use by %d virtual machine(s)", e.Count)
}

// Unwrap lets errors.Is match the sentinel.
func (e *DisplayProfileInUseError) Unwrap() error { return ErrDisplayProfileInUse }

// DisplayProfile is one named set of SPICE options.
type DisplayProfile struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Builtin     bool   `json:"builtin"`

	ImageCompression      string `json:"imageCompression"`
	JPEGWANCompression    string `json:"jpegWanCompression"`
	ZlibGLZWANCompression string `json:"zlibGlzWanCompression"`
	StreamingVideo        string `json:"streamingVideo"`
	PlaybackCompression   string `json:"playbackCompression"`
	AgentMouse            string `json:"agentMouse"`

	// VMCount is how many guests and templates launch with this profile,
	// counting a guest with no choice under Fast LAN.
	VMCount int `json:"vmCount"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// DisplayProfileRequest creates or edits a profile. Every field is optional on
// an edit; on a create an unset option takes Fast LAN's value.
type DisplayProfileRequest struct {
	Name                  *string `json:"name,omitempty"`
	Description           *string `json:"description,omitempty"`
	ImageCompression      *string `json:"imageCompression,omitempty"`
	JPEGWANCompression    *string `json:"jpegWanCompression,omitempty"`
	ZlibGLZWANCompression *string `json:"zlibGlzWanCompression,omitempty"`
	StreamingVideo        *string `json:"streamingVideo,omitempty"`
	PlaybackCompression   *string `json:"playbackCompression,omitempty"`
	AgentMouse            *string `json:"agentMouse,omitempty"`
}

// agentSpiceOptions is VMSpiceOptions on the wire; the agent drops any value
// outside QEMU's closed set, so a bad row cannot stop a guest from booting.
type agentSpiceOptions struct {
	ImageCompression      string `json:"imageCompression,omitempty"`
	JPEGWANCompression    string `json:"jpegWanCompression,omitempty"`
	ZlibGLZWANCompression string `json:"zlibGlzWanCompression,omitempty"`
	StreamingVideo        string `json:"streamingVideo,omitempty"`
	PlaybackCompression   string `json:"playbackCompression,omitempty"`
	AgentMouse            string `json:"agentMouse,omitempty"`
}

// spiceOptionSets are the values QEMU's -spice accepts for each option.
// video-codec is deliberately absent: it needs a GStreamer-enabled
// spice-server that no probe confirms, and a wrong value stops QEMU starting.
var spiceOptionSets = map[string]map[string]bool{
	"imageCompression":      {"auto_glz": true, "auto_lz": true, "quic": true, "glz": true, "lz": true, "off": true},
	"jpegWanCompression":    {"auto": true, "never": true, "always": true},
	"zlibGlzWanCompression": {"auto": true, "never": true, "always": true},
	"streamingVideo":        {"off": true, "all": true, "filter": true},
	"playbackCompression":   {"on": true, "off": true},
	"agentMouse":            {"on": true, "off": true},
}

// builtinProfiles are the three presets, tuned for spice-html5. Playback
// compression stays on in all three because the browser plays only Opus: off
// sends raw PCM, which spice-html5 drops. None uses GLZ, which it cannot draw.
var builtinProfiles = []DisplayProfile{
	{
		ID: ProfileFastLAN, Name: "Fast LAN", Builtin: true,
		Description:      "Uncompressed images: the most bandwidth, the least work for the browser.",
		ImageCompression: "off", JPEGWANCompression: "never", ZlibGLZWANCompression: "never",
		StreamingVideo: "filter", PlaybackCompression: "on", AgentMouse: "on",
	},
	{
		ID: ProfileBalanced, Name: "Balanced", Builtin: true,
		Description:      "LZ for desktop content, JPEG on a slow link.",
		ImageCompression: "auto_lz", JPEGWANCompression: "auto", ZlibGLZWANCompression: "never",
		StreamingVideo: "filter", PlaybackCompression: "on", AgentMouse: "on",
	},
	{
		ID: ProfileLowBandwidth, Name: "Low bandwidth", Builtin: true,
		Description:      "Lossy JPEG and MJPEG streaming everywhere: the fewest bytes.",
		ImageCompression: "auto_lz", JPEGWANCompression: "always", ZlibGLZWANCompression: "never",
		StreamingVideo: "all", PlaybackCompression: "on", AgentMouse: "on",
	},
}

// builtinProfile returns the preset with id, if it is one.
func builtinProfile(id string) (DisplayProfile, bool) {
	for _, profile := range builtinProfiles {
		if profile.ID == id {
			return profile, true
		}
	}
	return DisplayProfile{}, false
}

// effectiveProfileID maps a guest's stored choice to the profile it launches
// with: no choice means Fast LAN.
func effectiveProfileID(stored string) string {
	if stored == "" {
		return ProfileFastLAN
	}
	return stored
}

// storedProfileID is what the column holds for a choice. Fast LAN is stored as
// NULL, so the default can move without rewriting every guest that never chose.
func storedProfileID(id string) any {
	if id == "" || id == ProfileFastLAN {
		return nil
	}
	return id
}

const displayProfileColumns = `
	p.id, p.name, COALESCE(p.description, ''), p.builtin,
	p.image_compression, p.jpeg_wan_compression, p.zlib_glz_wan_compression,
	p.streaming_video, p.playback_compression, p.agent_mouse,
	p.created_at, p.updated_at,
	(SELECT COUNT(*) FROM plugin_addon_kvm_vms v
	  WHERE COALESCE(v.display_profile_id, 'builtin-fast-lan') = p.id)`

// ensureDisplayProfileSchema creates the profile table, adds the guest's
// column, and re-asserts the built-ins.
//
// Every statement is idempotent. The column is added here rather than in
// 0009: an ALTER there would fail a repair whenever Init had already run.
func (s *Service) ensureDisplayProfileSchema(ctx context.Context) error {
	if _, err := s.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS plugin_addon_kvm_display_profiles (
		    id                       TEXT PRIMARY KEY,
		    name                     TEXT NOT NULL UNIQUE,
		    description              TEXT,
		    builtin                  INTEGER NOT NULL DEFAULT 0,
		    image_compression        TEXT NOT NULL,
		    jpeg_wan_compression     TEXT NOT NULL,
		    zlib_glz_wan_compression TEXT NOT NULL,
		    streaming_video          TEXT NOT NULL,
		    playback_compression     TEXT NOT NULL,
		    agent_mouse              TEXT NOT NULL,
		    created_at               TEXT NOT NULL,
		    updated_at               TEXT NOT NULL
		)`); err != nil {
		return fmt.Errorf("preparing the display profile table: %w", err)
	}
	if _, err := s.db.Exec(ctx,
		`ALTER TABLE plugin_addon_kvm_vms ADD COLUMN display_profile_id TEXT`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		return fmt.Errorf("adding the display profile column: %w", err)
	}
	// The boot resolution columns ride along for the same reason: an ALTER in a
	// migration file fails a repair once Init has added them.
	for _, column := range []string{"display_width", "display_height"} {
		stmt := `ALTER TABLE plugin_addon_kvm_vms ADD COLUMN ` + column + ` INTEGER NOT NULL DEFAULT 0` // SQL identifier: column comes from the literal list above
		if _, err := s.db.Exec(ctx, stmt); err != nil &&
			!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return fmt.Errorf("adding the %s column: %w", column, err)
		}
	}
	// The seed timestamps differ by a second each so the list keeps this order.
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i, profile := range builtinProfiles {
		stamp := base.Add(time.Duration(i) * time.Second).Format(time.RFC3339)
		if _, err := s.db.Exec(ctx, `
			INSERT INTO plugin_addon_kvm_display_profiles
				(id, name, description, builtin, image_compression, jpeg_wan_compression,
				 zlib_glz_wan_compression, streaming_video, playback_compression, agent_mouse,
				 created_at, updated_at)
			VALUES (?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				builtin = 1,
				image_compression = excluded.image_compression,
				jpeg_wan_compression = excluded.jpeg_wan_compression,
				zlib_glz_wan_compression = excluded.zlib_glz_wan_compression,
				streaming_video = excluded.streaming_video,
				playback_compression = excluded.playback_compression,
				agent_mouse = excluded.agent_mouse`,
			profile.ID, profile.Name, profile.Description,
			profile.ImageCompression, profile.JPEGWANCompression, profile.ZlibGLZWANCompression,
			profile.StreamingVideo, profile.PlaybackCompression, profile.AgentMouse,
			stamp, stamp); err != nil {
			return fmt.Errorf("seeding the %s display profile: %w", profile.ID, err)
		}
	}
	return nil
}

// scanDisplayProfile reads one row of displayProfileColumns.
func scanDisplayProfile(rows interface{ Scan(...any) error }) (DisplayProfile, error) {
	var profile DisplayProfile
	var builtin int
	var created, updated string
	if err := rows.Scan(&profile.ID, &profile.Name, &profile.Description, &builtin,
		&profile.ImageCompression, &profile.JPEGWANCompression, &profile.ZlibGLZWANCompression,
		&profile.StreamingVideo, &profile.PlaybackCompression, &profile.AgentMouse,
		&created, &updated, &profile.VMCount); err != nil {
		return DisplayProfile{}, err
	}
	profile.Builtin = builtin == 1
	profile.CreatedAt = parseTime(created)
	profile.UpdatedAt = parseTime(updated)
	return profile, nil
}

// ListDisplayProfiles returns every profile, built-ins first.
func (s *Service) ListDisplayProfiles(ctx context.Context) ([]DisplayProfile, error) {
	rows, err := s.db.Query(ctx, `SELECT `+displayProfileColumns+`
		  FROM plugin_addon_kvm_display_profiles p
		 ORDER BY p.builtin DESC, p.created_at, p.name
		 LIMIT ?`, maxRows)
	if err != nil {
		return nil, fmt.Errorf("listing display profiles: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []DisplayProfile{}
	for rows.Next() {
		profile, err := scanDisplayProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, profile)
	}
	return out, rows.Err()
}

// DisplayProfile reads one profile.
func (s *Service) DisplayProfile(ctx context.Context, id string) (DisplayProfile, error) {
	rows, err := s.db.Query(ctx, `SELECT `+displayProfileColumns+`
		  FROM plugin_addon_kvm_display_profiles p WHERE p.id = ?`, id)
	if err != nil {
		return DisplayProfile{}, fmt.Errorf("reading display profile: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return DisplayProfile{}, err
		}
		return DisplayProfile{}, ErrDisplayProfileNotFound
	}
	return scanDisplayProfile(rows)
}

// applyDisplayProfileRequest merges req onto profile and validates the result.
func applyDisplayProfileRequest(profile *DisplayProfile, req DisplayProfileRequest) error {
	if req.Name != nil {
		profile.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		profile.Description = strings.TrimSpace(*req.Description)
	}
	fields := []struct {
		key    string
		value  *string
		target *string
	}{
		{"imageCompression", req.ImageCompression, &profile.ImageCompression},
		{"jpegWanCompression", req.JPEGWANCompression, &profile.JPEGWANCompression},
		{"zlibGlzWanCompression", req.ZlibGLZWANCompression, &profile.ZlibGLZWANCompression},
		{"streamingVideo", req.StreamingVideo, &profile.StreamingVideo},
		{"playbackCompression", req.PlaybackCompression, &profile.PlaybackCompression},
		{"agentMouse", req.AgentMouse, &profile.AgentMouse},
	}
	for _, field := range fields {
		if field.value != nil {
			*field.target = *field.value
		}
		if !spiceOptionSets[field.key][*field.target] {
			return invalid(codeDisplayProfileOption, fmt.Sprintf("%q is not a value of %s", *field.target, field.key))
		}
	}
	return validateDisplayProfileName(profile.Name, profile.Description)
}

// validateDisplayProfileName bounds the two free-text fields.
func validateDisplayProfileName(name, description string) error {
	if name == "" {
		return invalid(codeDisplayProfileNameRequired, "a display profile needs a name")
	}
	if len([]rune(name)) > 80 {
		return invalid(codeDisplayProfileNameTooLong, "that display profile name is too long")
	}
	if len([]rune(description)) > 500 {
		return invalid(codeDisplayProfileNameTooLong, "that description is too long")
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return invalid(codeDisplayProfileNameRequired, "a name cannot contain control characters")
		}
	}
	return nil
}

// CreateDisplayProfile adds a custom profile; unset options take Fast LAN's.
func (s *Service) CreateDisplayProfile(ctx context.Context, req DisplayProfileRequest) (DisplayProfile, error) {
	base, _ := builtinProfile(ProfileFastLAN)
	profile := DisplayProfile{
		ID:                    xid.New().String(),
		ImageCompression:      base.ImageCompression,
		JPEGWANCompression:    base.JPEGWANCompression,
		ZlibGLZWANCompression: base.ZlibGLZWANCompression,
		StreamingVideo:        base.StreamingVideo,
		PlaybackCompression:   base.PlaybackCompression,
		AgentMouse:            base.AgentMouse,
	}
	if err := applyDisplayProfileRequest(&profile, req); err != nil {
		return DisplayProfile{}, err
	}
	return s.insertDisplayProfile(ctx, profile)
}

// insertDisplayProfile writes a new custom row.
func (s *Service) insertDisplayProfile(ctx context.Context, profile DisplayProfile) (DisplayProfile, error) {
	now := time.Now().UTC()
	profile.Builtin = false
	profile.CreatedAt, profile.UpdatedAt = now, now
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_kvm_display_profiles
			(id, name, description, builtin, image_compression, jpeg_wan_compression,
			 zlib_glz_wan_compression, streaming_video, playback_compression, agent_mouse,
			 created_at, updated_at)
		VALUES (?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?)`,
		profile.ID, profile.Name, nullIfEmpty(profile.Description),
		profile.ImageCompression, profile.JPEGWANCompression, profile.ZlibGLZWANCompression,
		profile.StreamingVideo, profile.PlaybackCompression, profile.AgentMouse,
		now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		if isUniqueViolation(err) {
			return DisplayProfile{}, ErrDisplayProfileNameTaken
		}
		return DisplayProfile{}, fmt.Errorf("recording display profile: %w", err)
	}
	return profile, nil
}

// UpdateDisplayProfile edits a custom profile. A guest picks the change up at
// its next start; a running one keeps the options it booted with.
func (s *Service) UpdateDisplayProfile(ctx context.Context, id string, req DisplayProfileRequest) (DisplayProfile, error) {
	profile, err := s.DisplayProfile(ctx, id)
	if err != nil {
		return DisplayProfile{}, err
	}
	if profile.Builtin {
		return DisplayProfile{}, ErrDisplayProfileBuiltin
	}
	if err := applyDisplayProfileRequest(&profile, req); err != nil {
		return DisplayProfile{}, err
	}
	profile.UpdatedAt = time.Now().UTC()
	// builtin = 0 in the WHERE as well: the check above and this write are two
	// statements, and a built-in must never be the row that changes.
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_display_profiles
		   SET name = ?, description = ?, image_compression = ?, jpeg_wan_compression = ?,
		       zlib_glz_wan_compression = ?, streaming_video = ?, playback_compression = ?,
		       agent_mouse = ?, updated_at = ?
		 WHERE id = ? AND builtin = 0`,
		profile.Name, nullIfEmpty(profile.Description), profile.ImageCompression,
		profile.JPEGWANCompression, profile.ZlibGLZWANCompression, profile.StreamingVideo,
		profile.PlaybackCompression, profile.AgentMouse,
		profile.UpdatedAt.Format(time.RFC3339), id); err != nil {
		if isUniqueViolation(err) {
			return DisplayProfile{}, ErrDisplayProfileNameTaken
		}
		return DisplayProfile{}, fmt.Errorf("updating display profile: %w", err)
	}
	return profile, nil
}

// DuplicateDisplayProfile copies any profile, built-in or not, into a new
// custom one named "<name> (copy)", numbering the copy when that is taken.
func (s *Service) DuplicateDisplayProfile(ctx context.Context, id string) (DisplayProfile, error) {
	source, err := s.DisplayProfile(ctx, id)
	if err != nil {
		return DisplayProfile{}, err
	}
	copied := source
	copied.ID = xid.New().String()
	copied.VMCount = 0
	for attempt := 1; attempt <= 50; attempt++ { // bounded by the attempt cap
		copied.Name = duplicateName(source.Name, attempt)
		created, err := s.insertDisplayProfile(ctx, copied)
		if errors.Is(err, ErrDisplayProfileNameTaken) {
			continue
		}
		return created, err
	}
	return DisplayProfile{}, ErrDisplayProfileNameTaken
}

// duplicateName is "X (copy)", then "X (copy 2)", "X (copy 3)"…
func duplicateName(name string, attempt int) string {
	if attempt <= 1 {
		return name + " (copy)"
	}
	return fmt.Sprintf("%s (copy %d)", name, attempt)
}

// DeleteDisplayProfile removes a custom profile. While guests use it the
// delete is refused with their count, unless reassign moves them to Fast LAN
// first; it returns how many guests were moved.
func (s *Service) DeleteDisplayProfile(ctx context.Context, id string, reassign bool) (int, error) {
	profile, err := s.DisplayProfile(ctx, id)
	if err != nil {
		return 0, err
	}
	if profile.Builtin {
		return 0, ErrDisplayProfileBuiltin
	}
	moved := 0
	if reassign {
		affected, err := s.db.Exec(ctx, `
			UPDATE plugin_addon_kvm_vms SET display_profile_id = NULL, updated_at = ?
			 WHERE display_profile_id = ?`, time.Now().UTC().Format(time.RFC3339), id)
		if err != nil {
			return 0, fmt.Errorf("reassigning guests to Fast LAN: %w", err)
		}
		moved = int(affected)
	}
	// The in-use check is part of the delete itself, so a guest that picks the
	// profile between the count and this statement still blocks it.
	deleted, err := s.db.Exec(ctx, `
		DELETE FROM plugin_addon_kvm_display_profiles
		 WHERE id = ? AND builtin = 0
		   AND NOT EXISTS (SELECT 1 FROM plugin_addon_kvm_vms WHERE display_profile_id = ?)`, id, id)
	if err != nil {
		return moved, fmt.Errorf("deleting display profile: %w", err)
	}
	if deleted == 0 {
		count, err := s.displayProfileUsers(ctx, id)
		if err != nil {
			return moved, err
		}
		return moved, &DisplayProfileInUseError{Count: count}
	}
	return moved, nil
}

// displayProfileUsers counts the guests whose stored choice is id.
func (s *Service) displayProfileUsers(ctx context.Context, id string) (int, error) {
	rows, err := s.db.Query(ctx,
		`SELECT COUNT(*) FROM plugin_addon_kvm_vms WHERE display_profile_id = ?`, id)
	if err != nil {
		return 0, fmt.Errorf("counting display profile users: %w", err)
	}
	defer func() { _ = rows.Close() }()
	count := 0
	if rows.Next() {
		if err := rows.Scan(&count); err != nil {
			return 0, err
		}
	}
	return count, rows.Err()
}

// validateDisplayProfileChoice checks a guest's new choice names a profile.
// Empty is allowed and means Fast LAN.
func (s *Service) validateDisplayProfileChoice(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	if _, err := s.DisplayProfile(ctx, id); err != nil {
		if errors.Is(err, ErrDisplayProfileNotFound) {
			return invalid(codeDisplayProfileUnknown, fmt.Sprintf("%q is not a display profile", id))
		}
		return err
	}
	return nil
}

// spiceOptionsFor resolves a guest's choice to the options its spec carries.
// A profile deleted from under the guest falls back to Fast LAN; a read failure
// sends none, which leaves QEMU's defaults rather than failing the start.
func (s *Service) spiceOptionsFor(ctx context.Context, stored string) *agentSpiceOptions {
	id := effectiveProfileID(stored)
	profile, err := s.DisplayProfile(ctx, id)
	if errors.Is(err, ErrDisplayProfileNotFound) {
		profile, _ = builtinProfile(ProfileFastLAN)
		err = nil
	}
	if err != nil {
		s.logger.Warn("kvm: reading the display profile for a guest start", "profile", id, "error", err)
		return nil
	}
	return &agentSpiceOptions{
		ImageCompression:      profile.ImageCompression,
		JPEGWANCompression:    profile.JPEGWANCompression,
		ZlibGLZWANCompression: profile.ZlibGLZWANCompression,
		StreamingVideo:        profile.StreamingVideo,
		PlaybackCompression:   profile.PlaybackCompression,
		AgentMouse:            profile.AgentMouse,
	}
}
