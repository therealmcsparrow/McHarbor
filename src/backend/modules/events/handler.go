// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for events HTTP handlers.
type Handler struct {
	app *router.AppDeps
}

// NewHandler creates a new events handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{app: app}
}

// eventData is the JSON shape sent to clients via SSE.
type eventData struct {
	Type   string     `json:"type"`
	Action string     `json:"action"`
	Actor  eventActor `json:"actor"`
	Time   int64      `json:"time"`
	Status string     `json:"status,omitempty"`
}

type eventActor struct {
	ID         string            `json:"id"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// HandleStream streams Docker events via Server-Sent Events.
func (h *Handler) HandleStream(w http.ResponseWriter, r *http.Request) {
	envID := response.ParseEnvID(r)
	q := r.URL.Query()

	cli, err := h.app.DockerPool.Get(envID)
	if err != nil {
		h.app.Logger.Error("events: docker client error", "error", err, "env", envID)
		response.InternalErrorCode(w, r, i18n.ErrEventsFailed)
		return
	}

	ctx := r.Context()

	filterArgs := filters.NewArgs()
	if eventType := q.Get("type"); eventType != "" {
		filterArgs.Add("type", eventType)
	}

	eventsCh, errCh := cli.Events(ctx, events.ListOptions{
		Filters: filterArgs,
	})

	flusher, ok := w.(http.Flusher)
	if !ok {
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	// Disable the server write deadline for this long-lived SSE stream.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Send initial connection event
	if _, writeErr := fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\"}\n\n"); writeErr != nil {
		return
	}
	flusher.Flush()

	for {
		select {
		case <-ctx.Done():
			return
		case err, ok := <-errCh:
			if !ok {
				return
			}
			if err != nil {
				if expectedDockerEventStreamClose(ctx, err) {
					h.app.Logger.Debug("events: docker events stream closed", "error", err, "env", envID)
				} else {
					slog.Error("events: docker events stream error", "error", err, "env", envID)
					if _, writeErr := fmt.Fprintf(w, "event: error\ndata: {\"error\":\"event stream error\"}\n\n"); writeErr != nil {
						return
					}
					flusher.Flush()
				}
			}
			return
		case event, ok := <-eventsCh:
			if !ok {
				return
			}
			data := eventData{
				Type:   string(event.Type),
				Action: string(event.Action),
				Actor: eventActor{
					ID:         event.Actor.ID,
					Attributes: event.Actor.Attributes,
				},
				Time:   event.Time,
				Status: string(event.Status),
			}

			payload, jsonErr := json.Marshal(data)
			if jsonErr != nil {
				continue
			}

			if _, writeErr := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", data.Type, string(payload)); writeErr != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func expectedDockerEventStreamClose(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil {
		return true
	}
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF)
}
