// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// maxHistoryWindow bounds how far back a single request may reach. It is set
// well past the default rollup retention so a legitimate long report is never
// refused, while still rejecting a `from` of 1970 that would scan the table.
const maxHistoryWindow = 400 * 24 * time.Hour

// defaultHistoryRange is what the UI gets when it asks for no window in
// particular: the raw tier, at full resolution.
const defaultHistoryRange = 24 * time.Hour

// historyRanges are the shorthand windows the UI uses. Reports pass explicit
// from/to instead, because a report covers the period printed on its cover
// rather than the last N hours from whenever it was generated.
var historyRanges = map[string]time.Duration{
	"1h":  time.Hour,
	"6h":  6 * time.Hour,
	"12h": 12 * time.Hour,
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
	"90d": 90 * 24 * time.Hour,
}

// HandleContainerHistory returns the stored metric series for one container.
func (h *Handler) HandleContainerHistory(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	containerID := chi.URLParam(r, "id")
	if containerID == "" {
		response.BadRequestCode(w, r, i18n.ErrMetricsContainerRequired)
		return
	}

	from, to, err := parseHistoryWindow(r)
	if err != nil {
		response.BadRequestCause(w, r, i18n.ErrMetricsRangeInvalid, err)
		return
	}

	envID := response.ParseEnvID(r)
	series, err := h.history.ContainerHistory(r.Context(), envID, containerID, from, to)
	if err != nil {
		h.app.Logger.Error("failed to read container metric history",
			"error", err, "env", envID, "container", containerID)
		response.InternalErrorCause(w, r, i18n.ErrMetricsHistoryFailed, err)
		return
	}

	response.OK(w, series)
}

// HandleHostHistory returns the stored environment-aggregate series.
func (h *Handler) HandleHostHistory(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	from, to, err := parseHistoryWindow(r)
	if err != nil {
		response.BadRequestCause(w, r, i18n.ErrMetricsRangeInvalid, err)
		return
	}

	envID := response.ParseEnvID(r)
	series, err := h.history.HostHistory(r.Context(), envID, from, to)
	if err != nil {
		h.app.Logger.Error("failed to read host metric history", "error", err, "env", envID)
		response.InternalErrorCause(w, r, i18n.ErrMetricsHistoryFailed, err)
		return
	}

	response.OK(w, series)
}

// parseHistoryWindow resolves the requested window from either an explicit
// from/to pair or a `range` shorthand.
//
// The two forms exist for two different callers and must not be blended: the UI
// wants "the last 6 hours" and should move with the clock, while a report wants
// a fixed period so that regenerating it later produces the same figures.
func parseHistoryWindow(r *http.Request) (time.Time, time.Time, error) {
	q := r.URL.Query()
	now := time.Now().UTC()

	fromRaw, toRaw := q.Get("from"), q.Get("to")
	if fromRaw != "" || toRaw != "" {
		from, err := parseHistoryTime(fromRaw, now.Add(-defaultHistoryRange))
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		to, err := parseHistoryTime(toRaw, now)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		return validateWindow(from, to)
	}

	window := defaultHistoryRange
	if name := q.Get("range"); name != "" {
		d, ok := historyRanges[name]
		if !ok {
			return time.Time{}, time.Time{}, fmt.Errorf("unknown range %q", name)
		}
		window = d
	}
	return validateWindow(now.Add(-window), now)
}

// parseHistoryTime accepts RFC3339 and a bare YYYY-MM-DD date, which is what a
// report's period fields carry.
func parseHistoryTime(raw string, fallback time.Time) (time.Time, error) {
	if raw == "" {
		return fallback, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("unparseable timestamp %q", raw)
}

func validateWindow(from, to time.Time) (time.Time, time.Time, error) {
	if !to.After(from) {
		return time.Time{}, time.Time{}, fmt.Errorf("window end must be after its start")
	}
	if to.Sub(from) > maxHistoryWindow {
		return time.Time{}, time.Time{}, fmt.Errorf("window exceeds %s", maxHistoryWindow)
	}
	return from, to, nil
}
