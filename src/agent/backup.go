// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type agentBackupProgressWriter struct {
	writer     io.Writer
	conn       *websocket.Conn
	transferID string
	stage      string
	bytes      int64
	lastEmit   time.Time
}

type agentBackupProgressReader struct {
	reader     io.Reader
	conn       *websocket.Conn
	transferID string
	stage      string
	storageID  string
	total      int64
	bytes      int64
	lastEmit   time.Time
}

func (a *Agent) runBackup(ctx context.Context, conn *websocket.Conn, payload BackupPayload) {
	size, err := a.runBackupArchiveAndUploads(ctx, conn, payload)
	if err != nil {
		a.sendBackupResult(conn, payload.TransferID, false, size, nil, err)
		return
	}
	a.sendBackupResult(conn, payload.TransferID, true, size, nil, nil)
}

func (a *Agent) runBackupArchiveAndUploads(ctx context.Context, conn *websocket.Conn, payload BackupPayload) (int64, error) {
	if strings.TrimSpace(payload.TransferID) == "" || strings.TrimSpace(payload.ContainerID) == "" || strings.TrimSpace(payload.EncryptionKey) == "" {
		return 0, fmt.Errorf("invalid backup request")
	}
	if len(payload.StorageDestinations) == 0 {
		return 0, fmt.Errorf("backup storage destination is required")
	}
	cryptoSvc, err := newBackupCryptoFromKeyMaterial(payload.EncryptionKey)
	if err != nil {
		return 0, fmt.Errorf("loading backup encryption key: %w", err)
	}
	tmp, err := os.CreateTemp("", "mcharbor-agent-backup-*.tar")
	if err != nil {
		return 0, fmt.Errorf("creating temporary backup archive: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if closeErr := tmp.Close(); closeErr != nil {
			a.logger.Warn("close temporary backup archive failed", "path", tmpPath, "error", closeErr)
		}
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			a.logger.Warn("remove temporary backup archive failed", "path", tmpPath, "error", removeErr)
		}
	}()

	if err := a.writeAgentBackupArchive(ctx, conn, tmp, cryptoSvc, payload); err != nil {
		return 0, err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return 0, fmt.Errorf("rewinding temporary backup archive: %w", err)
	}
	stat, err := tmp.Stat()
	if err != nil {
		return 0, fmt.Errorf("stating temporary backup archive: %w", err)
	}
	size := stat.Size()
	for _, destination := range payload.StorageDestinations {
		if err := a.uploadAgentBackupArchive(ctx, conn, tmp, size, payload.TransferID, destination); err != nil {
			return size, err
		}
	}
	return size, nil
}

func (a *Agent) writeAgentBackupArchive(ctx context.Context, conn *websocket.Conn, writer io.Writer, cryptoSvc *backupCryptoService, payload BackupPayload) error {
	sendBackupProgress(conn, payload.TransferID, "agent_backup", "", 0, 0)
	inspect, err := a.proxy.InspectContainer(ctx, payload.ContainerID)
	if err != nil {
		return err
	}
	encryptedWriter, _, err := cryptoSvc.EncryptWriter(writer)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(encryptedWriter)
	progressWriter := &agentBackupProgressWriter{writer: tw, conn: conn, transferID: payload.TransferID}
	writeErr := a.writeAgentBackupEntries(ctx, progressWriter, payload, inspect)
	sendBackupProgress(conn, payload.TransferID, "finalizing", "", progressWriter.bytes, 0)
	closeErr := tw.Close()
	encryptCloseErr := encryptedWriter.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return fmt.Errorf("closing backup archive: %w", closeErr)
	}
	if encryptCloseErr != nil {
		return fmt.Errorf("closing encrypted backup archive: %w", encryptCloseErr)
	}
	return nil
}

