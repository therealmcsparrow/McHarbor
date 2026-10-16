// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/selfguard"
)

// Executors for the storage-contract and backup/restore procedure
// nodes. Every executor follows the action-node contract: clone the
// incoming msg, replace payload, route to "output" or "error".

// ---------------------------------------------------------------------------
// storage-object — list / upload / download / delete through the
// storage adapter contract (local or plugin-owned destinations).
// ---------------------------------------------------------------------------

func (s *Service) executeStorageObject(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	if s.storageObjects == nil {
		return "", nil, ErrStorageObjectsUnavailable
	}
	locationID := nodeConfigString(node, "storage_location_id", "")
	if locationID == "" {
		return "", nil, fmt.Errorf("storage_location_id is required")
	}

	switch action := nodeConfigString(node, "action", "list"); action {
	case "list":
		return s.executeStorageObjectList(ctx, node, msg, locationID)
	case "upload":
		return s.executeStorageObjectUpload(ctx, node, msg, locationID)
	case "download":
		return s.executeStorageObjectDownload(ctx, node, msg, locationID)
	case "delete":
		return s.executeStorageObjectDelete(ctx, node, msg, locationID)
	default:
		return "", nil, unsupportedSelector("storage-object", "action", action)
	}
}

func (s *Service) executeStorageObjectList(ctx context.Context, node *CanvasNode, msg Msg, locationID string) (string, Msg, error) {
	prefix := nodeConfigString(node, "prefix", "")
	opCtx, cancel := context.WithTimeout(ctx, nodeLongOpTimeout)
	defer cancel()

	items, err := s.storageObjects.ListObjects(opCtx, locationID, prefix)
	if err != nil {
		s.logger.Error("workflows: storage object list failed", "error", err, "location", locationID, "prefix", prefix)
		return "error", storageObjectError(msg, "storage object list failed", locationID, prefix), nil
	}
	if items == nil {
		items = []StorageObjectSummary{}
	}
	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"storageLocationId": locationID,
		"prefix":            prefix,
		"items":             items,
		"count":             len(items),
	}
	return "output", out, nil
}

func (s *Service) executeStorageObjectUpload(ctx context.Context, node *CanvasNode, msg Msg, locationID string) (string, Msg, error) {
	key := nodeConfigString(node, "key", "")
	if key == "" {
		return "", nil, fmt.Errorf("key is required")
	}

	var data []byte
	switch mode := nodeConfigString(node, "source_mode", "property"); mode {
	case "text":
		data = []byte(nodeConfigString(node, "content", ""))
	case "property":
		property := nodeConfigString(node, "property", "payload")
		raw, ok := GetPath(msg, property)
		if !ok {
			return "error", storageObjectError(msg, "upload source property not found", locationID, key), nil
		}
		data = storageBytesFromValue(raw, nodeConfigString(node, "content_encoding", "auto"))
	default:
		return "", nil, unsupportedSelector("storage-object", "source_mode", mode)
	}

	opCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if err := s.storageObjects.UploadObject(opCtx, locationID, key, data); err != nil {
		s.logger.Error("workflows: storage object upload failed", "error", err, "location", locationID, "key", key)
		return "error", storageObjectError(msg, "storage object upload failed", locationID, key), nil
	}
	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"storageLocationId": locationID,
		"key":               key,
		"size":              len(data),
		"uploadedAt":        time.Now().UTC().Format(time.RFC3339),
	}
	return "output", out, nil
}

func (s *Service) executeStorageObjectDownload(ctx context.Context, node *CanvasNode, msg Msg, locationID string) (string, Msg, error) {
	key := nodeConfigString(node, "key", "")
	if key == "" {
		return "", nil, fmt.Errorf("key is required")
	}
	opCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	data, err := s.storageObjects.DownloadObject(opCtx, locationID, key)
	if err != nil {
		s.logger.Error("workflows: storage object download failed", "error", err, "location", locationID, "key", key)
		return "error", storageObjectError(msg, "storage object download failed", locationID, key), nil
	}

	var value interface{}
	switch encoding := nodeConfigString(node, "output_encoding", "text"); encoding {
	case "base64":
		value = base64.StdEncoding.EncodeToString(data)
	case "json":
		var parsed interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return "error", storageObjectError(msg, "storage object is not valid JSON", locationID, key), nil
		}
		value = parsed
	default:
		value = string(data)
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	property := nodeConfigString(node, "output_property", "payload")
	if property == "payload" {
		out["payload"] = value
	} else {
		SetPath(out, property, value)
		out["payload"] = map[string]interface{}{
			"storageLocationId": locationID,
			"key":               key,
			"size":              len(data),
			"outputProperty":    property,
		}
	}
	return "output", out, nil
}

