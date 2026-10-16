// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/config"
	"github.com/therealmcsparrow/mcharbor/core/envredact"
)

// secretEnv is the variable the shared secret arrives in, pinned by the operator or filled
// in from the data directory before the config is loaded.
const secretEnv = "MCHARBOR_SECRET"

// secretFileEnv is the Docker-secret form of secretEnv; config.Load reads the file.
const secretFileEnv = "MCHARBOR_SECRET_FILE"

// rootOfTrustEnv are the variables that carry McHarbor's own keys. After the config has
// loaded, every reader uses the config, so all of them leave the process environment.
var rootOfTrustEnv = []string{secretEnv, "ENCRYPTION_KEY", "MCHARBOR_DB_DSN", "MCHARBOR_BACKUP_KEY"}

// secretFromOperator reports whether the operator supplied the secret, as a variable or as a
// file. Either way the data-directory secret must not be generated or exported: a restore
// must not replace it, and config.Load refuses a variable set alongside its _FILE form. A
// value of only whitespace counts as unset, as it does for the _FILE form.
func secretFromOperator() bool {
	return strings.TrimSpace(os.Getenv(secretEnv)) != "" || strings.TrimSpace(os.Getenv(secretFileEnv)) != ""
}

// loadConfig loads the configuration, registers its keys as the process-wide pinned set so
// no redactor can print them, and then removes every root-of-trust variable from the process
// environment, so no child process (compose, a node runtime) and no later os.Getenv sees one;
// every reader after this point uses the config. This does not reach the container's own
// Config.Env or /proc/1/environ, which keep a variable-pinned value: the _FILE forms keep a
// key out of both.
func loadConfig() (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	envredact.RegisterPinned(cfg.Secret, cfg.EncryptionKey, cfg.DBDSN)
	for _, name := range rootOfTrustEnv {
		if err := os.Unsetenv(name); err != nil {
			return nil, fmt.Errorf("removing %s from the environment: %w", name, err)
		}
	}
	return cfg, nil
}