func (a *Agent) writeAgentBackupEntries(ctx context.Context, tw *agentBackupProgressWriter, payload BackupPayload, inspect DockerContainerInspect) error {
	manifest, err := json.MarshalIndent(map[string]any{
		"format":    "mcharbor.container.backup.v1",
		"createdAt": time.Now().UTC().Format(time.RFC3339),
		"plan": map[string]any{
			"containerId":       payload.ContainerID,
			"containerName":     payload.ContainerName,
			"includeConfig":     payload.IncludeConfig,
			"includeLogs":       payload.IncludeLogs,
			"includeFilesystem": payload.IncludeFilesystem,
			"includeImage":      payload.IncludeImage,
			"selectedMounts":    payload.SelectedMounts,
		},
		"destination": "agent-local",
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("creating backup manifest: %w", err)
	}
	tw.stage = "manifest"
	if err := writeAgentBackupBytes(tw, "manifest.json", manifest); err != nil {
		return err
	}
	if payload.IncludeConfig {
		tw.stage = "config"
		encoded, err := json.MarshalIndent(inspect, "", "  ")
		if err != nil {
			return fmt.Errorf("encoding container inspect: %w", err)
		}
		if err := writeAgentBackupBytes(tw, "container/inspect.json", encoded); err != nil {
			return err
		}
	}
	if payload.IncludeLogs {
		tw.stage = "logs"
		reader, err := a.proxy.ContainerLogs(ctx, payload.ContainerID)
		if err != nil {
			return err
		}
		if err := writeAgentBackupStream(ctx, tw, "container/logs.txt", reader); err != nil {
			return err
		}
	}
	if payload.IncludeFilesystem {
		tw.stage = "filesystem"
		reader, err := a.proxy.ExportContainer(ctx, payload.ContainerID)
		if err != nil {
			return err
		}
		if err := writeAgentBackupStream(ctx, tw, "container/filesystem.tar", reader); err != nil {
			return err
		}
	}
	if payload.IncludeImage {
		imageRef := ""
		if inspect.Config != nil {
			imageRef = inspect.Config.Image
		}
		if imageRef != "" {
			tw.stage = "image"
			reader, err := a.proxy.SaveImage(ctx, imageRef)
			if err != nil {
				return err
			}
			if err := writeAgentBackupStream(ctx, tw, "image/image.tar", reader); err != nil {
				return err
			}
		}
	}
	allowedMounts := map[string]bool{}
	for _, mount := range inspect.Mounts {
		if mount.Destination != "" && (mount.Type == "volume" || mount.Type == "bind") {
			allowedMounts[mount.Destination] = true
		}
	}
	for _, mountPath := range payload.SelectedMounts {
		cleanMount := strings.TrimSpace(mountPath)
		if cleanMount == "" || strings.Contains(cleanMount, "..") || !strings.HasPrefix(cleanMount, "/") || !allowedMounts[cleanMount] {
			return fmt.Errorf("invalid backup mount path")
		}
		tw.stage = "mounts"
		reader, _, err := a.proxy.CopyArchiveFromContainer(ctx, payload.ContainerID, cleanMount)
		if err != nil {
			return fmt.Errorf("copying mounted data %s: %w", cleanMount, err)
		}
		if err := writeAgentBackupStream(ctx, tw, "mounts/"+safeAgentArchiveName(cleanMount)+".tar", reader); err != nil {
			return err
		}
	}
	return nil
}

func writeAgentBackupBytes(tw *agentBackupProgressWriter, name string, data []byte) error {
	if err := tw.writeHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(data))}); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

