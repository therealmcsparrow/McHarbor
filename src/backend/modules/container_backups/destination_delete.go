// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// ErrBackupDestinationDeleteFailed is returned by destination delete
// helpers when the remote provider rejects the delete. The caller
// surfaces this in the response and the audit log so operators can
// clean up stragglers by hand.
var ErrBackupDestinationDeleteFailed = errors.New("backup destination delete failed")

// deleteDestinationFile removes the backup archive at one
// destination. It selects the right provider delete function based on
// the destination's location_type and skips silently for providers
// we don't support yet (so adding new providers doesn't silently
// strand files).
//
// Errors are logged but never returned as fatal — DeleteRun continues
// even when one destination can't be cleaned up, because the
// operator still wants the DB row gone and the local archive gone.
func (s *Service) deleteDestinationFile(
	ctx context.Context,
	destination BackupRunDestination,
	location *backupStorageDestination,
	logger *slog.Logger,
) error {
	if strings.TrimSpace(destination.Path) == "" {
		return nil
	}
	switch destination.LocationType {
	case "local":
		return deleteLocalDestinationFile(destination.Path, logger)
	case "samba":
		if location == nil {
			return fmt.Errorf("samba destination %s is missing storage location config", destination.ID)
		}
		return s.deleteSambaDestinationFile(ctx, *location, destination.Path, logger)
	case "onedrive_personal", "onedrive_business", "sharepoint":
		if location == nil {
			return fmt.Errorf("OneDrive destination %s is missing storage location config", destination.ID)
		}
		return s.deleteMicrosoftDestinationFile(ctx, *location, destination.Path, logger)
	default:
		if logger != nil {
			logger.Warn(
				"container backup destination delete skipped: unsupported provider",
				"destination", destination.ID,
				"type", destination.LocationType,
				"path", destination.Path,
			)
		}
		return nil
	}
}

// deleteLocalDestinationFile removes the archive file from a local
// storage destination. The path stored on the destination row is the
// absolute file path written by uploadBackupToLocal. We treat missing
// files as success — the operator's intent (no leftover file) is met.
func deleteLocalDestinationFile(remotePath string, logger *slog.Logger) error {
	cleaned, err := cleanLocalBackupPath(remotePath)
	if err != nil {
		return fmt.Errorf("invalid local destination path: %w", err)
	}
	if err := os.Remove(cleaned); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("removing local destination file %s: %w", cleaned, err)
	}
	if logger != nil {
		logger.Info("container backup destination local file removed", "path", cleaned)
	}
	return nil
}

// deleteMicrosoftDestinationFile deletes a file from OneDrive /
// SharePoint using Microsoft Graph. The Microsoft Graph DELETE
// endpoint accepts `root:/<encoded-path>:` for both `/me/drive` and
// `/drives/{id}` paths, mirroring the upload session endpoints.
//
// Auth reuses microsoftAccessToken (which refreshes and persists the
// new tokens), and 404 is treated as success so re-running a delete
// after a partial failure doesn't fail again.
func (s *Service) deleteMicrosoftDestinationFile(
	ctx context.Context,
	location backupStorageDestination,
	remotePath string,
	logger *slog.Logger,
) error {
	accessToken, err := s.microsoftAccessToken(ctx, location)
	if err != nil {
		return fmt.Errorf("refreshing OneDrive access token: %w", err)
	}
	encodedPath := microsoftGraphPath(remotePath)
	if encodedPath == "" {
		return fmt.Errorf("destination path is empty")
	}
	endpoints := microsoftDeleteEndpoints(location, encodedPath)

	var firstErr error
	for _, endpoint := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
		if err != nil {
			return fmt.Errorf("creating OneDrive delete request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("deleting OneDrive file: %w", err)
			}
			if logger != nil {
				logger.Warn(
					"container backup destination OneDrive delete attempt failed",
					"storage", location.ID,
					"name", location.Name,
					"path", remotePath,
					"endpoint", endpoint,
					"error", err,
				)
			}
			if ctx.Err() != nil {
				return firstErr
			}
			continue
		}
		body := limitedResponseBody(resp.Body)
		_ = resp.Body.Close()
		// 204 No Content = success; 404 Not Found = already gone.
		if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK ||
			resp.StatusCode == http.StatusNotFound {
			if logger != nil {
				logger.Info(
					"container backup destination OneDrive file removed",
					"storage", location.ID,
					"name", location.Name,
					"path", remotePath,
					"endpoint", endpoint,
					"status", resp.StatusCode,
				)
			}
			return nil
		}
		err = fmt.Errorf("%w: status %d: %s", ErrBackupDestinationDeleteFailed, resp.StatusCode, body)
		if firstErr == nil {
			firstErr = err
		}
		if logger != nil {
			logger.Warn(
				"container backup destination OneDrive delete attempt rejected",
				"storage", location.ID,
				"name", location.Name,
				"path", remotePath,
				"endpoint", endpoint,
				"status", resp.StatusCode,
				"error", err,
			)
		}
		if ctx.Err() != nil {
			return firstErr
		}
	}
	if firstErr == nil {
		firstErr = fmt.Errorf("%w: no OneDrive delete endpoint reachable", ErrBackupDestinationDeleteFailed)
	}
	return firstErr
}

// microsoftDeleteEndpoints returns the Graph URLs that should be
// tried (in order) when deleting a file at remotePath. Mirrors the
// upload-session endpoint list so we delete from the same drive the
// upload targeted.
func microsoftDeleteEndpoints(location backupStorageDestination, encodedPath string) []string {
	if location.LocationType == "onedrive_business" && strings.TrimSpace(location.DriveID) != "" {
		return []string{
			microsoftDriveDeleteEndpoint(location.DriveID, encodedPath),
			microsoftDefaultDriveDeleteEndpoint(encodedPath),
		}
	}
	if location.LocationType != "onedrive_personal" && strings.TrimSpace(location.DriveID) != "" {
		return []string{microsoftDriveDeleteEndpoint(location.DriveID, encodedPath)}
	}
	return []string{microsoftDefaultDriveDeleteEndpoint(encodedPath)}
}

func microsoftDriveDeleteEndpoint(driveID, encodedPath string) string {
	return "https://graph.microsoft.com/v1.0/drives/" + url.PathEscape(strings.TrimSpace(driveID)) + "/root:/" + encodedPath
}

func microsoftDefaultDriveDeleteEndpoint(encodedPath string) string {
	return "https://graph.microsoft.com/v1.0/me/drive/root:/" + encodedPath
}