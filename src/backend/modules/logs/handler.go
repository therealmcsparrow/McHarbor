// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package logs streams container logs to the browser over Server-Sent Events with per-environment and per-user stream limits.
package logs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for logs HTTP handlers.
type Handler struct {
	app *router.AppDeps
	svc *Service
}

// NewHandler creates a new logs handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{app: app, svc: NewService(app.DockerPool)}
}

// HandleStreamLogs streams container logs via Server-Sent Events.
func (h *Handler) HandleStreamLogs(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	containerID := chi.URLParam(r, "id")
	if containerID == "" {
		response.BadRequestCode(w, r, i18n.ErrLogsContainerReq)
		return
	}

	_, ok := w.(http.Flusher)
	if !ok {
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	query, err := parseStreamQuery(r)
	if err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	envID := h.svc.resolveEnvironmentID(r.Context(), response.ParseEnvID(r))
	release, acquired := h.svc.acquireStream(r.Context(), envID, user.ID)
	if !acquired {
		w.Header().Set("Retry-After", "5")
		response.ErrCode(w, r, http.StatusTooManyRequests, i18n.ErrRateLimitExceed)
		return
	}
	defer release()

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	stream, err := h.svc.prepareStream(ctx, envID, containerID, query)
	if err != nil {
		cancel()
		h.app.Logger.Error("prepare log stream failed", "env", envID, "container", containerID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrLogsFailed, err)
		return
	}
	var workers sync.WaitGroup
	defer func() {
		cancel()
		stream.Close()
		workers.Wait()
	}()

	controller := http.NewResponseController(w)
	if err := setSSEWriteDeadline(controller); err != nil {
		h.app.Logger.Error("set log stream write deadline failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrLogsFailed, err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store, private, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	if _, err := fmt.Fprintf(w, "retry: %d\n\n", streamRetryMillis); err != nil {
		return
	}
	if err := writeSSE(w, "connected", "", map[string]int{
		"maxSessionSeconds": 300,
		"maxRecords":        maxStreamRecords,
		"maxBytes":          maxStreamBytes,
	}); err != nil {
		return
	}
	if err := controller.Flush(); err != nil {
		return
	}

	events := make(chan streamEvent, streamBufferSize)
	workers.Add(1)
	go func() {
		defer workers.Done()
		reason, streamErr := stream.run(ctx, func(record logRecord) error {
			select {
			case events <- streamEvent{Record: &record}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		result := streamResult{Reason: reason, Err: streamErr}
		select {
		case events <- streamEvent{Result: &result}:
		case <-ctx.Done():
		}
	}()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	lineCount := 0
	writtenBytes := 0
	lastCursor := ""

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ctx.Done():
			if r.Context().Err() == nil {
				if setSSEWriteDeadline(controller) == nil {
					h.endSSE(w, controller, streamEnd{Reason: "session_limit", Cursor: lastCursor})
				}
			}
			return
		case <-heartbeat.C:
			if err := setSSEWriteDeadline(controller); err != nil {
				return
			}
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
		case event := <-events:
			if event.Result != nil {
				streamResult := *event.Result
				if err := setSSEWriteDeadline(controller); err != nil {
					return
				}
				if streamResult.Err != nil && !isContextError(streamResult.Err) {
					h.app.Logger.Error("log stream failed", "env", envID, "container", containerID, "error", streamResult.Err)
					if err := writeSSE(w, "stream-error", "", map[string]any{"code": string(i18n.ErrLogsFailed), "retryable": true}); err != nil {
						h.app.Logger.Debug("write log stream error event failed", "env", envID, "container", containerID, "error", err)
					}
				}
				h.endSSE(w, controller, streamEnd{Reason: streamResult.Reason, Cursor: lastCursor})
				return
			}
			if event.Record == nil {
				continue
			}
			record := *event.Record
			payload, marshalErr := json.Marshal(record)
			if marshalErr != nil {
				h.app.Logger.Error("encode log stream record failed", "error", marshalErr)
				return
			}
			if lineCount >= maxStreamRecords || writtenBytes+len(payload) > maxStreamBytes {
				reason := "record_limit"
				if writtenBytes+len(payload) > maxStreamBytes {
					reason = "byte_limit"
				}
				cancel()
				if setSSEWriteDeadline(controller) == nil {
					h.endSSE(w, controller, streamEnd{Reason: reason, Cursor: lastCursor})
				}
				return
			}
			if err := setSSEWriteDeadline(controller); err != nil {
				return
			}
			if err := writeSSEPayload(w, "log", record.ID, payload); err != nil {
				return
			}
			lineCount++
			writtenBytes += len(payload)
			lastCursor = record.ID
			if err := controller.Flush(); err != nil {
				return
			}
		}
	}
}

func parseStreamQuery(r *http.Request) (streamQuery, error) {
	values := r.URL.Query()
	tail := 500
	if value := values.Get("tail"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > maxStreamTail {
			return streamQuery{}, fmt.Errorf("invalid log tail")
		}
		tail = parsed
	}
	stdout, err := parseBool(values.Get("stdout"), true)
	if err != nil {
		return streamQuery{}, err
	}
	stderr, err := parseBool(values.Get("stderr"), true)
	if err != nil {
		return streamQuery{}, err
	}
	if !stdout && !stderr {
		return streamQuery{}, fmt.Errorf("stdout and stderr cannot both be disabled")
	}
	follow, err := parseBool(values.Get("follow"), false)
	if err != nil {
		return streamQuery{}, err
	}

	cursorValue := r.Header.Get("Last-Event-ID")
	if cursorValue == "" {
		cursorValue = values.Get("cursor")
	}
	if len(cursorValue) > 512 {
		return streamQuery{}, fmt.Errorf("log cursor is too long")
	}
	cursor, err := decodeCursor(cursorValue)
	if err != nil {
		return streamQuery{}, err
	}
	since := values.Get("since")
	if cursor.Timestamp != "" && since != "" {
		return streamQuery{}, fmt.Errorf("cursor and since cannot be combined")
	}
	if since != "" && !validSince(since) {
		return streamQuery{}, fmt.Errorf("invalid log since value")
	}

	return streamQuery{
		ShowStdout: stdout,
		ShowStderr: stderr,
		Tail:       tail,
		Follow:     follow,
		Since:      since,
		Cursor:     cursor,
	}, nil
}

func parseBool(value string, fallback bool) (bool, error) {
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("invalid boolean value")
	}
	return parsed, nil
}

func validSince(value string) bool {
	if len(value) > 64 || strings.ContainsAny(value, "\r\n") {
		return false
	}
	if numeric, err := strconv.ParseFloat(value, 64); err == nil {
		return numeric >= 0 && !math.IsNaN(numeric) && !math.IsInf(numeric, 0)
	}
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}

// endSSE writes the terminal "end" event and flushes it; a failure means the client already went away, so it is logged at debug only.
func (h *Handler) endSSE(w http.ResponseWriter, controller *http.ResponseController, end streamEnd) {
	if err := writeSSE(w, "end", "", end); err != nil {
		h.app.Logger.Debug("write log stream end event failed", "error", err)
		return
	}
	if err := controller.Flush(); err != nil {
		h.app.Logger.Debug("flush log stream end event failed", "error", err)
	}
}

func setSSEWriteDeadline(controller *http.ResponseController) error {
	return controller.SetWriteDeadline(time.Now().Add(streamWriteTimeout * time.Second))
}

func writeSSE(w http.ResponseWriter, event, id string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("encoding sse payload: %w", err)
	}
	return writeSSEPayload(w, event, id, payload)
}

func writeSSEPayload(w http.ResponseWriter, event, id string, payload []byte) error {
	if id != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", id); err != nil {
			return fmt.Errorf("writing sse id: %w", err)
		}
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload); err != nil {
		return fmt.Errorf("writing sse event: %w", err)
	}
	return nil
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
