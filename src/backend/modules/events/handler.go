// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package events

import (
	"encoding/json"
	"fmt"
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
	Type   string            `json:"type"`
	Action string            `json:"action"`
	Actor  eventActor        `json:"actor"`
	Time   int64             `json:"time"`
	Status string            `json:"status,omitempty"`
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
	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\"}\n\n")
	flusher.Flush()

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-errCh:
			if err != nil {
				slog.Error("events: docker events stream error", "error", err)
				fmt.Fprintf(w, "event: error\ndata: {\"error\":\"event stream error\"}\n\n")
				flusher.Flush()
			}
			return
		case event := <-eventsCh:
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

			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", data.Type, string(payload))
			flusher.Flush()
		}
	}
}
