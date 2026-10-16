// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"encoding/json"
	"log/slog"
	"strings"
)

const secretRefsLabel = "com.mcharbor.secret-refs" //nolint:gosec // G101 false positive: a Docker label key, not a credential

func captureSecretRefs(env []string) map[string]string {
	refs := map[string]string{}
	for _, entry := range env {
		key, value, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		if strings.Contains(value, "${secret:") {
			refs[key] = value
		}
	}
	if len(refs) == 0 {
		return nil
	}
	return refs
}

func encodeSecretRefs(refs map[string]string) string {
	if len(refs) == 0 {
		return ""
	}
	encoded, err := json.Marshal(refs)
	if err != nil {
		slog.Warn("containers: encoding secret references failed", "error", err)
		return ""
	}
	return string(encoded)
}

func decodeSecretRefs(raw string) map[string]string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	refs := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &refs); err != nil {
		slog.Warn("containers: decoding secret references failed", "error", err)
		return nil
	}
	return refs
}

func restoreSecretRefs(env []string, refs map[string]string) []string {
	if len(refs) == 0 {
		return env
	}

	drop := make(map[string]struct{}, len(refs)*2)
	for key := range refs {
		drop[key] = struct{}{}
		drop[key+"_FILE"] = struct{}{}
	}

	out := make([]string, 0, len(env)+len(refs))
	for _, entry := range env {
		key, _, found := strings.Cut(entry, "=")
		if !found {
			out = append(out, entry)
			continue
		}
		if _, skip := drop[key]; skip {
			continue
		}
		out = append(out, entry)
	}
	for key, value := range refs {
		out = append(out, key+"="+value)
	}
	return out
}