func writeAgentBackupStream(ctx context.Context, tw *agentBackupProgressWriter, name string, reader io.ReadCloser) error {
	defer reader.Close()
	tmp, err := os.CreateTemp("", "mcharbor-agent-backup-entry-*.tar")
	if err != nil {
		return fmt.Errorf("creating backup entry temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	n, err := copyAgentBackupWithContext(ctx, tmp, reader)
	if err != nil {
		return fmt.Errorf("spooling backup entry %s: %w", name, err)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewinding backup entry %s: %w", name, err)
	}
	if err := tw.writeHeader(&tar.Header{Name: name, Mode: 0644, Size: n}); err != nil {
		return err
	}
	if _, err := copyAgentBackupWithContext(ctx, tw, tmp); err != nil {
		return fmt.Errorf("writing backup entry %s: %w", name, err)
	}
	return nil
}

func (w *agentBackupProgressWriter) writeHeader(header *tar.Header) error {
	if tw, ok := w.writer.(*tar.Writer); ok {
		return tw.WriteHeader(header)
	}
	return fmt.Errorf("backup progress writer is not a tar writer")
}

func (w *agentBackupProgressWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	if n > 0 {
		w.bytes += int64(n)
		if w.lastEmit.IsZero() || time.Since(w.lastEmit) >= 3*time.Second {
			w.lastEmit = time.Now()
			sendBackupProgress(w.conn, w.transferID, w.stage, "", w.bytes, 0)
		}
	}
	return n, err
}

func (a *Agent) uploadAgentBackupArchive(ctx context.Context, conn *websocket.Conn, file *os.File, size int64, transferID string, destination BackupStorageDestination) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewinding backup archive for upload: %w", err)
	}
	uploadURL, err := a.resolveServerURL(destination.UploadURL)
	if err != nil {
		return err
	}
	reader := &agentBackupProgressReader{
		reader:     file,
		conn:       conn,
		transferID: transferID,
		stage:      "uploading",
		storageID:  destination.Name,
		total:      size,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, reader)
	if err != nil {
		return fmt.Errorf("building backup upload request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+destination.Token)
	req.Header.Set("Content-Type", "application/x-tar")
	req.ContentLength = size
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("uploading backup archive to %s: %w", destination.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("backup upload to %s returned status %d: %s", destination.Name, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	sendBackupProgress(conn, transferID, "uploading", destination.Name, size, size)
	return nil
}

func (a *Agent) runBackupRestore(ctx context.Context, conn *websocket.Conn, payload BackupPayload) {
	restored, err := a.runBackupRestoreArchive(ctx, conn, payload)
	if err != nil {
		a.sendBackupResult(conn, payload.TransferID, false, 0, nil, err)
		return
	}
	a.sendBackupResult(conn, payload.TransferID, true, 0, restored, nil)
}

func (a *Agent) runBackupRestoreArchive(ctx context.Context, conn *websocket.Conn, payload BackupPayload) ([]string, error) {
	if strings.TrimSpace(payload.TransferID) == "" || strings.TrimSpace(payload.ContainerID) == "" || strings.TrimSpace(payload.ArchiveURL) == "" || strings.TrimSpace(payload.ArchiveToken) == "" || strings.TrimSpace(payload.EncryptionKey) == "" {
		return nil, fmt.Errorf("invalid backup restore request")
	}
	cryptoSvc, err := newBackupCryptoFromKeyMaterial(payload.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("loading backup encryption key: %w", err)
	}
	archiveURL, err := a.resolveServerURL(payload.ArchiveURL)
	if err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp("", "mcharbor-agent-restore-full-*.tar")
	if err != nil {
		return nil, fmt.Errorf("creating temporary restore archive: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if closeErr := tmp.Close(); closeErr != nil {
			a.logger.Warn("close temporary restore archive failed", "path", tmpPath, "error", closeErr)
		}
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			a.logger.Warn("remove temporary restore archive failed", "path", tmpPath, "error", removeErr)
		}
	}()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, archiveURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building restore archive download request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+payload.ArchiveToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading restore archive: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("restore archive download returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	reader := &agentBackupProgressReader{reader: resp.Body, conn: conn, transferID: payload.TransferID, stage: "restore_download", total: payload.ArchiveSize}
	if _, err := copyAgentBackupWithContext(ctx, tmp, reader); err != nil {
		return nil, fmt.Errorf("writing temporary restore archive: %w", err)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewinding temporary restore archive: %w", err)
	}
	decrypted, err := cryptoSvc.DecryptReader(tmp)
	if err != nil {
		return nil, err
	}
	defer decrypted.Close()
	sendBackupProgress(conn, payload.TransferID, "restore_apply", "", 0, 0)
	return a.applyBackupArchive(ctx, payload, decrypted)
}

func (a *Agent) applyBackupArchive(ctx context.Context, payload BackupPayload, reader io.Reader) ([]string, error) {
	tr := tar.NewReader(reader)
	mountTargets := map[string]string{}
	restored := []string{}
	wanted := agentRestoreSelection(payload.RestoreItems)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading backup restore entry: %w", err)
		}
		if header == nil || header.Typeflag != tar.TypeReg {
			continue
		}
		switch {
		case header.Name == "manifest.json":
			mountTargets = agentRestoreMountTargets(tr)
		case header.Name == "image/image.tar":
			if !wanted("image") {
				continue
			}
			if err := a.applyBackupEntry(ctx, payload.ContainerID, "", tr, header.Size, true); err != nil {
				return nil, err
			}
			restored = append(restored, "image")
		case header.Name == "container/filesystem.tar":
			if !wanted("filesystem") {
				continue
			}
			if err := a.applyBackupEntry(ctx, payload.ContainerID, "/", tr, header.Size, false); err != nil {
				return nil, err
			}
			restored = append(restored, "filesystem")
		case strings.HasPrefix(header.Name, "mounts/") && strings.HasSuffix(header.Name, ".tar"):
			target := mountTargets[header.Name]
			if target == "" {
				return nil, fmt.Errorf("backup mount target is missing")
			}
			if !wanted("mount:" + target) {
				continue
			}
			restoreTarget := filepath.Dir(target)
			if restoreTarget == "." {
				restoreTarget = "/"
			}
			if err := a.applyBackupEntry(ctx, payload.ContainerID, restoreTarget, tr, header.Size, false); err != nil {
				return nil, err
			}
			restored = append(restored, "mount:"+target)
		}
	}
	if len(restored) == 0 {
		return nil, fmt.Errorf("backup archive has no restorable entries")
	}
	return restored, nil
}

func (a *Agent) applyBackupEntry(ctx context.Context, containerID, target string, reader io.Reader, size int64, image bool) error {
	tmp, err := os.CreateTemp("", "mcharbor-agent-restore-entry-*.tar")
	if err != nil {
		return fmt.Errorf("creating restore entry temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	if _, err := copyAgentBackupWithContext(ctx, tmp, reader); err != nil {
		return fmt.Errorf("spooling restore entry: %w", err)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewinding restore entry: %w", err)
	}
	if image {
		return a.proxy.LoadImage(ctx, tmp)
	}
	return a.proxy.CopyArchiveToContainer(ctx, containerID, target, tmp, size)
}

func (r *agentBackupProgressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.bytes += int64(n)
		if r.lastEmit.IsZero() || time.Since(r.lastEmit) >= 3*time.Second || (r.total > 0 && r.bytes >= r.total) {
			r.lastEmit = time.Now()
			sendBackupProgress(r.conn, r.transferID, r.stage, r.storageID, r.bytes, r.total)
		}
	}
	return n, err
}

func sendBackupProgress(conn *websocket.Conn, transferID, stage, storageID string, bytes, total int64) {
	writeMu.Lock()
	_ = conn.WriteJSON(WSMessage{
		Type: MsgTransferProgress,
		Backup: &BackupPayload{
			TransferID:        transferID,
			Stage:             strings.TrimSpace(stage),
			StorageLocationID: strings.TrimSpace(storageID),
			Bytes:             bytes,
			Size:              total,
		},
	})
	writeMu.Unlock()
}

func (a *Agent) sendBackupResult(conn *websocket.Conn, transferID string, success bool, size int64, restored []string, err error) {
	payload := &BackupPayload{TransferID: transferID, Success: success, Size: size, Restored: restored}
	if err != nil {
		payload.Error = err.Error()
	}
	writeMu.Lock()
	if writeErr := conn.WriteJSON(WSMessage{Type: MsgTransferResult, Backup: payload}); writeErr != nil {
		a.logger.Warn("backup result write failed", "transferId", transferID, "error", writeErr)
	}
	writeMu.Unlock()
}

func copyAgentBackupWithContext(ctx context.Context, writer io.Writer, reader io.Reader) (int64, error) {
	buf := make([]byte, 1024*1024)
	var written int64
	for {
		select {
		case <-ctx.Done():
			return written, ctx.Err()
		default:
		}
		n, readErr := reader.Read(buf)
		if n > 0 {
			w, writeErr := writer.Write(buf[:n])
			written += int64(w)
			if writeErr != nil {
				return written, writeErr
			}
			if w != n {
				return written, io.ErrShortWrite
			}
		}
		if errors.Is(readErr, io.EOF) {
			return written, nil
		}
		if readErr != nil {
			return written, readErr
		}
	}
}

func agentRestoreSelection(items []string) func(string) bool {
	if len(items) == 0 {
		return func(string) bool { return true }
	}
	selected := map[string]bool{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			selected[item] = true
		}
	}
	return func(item string) bool {
		return selected[item]
	}
}

func agentRestoreMountTargets(reader io.Reader) map[string]string {
	var manifest struct {
		Plan struct {
			SelectedMounts []string `json:"selectedMounts"`
		} `json:"plan"`
	}
	if err := json.NewDecoder(reader).Decode(&manifest); err != nil {
		return nil
	}
	targets := map[string]string{}
	for _, mountPath := range manifest.Plan.SelectedMounts {
		targets["mounts/"+safeAgentArchiveName(mountPath)+".tar"] = mountPath
	}
	return targets
}

func safeAgentArchiveName(value string) string {
	value = strings.Trim(value, "/")
	value = strings.NewReplacer("\\", "-", "/", "-", " ", "-", "..", "-").Replace(value)
	value = strings.Trim(value, "-")
	if value == "" {
		return "mount"
	}
	return value
}
