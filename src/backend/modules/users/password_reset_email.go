// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package users

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	corenotify "github.com/therealmcsparrow/mcharbor/core/notify"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// Delivery of the password-reset link.
//
// The link is the whole security boundary of the reset flow, so the
// pieces an operator gets to change are deliberately narrow: which
// channel carries it, who it appears to come from, and what it says.
// The token itself, the TTL enforcement, the single-use consumption
// and the session revocation are not configurable.
//
// Everything here is best-effort by design. HandleForgotPassword must
// answer identically for a known and an unknown account, so a missing
// channel, a disabled plugin or a dead SMTP relay can only be logged —
// never surfaced. That is also why this file returns errors up to a
// caller that swallows them rather than writing responses itself.

const (
	// appName is the product name substituted for {{appName}}. A
	// constant rather than a setting: there is no white-label story in
	// the product yet, and inventing a settings key for one template
	// would be the wrong place to start one.
	appName = "McHarbor"

	settingResetTTLMinutes = "security.password_reset_ttl_minutes"
	settingResetChannelID  = "security.password_reset_email_channel_id"
	settingResetFrom       = "security.password_reset_email_from"
	settingResetSubject    = "security.password_reset_email_subject"
	settingResetBody       = "security.password_reset_email_body"

	// Bounds for the configured TTL. Mirrors the range the Security ->
	// Password form enforces; re-checked here because a settings row
	// can also be written by the bulk endpoint or by hand.
	minResetTTL = 5 * time.Minute
	maxResetTTL = 7 * 24 * time.Hour
)

// Fallbacks used when the seeded settings row is missing or an admin has
// blanked a row. Kept in sync with 065_settings.sql.
const (
	defaultResetSubject = "Reset your {{appName}} password"
	defaultResetBody    = "Hello {{displayName}},\n\n" +
		"We received a request to reset the password for your {{appName}} account ({{username}}).\n\n" +
		"Choose a new password here:\n{{link}}\n\n" +
		"This link can be used once and expires in {{expiresMinutes}} minutes, at {{expiresAt}}.\n\n" +
		"If you did not ask for this, you can ignore this message — your password stays " +
		"unchanged and no one has been given access to your account."
)

// ResetEmailSettings is the operator-controlled half of the reset mail.
type ResetEmailSettings struct {
	// ChannelID names a row in communication_channels. Empty means
	// delivery is not configured; the caller logs and sends nothing.
	ChannelID string
	// From overrides the channel's own from_address for this send.
	// Empty means "leave the channel's sender alone".
	From string
	// Subject and Body are templates; see renderResetTemplate.
	Subject string
	Body    string
	// TTL is how long the issued link stays valid.
	TTL time.Duration
}

// Configured reports whether a channel has been chosen.
func (s ResetEmailSettings) Configured() bool {
	return strings.TrimSpace(s.ChannelID) != ""
}

// ResetEmailSettings loads the password-reset delivery configuration.
//
// Missing or unparsable rows fall back to the seeded defaults rather
// than erroring: a reset flow that refuses to run because someone
// blanked a subject line would be a worse failure than sending with
// the default wording.
func (s *Service) ResetEmailSettings(ctx context.Context) (ResetEmailSettings, error) {
	values, err := s.settingValues(ctx,
		settingResetChannelID,
		settingResetFrom,
		settingResetSubject,
		settingResetBody,
		settingResetTTLMinutes,
	)
	if err != nil {
		return ResetEmailSettings{}, err
	}

	out := ResetEmailSettings{
		ChannelID: strings.TrimSpace(values[settingResetChannelID]),
		From:      strings.TrimSpace(values[settingResetFrom]),
		Subject:   strings.TrimSpace(values[settingResetSubject]),
		Body:      values[settingResetBody],
		TTL:       PasswordResetEmailTTL,
	}
	if out.Subject == "" {
		out.Subject = defaultResetSubject
	}
	if strings.TrimSpace(out.Body) == "" {
		out.Body = defaultResetBody
	}
	if minutes, convErr := strconv.Atoi(strings.TrimSpace(values[settingResetTTLMinutes])); convErr == nil {
		ttl := time.Duration(minutes) * time.Minute
		if ttl >= minResetTTL && ttl <= maxResetTTL {
			out.TTL = ttl
		}
	}
	return out, nil
}

