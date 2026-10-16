// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package bootstrapfs

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// secretFile is the name the container entrypoint has always used, so an
// existing install keeps its sessions across the move into Go.
const secretFile = ".secret"

// EnsureSecret returns the persisted session secret, generating one on first
// boot. It never regenerates: a new secret would invalidate every session.
func EnsureSecret(dataDir string) (string, error) {
	path := filepath.Join(dataDir, secretFile)

	existing, err := os.ReadFile(path)
	if err == nil {
		if secret := strings.TrimSpace(string(existing)); secret != "" {
			return secret, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating session secret: %w", err)
	}
	secret := base64.StdEncoding.EncodeToString(raw)

	if err := os.WriteFile(path, []byte(secret+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("writing %s: %w", path, err)
	}
	return secret, nil
}
