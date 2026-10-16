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

	"github.com/moby/moby/api/types/events"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for events HTTP handlers.
type Handler struct {
	app    *router.AppDeps
	broker *Broker
}

// NewHandler creates a new events handler whose tabs share one Docker events
// subscription per environment.
func NewHandler(app *router.AppDeps) *Handler {
	source := func(envID string) (EventSource, error) {
		cli, err := app.DockerPool.Get(envID)
		if err != nil {
			return nil, err
		}
		return cli, nil
	}
	return &Handler{app: app, broker: NewBroker(app.ContextOrBackground(), source)}
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

	sub, leave, err := h.broker.Subscribe(envID, q.Get("type")) //nolint:contextcheck // the shared upstream stream outlives any one request and runs on the broker lifetime context
	if err != nil {
		h.app.Logger.Error("events: docker client error", "error", err, "env", envID)
		response.InternalErrorCause(w, r, i18n.ErrEventsFailed, err)
		return
	}
	defer leave()

	ctx := r.Context()

	flusher, ok := w.(http.Flusher)
	if !ok {
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	// Disable the server write deadline for this long-lived SSE stream.
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		h.app.Logger.Warn("events: disabling write deadline failed", "error", err, "env", envID)
	}

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
		case event, ok := <-sub.Events:
			if !ok {
				err := sub.Err()
				if err != nil && !expectedDockerEventStreamClose(ctx, err) {
					slog.Error("events: docker events stream error", "error", err, "env", envID)
					if _, writeErr := fmt.Fprintf(w, "event: error\ndata: {\"error\":\"event stream error\"}\n\n"); writeErr != nil {
						return
					}
					flusher.Flush()
				}
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
				Status: legacyStatus(event),
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

// legacyStatus rebuilds the deprecated "status" field the SSE payload has always
// carried: the daemon set it to the action for container and image events only,
// and the Moby API types no longer decode it.
func legacyStatus(event events.Message) string {
	if event.Type == events.ContainerEventType || event.Type == events.ImageEventType {
		return string(event.Action)
	}
	return ""
}
