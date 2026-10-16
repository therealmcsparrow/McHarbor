// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package logs

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

const (
	maxStreamTail      = 2000
	maxStreamRecords   = 10000
	maxStreamBytes     = 16 * 1024 * 1024
	maxRawStreamBytes  = 16 * 1024 * 1024
	maxLogLineBytes    = 64 * 1024
	maxStreamSessions  = 100
	maxLocalEnvStreams = 10
	maxAgentEnvStreams = 2
	maxUserStreams     = 3
	streamBufferSize   = 32
	streamRetryMillis  = 1000
	streamWriteTimeout = 10
)

type streamQuery struct {
	ShowStdout bool
	ShowStderr bool
	Tail       int
	Follow     bool
	Since      string
	Cursor     streamCursor
}

type streamCursor struct {
	Version   int    `json:"v"`
	Timestamp string `json:"t"`
	Ordinal   int    `json:"n"`
}

type logRecord struct {
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Stream    string `json:"stream"`
	Message   string `json:"message"`
	Truncated bool   `json:"truncated"`
}

type streamEnd struct {
	Reason string `json:"reason"`
	Cursor string `json:"cursor,omitempty"`
}

type streamResult struct {
	Reason string
	Err    error
}

type streamEvent struct {
	Record *logRecord
	Result *streamResult
}

func encodeCursor(cursor streamCursor) (string, error) {
	payload, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("encoding log cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeCursor(value string) (streamCursor, error) {
	if value == "" {
		return streamCursor{}, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return streamCursor{}, fmt.Errorf("decoding log cursor: %w", err)
	}
	var cursor streamCursor
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return streamCursor{}, fmt.Errorf("parsing log cursor: %w", err)
	}
	if cursor.Version != 1 || cursor.Timestamp == "" || cursor.Ordinal < 1 {
		return streamCursor{}, fmt.Errorf("invalid log cursor")
	}
	parsed, err := time.Parse(time.RFC3339Nano, cursor.Timestamp)
	if err != nil || cursor.Ordinal > maxStreamRecords {
		return streamCursor{}, fmt.Errorf("invalid log cursor")
	}
	cursor.Timestamp = parsed.UTC().Format(time.RFC3339Nano)
	return cursor, nil
}