func (s *Service) executeStorageObjectDelete(ctx context.Context, node *CanvasNode, msg Msg, locationID string) (string, Msg, error) {
	key := nodeConfigString(node, "key", "")
	if key == "" {
		return "", nil, fmt.Errorf("key is required")
	}
	opCtx, cancel := context.WithTimeout(ctx, nodeLongOpTimeout)
	defer cancel()

	if err := s.storageObjects.DeleteObject(opCtx, locationID, key); err != nil {
		s.logger.Error("workflows: storage object delete failed", "error", err, "location", locationID, "key", key)
		return "error", storageObjectError(msg, "storage object delete failed", locationID, key), nil
	}
	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"storageLocationId": locationID,
		"key":               key,
		"deleted":           true,
	}
	return "output", out, nil
}

func storageObjectError(msg Msg, message, locationID, key string) Msg {
	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"error":             message,
		"storageLocationId": locationID,
		"key":               key,
	}
	return out
}

// storageBytesFromValue turns a msg value into upload bytes. Strings
// are written verbatim (or base64-decoded when asked); anything else is
// serialised as JSON so an object payload lands as a readable file.
func storageBytesFromValue(raw interface{}, encoding string) []byte {
	switch v := raw.(type) {
	case []byte:
		return v
	case string:
		if encoding == "base64" {
			if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(v)); err == nil {
				return decoded
			}
		}
		return []byte(v)
	default:
		encoded, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return []byte(fmt.Sprintf("%v", v))
		}
		return encoded
	}
}

// ---------------------------------------------------------------------------
// storage-location-test — connection probe (+ optional quota).
// ---------------------------------------------------------------------------