// settingValues reads several settings rows in one pass. Absent keys
// are simply missing from the map, which every caller treats as "use
// the default".
func (s *Service) settingValues(ctx context.Context, keys ...string) (map[string]string, error) {
	if len(keys) == 0 {
		return map[string]string{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(keys)), ",")
	args := make([]any, 0, len(keys))
	for _, key := range keys {
		args = append(args, key)
	}

	//nolint:gosec // G202: the concatenated part is a list of "?" markers, every value is bound
	query := "SELECT key, value FROM settings WHERE key IN (" + placeholders + ") LIMIT 1000"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("reading settings: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	out := make(map[string]string, len(keys))
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scanning setting: %w", err)
		}
		out[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating settings: %w", err)
	}
	return out, nil
}

// resetTemplateVars is the substitution set for the subject and body.
type resetTemplateVars struct {
	Username    string
	DisplayName string
	Email       string
	Link        string
	ExpiresAt   time.Time
	TTL         time.Duration
}

// renderResetTemplate expands {{placeholder}} tokens.
//
// Deliberately not text/template: the value being rendered is operator
// input from a settings row, and text/template would hand whoever can
// edit that row the ability to walk arbitrary struct fields and call
// methods on them. A flat replace list can only ever produce the seven
// strings listed here. Unknown tokens are left as written so a typo is
// visible in the mail rather than silently blank.
func renderResetTemplate(tpl string, vars resetTemplateVars) string {
	display := vars.DisplayName
	if strings.TrimSpace(display) == "" {
		display = vars.Username
	}
	replacer := strings.NewReplacer(
		"{{appName}}", appName,
		"{{username}}", vars.Username,
		"{{displayName}}", display,
		"{{email}}", vars.Email,
		"{{link}}", vars.Link,
		"{{expiresMinutes}}", strconv.Itoa(int(vars.TTL.Minutes())),
		"{{expiresAt}}", vars.ExpiresAt.UTC().Format("2006-01-02 15:04 UTC"),
	)
	return replacer.Replace(tpl)
}

// resetLink builds the absolute URL the recipient clicks.
//
// The origin is taken from the request that asked for the reset, which
// is by definition the address the user reached McHarbor on — no
// separate "public URL" setting to drift out of date. X-Forwarded-*
// wins when present so a reverse-proxied install links to the public
// hostname rather than the container's.
//
// The token goes in the path (/reset-password/:token) to match the
// frontend route, and because query strings are the part of a URL most
// likely to end up in a proxy access log.
func resetLink(r *http.Request, token string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := firstForwardedValue(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		scheme = proto
	}

	host := r.Host
	if forwarded := firstForwardedValue(r.Header.Get("X-Forwarded-Host")); forwarded != "" {
		host = forwarded
	}

	return fmt.Sprintf("%s://%s/reset-password/%s", scheme, host, token)
}

// firstForwardedValue takes the left-most entry of a comma-separated
// X-Forwarded-* header — the value the outermost proxy wrote.
func firstForwardedValue(header string) string {
	if header == "" {
		return ""
	}
	return strings.TrimSpace(strings.Split(header, ",")[0])
}

// sendResetEmail delivers the reset link through the configured
// communication channel.
//
// Errors are returned to HandleForgotPassword, which logs and then
// answers with the same neutral message it gives an unknown account.
func (h *Handler) sendResetEmail(
	ctx context.Context,
	r *http.Request,
	recipient resetRecipient,
	token string,
	expiresAt time.Time,
	settings ResetEmailSettings,
) error {
	if !settings.Configured() {
		return fmt.Errorf("no password-reset email channel configured")
	}

	vars := resetTemplateVars{
		Username:    recipient.Username,
		DisplayName: recipient.DisplayName,
		Email:       recipient.Email,
		Link:        resetLink(r, token),
		ExpiresAt:   expiresAt,
		TTL:         settings.TTL,
	}

	dispatcher := corenotify.NewDispatcher(h.app.DB, h.app.Encryption)
	if h.app.PluginAdapters != nil {
		dispatcher.SetPluginRuntime(h.app.PluginAdapters)
	}

	sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// The overrides retarget the channel for this one message: `to` so
	// the mail reaches the account holder instead of the channel's
	// standing recipient list, `subject_prefix` blanked so the subject
	// the operator wrote arrives verbatim, and `from_address` only when
	// they actually asked for an override.
	overrides := map[string]string{
		"to":             recipient.Email,
		"subject_prefix": "",
	}
	if settings.From != "" {
		overrides["from_address"] = settings.From
	}

	if _, err := dispatcher.SendChannel(sendCtx, corenotify.ChannelRequest{
		ChannelID:       settings.ChannelID,
		Title:           renderResetTemplate(settings.Subject, vars),
		Message:         renderResetTemplate(settings.Body, vars),
		ConfigOverrides: overrides,
	}); err != nil {
		return fmt.Errorf("dispatching password reset email: %w", err)
	}
	return nil
}
