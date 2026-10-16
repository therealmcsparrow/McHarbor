// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package failreason turns an install or deploy error into the one line an
// operator can act on, safe to show in a progress stream.
package failreason

import (
	"errors"
	"regexp"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/pluginruntime"
)

var (
	reDockerSocket = regexp.MustCompile(`(?:unix|npipe)://[^\s"']+|(?:/var)?/run/docker\.sock`) // docker socket: redaction pattern, the path is never emitted
	reErrorLine    = regexp.MustCompile(`(?i)error|failed|denied|not found|no such|refused|invalid`)
)

// Of returns the line of err that says why it failed: docker compose prints its
// progress first and the daemon's answer last, so the last line naming an error
// wins over the first. Credentials and Docker socket paths are stripped, and the
// result is truncated; the full error stays in the server log.
func Of(err error) string {
	if err == nil {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(err.Error(), "\r\n", "\n"), "\n")
	pick := ""
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if pick == "" {
			pick = line
		}
		if reErrorLine.MatchString(line) {
			pick = line
			break
		}
	}
	pick = reDockerSocket.ReplaceAllString(pick, "the Docker socket")
	return pluginruntime.RedactAuditError(errors.New(pick))
}