func (s *Service) executeStorageLocationTest(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	if s.storageObjects == nil {
		return "", nil, ErrStorageObjectsUnavailable
	}
	locationID := nodeConfigString(node, "storage_location_id", "")
	if locationID == "" {
		return "", nil, fmt.Errorf("storage_location_id is required")
	}

	opCtx, cancel := context.WithTimeout(ctx, nodeLongOpTimeout)
	defer cancel()

	result, err := s.storageObjects.Test(opCtx, locationID)
	if err != nil || result == nil {
		s.logger.Error("workflows: storage location test failed", "error", err, "location", locationID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "storage location test failed", "storageLocationId": locationID}
		return "error", out, nil
	}

	payload := map[string]interface{}{
		"storageLocationId": result.StorageLocationID,
		"locationName":      result.LocationName,
		"locationType":      result.LocationType,
		"overallStatus":     result.OverallStatus,
		"duration":          result.Duration,
		"steps":             result.Steps,
	}
	if configBool(node.Config, "include_usage", false) {
		usage, usageErr := s.storageObjects.Usage(opCtx, locationID)
		if usageErr != nil {
			s.logger.Warn("workflows: storage location usage failed", "error", usageErr, "location", locationID)
			payload["usage"] = map[string]interface{}{"supported": false}
		} else if usage != nil {
			payload["usage"] = usage
		}
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = payload
	if configBool(node.Config, "fail_on_warn", false) && result.OverallStatus != "pass" {
		return "error", out, nil
	}
	if result.OverallStatus == "fail" {
		return "error", out, nil
	}
	return "output", out, nil
}

// ---------------------------------------------------------------------------
// container-backup-restore — replay a completed run.
// ---------------------------------------------------------------------------

func (s *Service) executeContainerBackupRestore(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	if s.backupRunner == nil {
		return "", nil, ErrContainerBackupsUnavailable
	}
	runID := nodeConfigString(node, "run_id", "")
	if runID == "" {
		return "", nil, fmt.Errorf("run_id is required")
	}
	input := ContainerBackupRestoreInput{
		SecretKey:    nodeConfigString(node, "secret_key", ""),
		RestoreItems: configStringSlice(node.Config["restore_items"]),
		// A workflow acts for the system: its saved flag, not the run's user, decides.
		SelfHostAuthorized: selfHostAuthorized(ctx),
	}

	opCtx, cancel := context.WithTimeout(ctx, 45*time.Minute)
	defer cancel()

	result, err := s.backupRunner.Restore(opCtx, runID, input)
	var refusal *selfguard.Refusal
	if errors.As(err, &refusal) {
		return "", nil, s.refuseSelfNode(ctx, "", selfCheck{target: refusal.Target, reasons: refusal.Reasons})
	}
	if err != nil {
		// The secret key never reaches the log line.
		s.logger.Error("workflows: container backup restore failed", "error", err, "run", runID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "container backup restore failed", "runId": runID}
		return "error", out, nil
	}
	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = result
	return "output", out, nil
}

// ---------------------------------------------------------------------------
// container-backup-manage — list plans / runs, get, cancel, delete.
// ---------------------------------------------------------------------------

func (s *Service) executeContainerBackupManage(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	if s.backupRunner == nil {
		return "", nil, ErrContainerBackupsUnavailable
	}
	opCtx, cancel := context.WithTimeout(ctx, nodeLongOpTimeout)
	defer cancel()

	switch action := nodeConfigString(node, "action", "list_runs"); action {
	case "list_plans":
		if envID == "" {
			return "", nil, fmt.Errorf("environment is required")
		}
		plans, err := s.backupRunner.ListPlans(opCtx, envID, "")
		if err != nil {
			s.logger.Error("workflows: container backup plan list failed", "error", err, "envID", envID)
			return backupManageError(msg, "container backup plan list failed", action, "")
		}
		items := filterBackupItemsByContainer(toInterfaceSlice(plans), nodeConfigString(node, "container", ""))
		return backupManageOK(msg, map[string]interface{}{"items": items, "count": len(items), "environmentId": envID})

	case "list_runs":
		if envID == "" {
			return "", nil, fmt.Errorf("environment is required")
		}
		runs, err := s.backupRunner.ListRuns(opCtx, envID, "")
		if err != nil {
			s.logger.Error("workflows: container backup run list failed", "error", err, "envID", envID)
			return backupManageError(msg, "container backup run list failed", action, "")
		}
		status := nodeConfigString(node, "status", "")
		if status == "all" {
			status = ""
		}
		items := filterBackupRunsByStatus(runs, status)
		items = filterBackupItemsByContainer(items, nodeConfigString(node, "container", ""))
		items = limitSlice(items, int(configFloat(node.Config, "limit", 0)))
		return backupManageOK(msg, map[string]interface{}{"items": items, "count": len(items), "environmentId": envID})

	case "get_run":
		runID := nodeConfigString(node, "run_id", "")
		if runID == "" {
			return "", nil, fmt.Errorf("run_id is required")
		}
		run, err := s.backupRunner.Run(opCtx, runID)
		if err != nil || run == nil {
			s.logger.Error("workflows: container backup run lookup failed", "error", err, "run", runID)
			return backupManageError(msg, "container backup run not found", action, runID)
		}
		return backupManageOK(msg, run)

	case "cancel_run":
		runID := nodeConfigString(node, "run_id", "")
		if runID == "" {
			return "", nil, fmt.Errorf("run_id is required")
		}
		run, err := s.backupRunner.CancelRun(opCtx, runID)
		if err != nil {
			s.logger.Error("workflows: container backup run cancel failed", "error", err, "run", runID)
			return backupManageError(msg, "container backup run cancel failed", action, runID)
		}
		return backupManageOK(msg, run)

	case "delete_run":
		runID := nodeConfigString(node, "run_id", "")
		if runID == "" {
			return "", nil, fmt.Errorf("run_id is required")
		}
		deleted, err := s.backupRunner.DeleteRun(opCtx, runID)
		if err != nil {
			s.logger.Error("workflows: container backup run delete failed", "error", err, "run", runID)
			return backupManageError(msg, "container backup run delete failed", action, runID)
		}
		if !deleted {
			return backupManageError(msg, "container backup run not found", action, runID)
		}
		return backupManageOK(msg, map[string]interface{}{"runId": runID, "deleted": true})

	default:
		return "", nil, unsupportedSelector("container-backup-manage", "action", action)
	}
}

func backupManageOK(msg Msg, payload interface{}) (string, Msg, error) {
	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = payload
	return "output", out, nil
}

func backupManageError(msg Msg, message, action, runID string) (string, Msg, error) {
	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	payload := map[string]interface{}{"error": message, "action": action}
	if runID != "" {
		payload["runId"] = runID
	}
	out["payload"] = payload
	return "error", out, nil
}

// filterBackupRunsByStatus keeps runs whose "status" field matches.
// Runs arrive as `any` from the runtime adapter, so the filter works on
// the JSON shape rather than the container_backups struct.
func filterBackupRunsByStatus(runs interface{}, status string) []interface{} {
	items := toInterfaceSlice(runs)
	if status == "" {
		return items
	}
	filtered := make([]interface{}, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]interface{}); ok {
			if fmt.Sprintf("%v", m["status"]) == status {
				filtered = append(filtered, item)
			}
			continue
		}
		encoded, err := json.Marshal(item)
		if err != nil {
			continue
		}
		var m map[string]interface{}
		if err := json.Unmarshal(encoded, &m); err != nil {
			continue
		}
		if fmt.Sprintf("%v", m["status"]) == status {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

// filterBackupItemsByContainer keeps plans / runs whose containerName or
// containerId matches the picker value. The container picker emits
// names, while the backup tables store Docker ids, so both are accepted
// (an id prefix as well, matching the UI's short-id display).
func filterBackupItemsByContainer(items []interface{}, filter string) []interface{} {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return items
	}
	filtered := make([]interface{}, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			encoded, err := json.Marshal(item)
			if err != nil {
				continue
			}
			if err := json.Unmarshal(encoded, &m); err != nil {
				continue
			}
		}
		name := fmt.Sprintf("%v", m["containerName"])
		id := fmt.Sprintf("%v", m["containerId"])
		if name == filter || id == filter || strings.HasPrefix(id, filter) {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

func toInterfaceSlice(value interface{}) []interface{} {
	if value == nil {
		return []interface{}{}
	}
	if direct, ok := value.([]interface{}); ok {
		return direct
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return []interface{}{}
	}
	var items []interface{}
	if err := json.Unmarshal(encoded, &items); err != nil {
		return []interface{}{}
	}
	return items
}

func limitSlice(items []interface{}, limit int) []interface{} {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}
