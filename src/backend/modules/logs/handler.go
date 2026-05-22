// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package logs

import (
	"bufio"
	"fmt"
	"net/http"
	"strconv"

	"github.com/docker/docker/api/types/container"
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for logs HTTP handlers.
type Handler struct {
	app *router.AppDeps
}

// NewHandler creates a new logs handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{app: app}
}

// HandleStreamLogs streams container logs via Server-Sent Events.
func (h *Handler) HandleStreamLogs(w http.ResponseWriter, r *http.Request) {
	containerID := chi.URLParam(r, "id")
	if containerID == "" {
		response.BadRequestCode(w, r, i18n.ErrLogsContainerReq)
		return
	}

	envID := response.ParseEnvID(r)
	q := r.URL.Query()

	showStdout := q.Get("stdout") != "false"
	showStderr := q.Get("stderr") != "false"
	tail := q.Get("tail")
	since := q.Get("since")
	follow := q.Get("follow") == "true"

	if tail == "" {
		tail = "100"
	}

	cli, err := h.app.DockerPool.Get(envID)
	if err != nil {
		h.app.Logger.Error("logs: docker client error", "error", err, "env", envID)
		response.InternalErrorCode(w, r, i18n.ErrLogsFailed)
		return
	}

	opts := container.LogsOptions{
		ShowStdout: showStdout,
		ShowStderr: showStderr,
		Tail:       tail,
		Follow:     follow,
		Timestamps: true,
	}

	if since != "" {
		opts.Since = since
	}

	ctx := r.Context()
	logReader, err := cli.ContainerLogs(ctx, containerID, opts)
	if err != nil {
		h.app.Logger.Error("logs: container logs error", "error", err, "container", containerID)
		response.InternalErrorCode(w, r, i18n.ErrLogsFailed)
		return
	}
	defer logReader.Close()

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	scanner := bufio.NewScanner(logReader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // up to 1MB lines

	lineCount := 0
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Text()

		// Docker multiplexed stream: first 8 bytes are header when TTY is disabled.
		// Strip the header if present (non-printable leading bytes).
		if len(line) > 8 && (line[0] == 1 || line[0] == 2) {
			streamType := "stdout"
			if line[0] == 2 {
				streamType = "stderr"
			}
			line = line[8:]
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", streamType, line)
		} else {
			fmt.Fprintf(w, "data: %s\n\n", line)
		}

		lineCount++
		// Flush periodically to avoid buffering too many lines
		if lineCount%10 == 0 || !follow {
			flusher.Flush()
		}
	}

	if err := scanner.Err(); err != nil {
		h.app.Logger.Debug("logs: scanner error", "error", err)
	}

	// Send end-of-stream event
	fmt.Fprintf(w, "event: end\ndata: %s\n\n", strconv.Itoa(lineCount))
	flusher.Flush()
}
