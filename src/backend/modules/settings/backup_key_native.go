// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package settings

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// containerBackupKeyFile is BACKUP_ENCRYPTION_KEY_FILE's default: the Docker secret mount.
const containerBackupKeyFile = "/run/secrets/mcharbor_backup_key" //nolint:gosec // G101: a path, not a credential

// ErrNativeRestartUnsupervised is returned when a native McHarbor has no service manager to
// start it again, so the installed key cannot be put into service by restarting.
var ErrNativeRestartUnsupervised = errors.New("backup key installed, but nothing would restart McHarbor")

var nativeHost struct {
	mu      sync.RWMutex
	set     bool
	dataDir string
	restart func(context.Context) error
}

// SetNativeHost tells the settings module McHarbor runs directly on its host: backup keys then
// live under dataDir and are put into service with restart instead of a container recreate.
func SetNativeHost(dataDir string, restart func(context.Context) error) {
	nativeHost.mu.Lock()
	defer nativeHost.mu.Unlock()
	nativeHost.set, nativeHost.dataDir, nativeHost.restart = true, dataDir, restart
}

func nativeHostConfig() (dataDir string, restart func(context.Context) error, ok bool) {
	nativeHost.mu.RLock()
	defer nativeHost.mu.RUnlock()
	return nativeHost.dataDir, nativeHost.restart, nativeHost.set
}

// NativeBackupKeyPath is where a native install keeps its backup key: the data directory is the
// one place the systemd unit lets McHarbor write.
func NativeBackupKeyPath(dataDir string) string {
	return filepath.Join(dataDir, filepath.FromSlash(backupSecretRelativeKey))
}

// ResolveBackupKeyFile picks the key file a native install reads. An operator-chosen path is
// kept; only the container default, which nothing on a native host ever creates, is replaced.
func ResolveBackupKeyFile(configured, dataDir string) string {
	if configured != "" && configured != containerBackupKeyFile {
		return configured
	}
	if configured != "" {
		if _, err := os.Stat(configured); err == nil {
			return configured
		}
	}
	return NativeBackupKeyPath(dataDir)
}

func (s *Service) installBackupKeyNative(ctx context.Context, key, dataDir string, restart func(context.Context) error) (BackupKeyInstallResponse, error) {
	path := NativeBackupKeyPath(dataDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return BackupKeyInstallResponse{}, fmt.Errorf("creating backup key directory: %w", err)
	}
	if current, err := os.ReadFile(path); err == nil && string(current) != key {
		// Replacing the key strands every backup encrypted with the old one; keep it beside.
		backup := path + "." + strconv.FormatInt(time.Now().Unix(), 10) + ".bak"
		if err := os.Rename(path, backup); err != nil {
			return BackupKeyInstallResponse{}, fmt.Errorf("keeping the previous backup key: %w", err)
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".backup-key-*")
	if err != nil {
		return BackupKeyInstallResponse{}, fmt.Errorf("creating backup key file: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		return BackupKeyInstallResponse{}, errors.Join(fmt.Errorf("restricting backup key file: %w", err), tmp.Close(), os.Remove(tmp.Name()))
	}
	if _, err := tmp.WriteString(key); err != nil {
		return BackupKeyInstallResponse{}, errors.Join(fmt.Errorf("writing backup key: %w", err), tmp.Close(), os.Remove(tmp.Name()))
	}
	if err := tmp.Close(); err != nil {
		return BackupKeyInstallResponse{}, errors.Join(fmt.Errorf("closing backup key file: %w", err), os.Remove(tmp.Name()))
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return BackupKeyInstallResponse{}, errors.Join(fmt.Errorf("placing backup key file: %w", err), os.Remove(tmp.Name()))
	}

	// McHarbor reads the key once at boot, so a restart is what puts it into service.
	if restart == nil {
		return BackupKeyInstallResponse{}, ErrNativeRestartUnsupervised
	}
	if err := restart(ctx); err != nil {
		return BackupKeyInstallResponse{}, err
	}
	return BackupKeyInstallResponse{
		Secret:     backupSecretName,
		SecretPath: path,
		Output:     "installed the backup key and scheduled a McHarbor restart",
	}, nil
}

func nativeSetupCommand(path, encodedKey string) string {
	dir := filepath.ToSlash(filepath.Dir(path))
	return fmt.Sprintf("sudo install -d -o mcharbor -g mcharbor -m 700 %s\n"+
		"printf '%%s' '%s' | sudo install -o mcharbor -g mcharbor -m 600 /dev/stdin %s\n"+
		"sudo systemctl restart mcharbor", dir, encodedKey, filepath.ToSlash(path))
}
