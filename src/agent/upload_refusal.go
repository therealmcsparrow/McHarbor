// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// maxRefusalBodyBytes caps how much of a refused upload's answer is read; the
// envelope is a sentence, and a proxy's error page must not be buffered whole.
const maxRefusalBodyBytes = 4 << 10

// uploadRefusal builds the error for a backup upload McHarbor answered with a
// non-2xx status, carrying McHarbor's own reason when the body holds one. The
// status alone ("502 Bad Gateway") told the operator nothing about the cause.
func uploadRefusal(status string, body io.Reader) error {
	var envelope struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	raw := errlog.ReadAll(io.LimitReader(body, maxRefusalBodyBytes))
	if json.Unmarshal(raw, &envelope) == nil {
		if reason := strings.TrimSpace(envelope.Error); reason != "" {
			return fmt.Errorf("compute: McHarbor refused the backup upload with %s: %s", status, reason)
		}
		if code := strings.TrimSpace(envelope.Code); code != "" {
			return fmt.Errorf("compute: McHarbor refused the backup upload with %s (%s)", status, code)
		}
	}
	return fmt.Errorf("compute: McHarbor refused the backup upload with %s", status)
}
