// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package apispec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// EmitEnv names the variable that makes the server write its document and exit
// instead of serving, which is how CI regenerates the committed spec.
const EmitEnv = "MCHARBOR_EMIT_OPENAPI"

// WriteDocument renders the registry to path as indented JSON with a trailing
// newline, so the committed file diffs line by line and never on a missing EOL.
func WriteDocument(path string, reg *Registry, version string) error {
	raw, err := json.MarshalIndent(Document(reg, version), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal openapi document: %w", err)
	}
	raw = append(raw, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create openapi output dir: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write openapi document: %w", err)
	}
	return nil
}
