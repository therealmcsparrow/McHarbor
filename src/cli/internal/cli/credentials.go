// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/zalando/go-keyring"
	"gopkg.in/yaml.v3"
)

const keyringService = "mcharbor"

// keyringAvailable is a variable so tests can force the file fallback; headless
// Linux has no Secret Service, and that is the normal case for servers.
var keyringAvailable = func() bool { return os.Getenv("MCHARBOR_NO_KEYRING") == "" }

// keyringTimeout bounds every keyring call. A Secret Service with a locked
// collection waits for an unlock prompt nobody on a server will ever answer.
var keyringTimeout = 3 * time.Second

// The keyring operations are variables so a test can make them hang.
var (
	keyringGet    = keyring.Get
	keyringSet    = keyring.Set
	keyringDelete = keyring.Delete
)

var errKeyringTimeout = errors.New("the OS keyring did not answer")

// withKeyring runs fn with keyringTimeout; on timeout the call is abandoned
// and the caller falls back to the credentials file.
func withKeyring(fn func() error) error {
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-time.After(keyringTimeout):
		return errKeyringTimeout
	}
}

// loadAPIKey returns the key for contextName and where it came from. The
// environment variable wins so CI never depends on a keyring it does not have.
func loadAPIKey(contextName string) (string, string, error) {
	if key := os.Getenv("MCHARBOR_API_KEY"); key != "" {
		return key, "env", nil
	}
	if contextName == "" {
		return "", "", nil
	}
	if keyringAvailable() {
		var key string
		err := withKeyring(func() error {
			var getErr error
			key, getErr = keyringGet(keyringService, contextName)
			return getErr
		})
		if err == nil && key != "" {
			return key, "keyring", nil
		}
	}
	creds, err := readCredentialFile()
	if err != nil {
		return "", "", err
	}
	if key := creds[contextName]; key != "" {
		return key, "file", nil
	}
	return "", "", nil
}

// storeAPIKey saves key for contextName in the keyring when one is usable, and
// in an owner-only file otherwise; neither path is a degraded one.
func storeAPIKey(contextName, key string) (string, error) {
	if keyringAvailable() {
		if err := withKeyring(func() error { return keyringSet(keyringService, contextName, key) }); err == nil {
			if err := deleteFromCredentialFile(contextName); err != nil {
				return "", err
			}
			return "keyring", nil
		}
	}
	creds, err := readCredentialFile()
	if err != nil {
		return "", err
	}
	creds[contextName] = key
	return "file", writeCredentialFile(creds)
}

// deleteAPIKey removes contextName's key from every store it could be in.
func deleteAPIKey(contextName string) error {
	if keyringAvailable() {
		err := withKeyring(func() error { return keyringDelete(keyringService, contextName) })
		if err != nil && !errors.Is(err, keyring.ErrNotFound) && !errors.Is(err, errKeyringTimeout) {
			return fmt.Errorf("removing the key from the OS keyring: %w", err)
		}
	}
	return deleteFromCredentialFile(contextName)
}

func credentialPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.yaml"), nil
}

func readCredentialFile() (map[string]string, error) {
	path, err := credentialPath()
	if err != nil {
		return nil, err
	}
	creds := map[string]string{}
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return creds, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := yaml.Unmarshal(raw, &creds); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return creds, nil
}

func writeCredentialFile(creds map[string]string) error {
	path, err := credentialPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	raw, err := yaml.Marshal(creds)
	if err != nil {
		return fmt.Errorf("encoding credentials: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

func deleteFromCredentialFile(contextName string) error {
	creds, err := readCredentialFile()
	if err != nil {
		return err
	}
	if _, ok := creds[contextName]; !ok {
		return nil
	}
	delete(creds, contextName)
	return writeCredentialFile(creds)
}
