// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package qmp

import (
	"context"
	"errors"
	"fmt"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// BitmapState is one persistent dirty bitmap as QEMU reports it.
//
// Recording says the bitmap is live and tracking writes; Inconsistent says it
// survived something it should not have and cannot be trusted. An incremental
// backup taken against either a missing or an inconsistent bitmap restores to
// a corrupt disk, so both are read before every incremental run rather than
// assumed from the fact that the name is on record.
type BitmapState struct {
	Name         string `json:"name"`
	Granularity  int64  `json:"granularity"`
	Recording    bool   `json:"recording"`
	Inconsistent bool   `json:"inconsistent"`
	Persistent   bool   `json:"persistent"`
	// DirtyBytes is how much has changed since the last backup, which is what
	// makes an incremental's size predictable before it runs.
	DirtyBytes int64 `json:"count"`
}

// bitmapNode is the slice of query-named-block-nodes this file reads.
type bitmapNode struct {
	NodeName string        `json:"node-name"`
	Bitmaps  []BitmapState `json:"dirty-bitmaps"`
}

// Bitmaps returns every dirty bitmap on one block node.
//
// Reads query-named-block-nodes rather than query-block because a backup
// addresses the node, and a disk that is part of a snapshot chain has several
// nodes of which only one carries the bitmap.
func (m *Monitor) Bitmaps(ctx context.Context, nodeName string) ([]BitmapState, error) {
	var nodes []bitmapNode
	if err := m.RunInto(ctx, "query-named-block-nodes", nil, &nodes); err != nil {
		return nil, fmt.Errorf("qmp: listing dirty bitmaps: %w", err)
	}
	for _, node := range nodes {
		if node.NodeName == nodeName {
			return node.Bitmaps, nil
		}
	}
	return nil, nil
}

// Bitmap returns one named bitmap on a node, or false when it is absent.
func (m *Monitor) Bitmap(ctx context.Context, nodeName, bitmapName string) (BitmapState, bool, error) {
	bitmaps, err := m.Bitmaps(ctx, nodeName)
	if err != nil {
		return BitmapState{}, false, err
	}
	for _, bitmap := range bitmaps {
		if bitmap.Name == bitmapName {
			return bitmap, true, nil
		}
	}
	return BitmapState{}, false, nil
}

// AddBitmap starts tracking writes to a node.
//
// Persistent, so the bitmap is written into the qcow2 and survives a guest
// restart — a bitmap that lived only in memory would force a full copy after
// every reboot and make incremental backup useless in practice.
func (m *Monitor) AddBitmap(ctx context.Context, nodeName, bitmapName string) error {
	_, err := m.Run(ctx, "block-dirty-bitmap-add", map[string]interface{}{
		"node":       nodeName,
		"name":       bitmapName,
		"persistent": true,
	})
	if err != nil {
		return fmt.Errorf("qmp: adding dirty bitmap %s: %w", bitmapName, err)
	}
	return nil
}

// RemoveBitmap drops a bitmap and the space it occupies in the image.
func (m *Monitor) RemoveBitmap(ctx context.Context, nodeName, bitmapName string) error {
	_, err := m.Run(ctx, "block-dirty-bitmap-remove", map[string]interface{}{
		"node": nodeName,
		"name": bitmapName,
	})
	if err != nil {
		return fmt.Errorf("qmp: removing dirty bitmap %s: %w", bitmapName, err)
	}
	return nil
}

// AddBlockdevFile opens a file as a named block node.
//
// Used for the backup target, which has to exist as a node before
// blockdev-backup can name it. The two-layer shape — a file protocol node
// under a format node — is the same one buildDiskArgs emits, because QEMU
// requires it for qcow2.
func (m *Monitor) AddBlockdevFile(ctx context.Context, nodeName, path, format string) error {
	fileNode := nodeName + "-file"
	// QEMU stores a node name in a char[32], so 31 is the limit and anything
	// longer comes back as a bare "Node name too long" naming neither the node
	// nor the bound. The derived file node is the longer of the two, so it is
	// the one that has to fit.
	if len(fileNode) > MaxNodeNameLen {
		return fmt.Errorf("qmp: block node name %q is %d characters; QEMU allows %d",
			fileNode, len(fileNode), MaxNodeNameLen)
	}
	if _, err := m.Run(ctx, "blockdev-add", map[string]interface{}{
		"driver":    "file",
		"node-name": fileNode,
		"filename":  path,
	}); err != nil {
		return fmt.Errorf("qmp: opening %s: %w", path, err)
	}
	if _, err := m.Run(ctx, "blockdev-add", map[string]interface{}{
		"driver":    format,
		"node-name": nodeName,
		"file":      fileNode,
	}); err != nil {
		// Leaving the protocol node behind would hold the file open and make
		// the next attempt fail for a reason that names neither.
		errlog.Log(m.RemoveBlockdev(ctx, fileNode))
		return fmt.Errorf("qmp: opening %s as %s: %w", path, format, err)
	}
	return nil
}

// MaxNodeNameLen is the longest node name QEMU accepts.
//
// BlockDriverState.node_name is a char[32] and the check is `strlen >= 32`,
// so 31 characters are usable.
const MaxNodeNameLen = 31

// RemoveBlockdev closes a node opened with blockdev-add.
func (m *Monitor) RemoveBlockdev(ctx context.Context, nodeName string) error {
	_, err := m.Run(ctx, "blockdev-del", map[string]interface{}{"node-name": nodeName})
	if err != nil {
		return fmt.Errorf("qmp: closing block node %s: %w", nodeName, err)
	}
	return nil
}

// RemoveBlockdevFile closes both nodes AddBlockdevFile opened.
//
// The protocol node was added by its own blockdev-add, so deleting the format
// node above it leaves it open; it has to be deleted by name or the next
// attach under the same name fails as a duplicate.
func (m *Monitor) RemoveBlockdevFile(ctx context.Context, nodeName string) error {
	formatErr := m.RemoveBlockdev(ctx, nodeName)
	fileErr := m.RemoveBlockdev(ctx, nodeName+"-file")
	return errors.Join(formatErr, fileErr)
}

// BlockdevBackup copies a node to a target node and returns the job id.
//
// sync is "full" or "incremental"; an incremental additionally names the
// bitmap whose dirty clusters are to be copied. The caller drives the job to
// completion with AwaitBlockJob, which already exists for commits.
func (m *Monitor) BlockdevBackup(ctx context.Context, jobID, sourceNode, targetNode, sync, bitmapName string) error {
	args := map[string]interface{}{
		"job-id": jobID,
		"device": sourceNode,
		"target": targetNode,
		"sync":   sync,
	}
	if sync == "incremental" {
		if bitmapName == "" {
			return fmt.Errorf("qmp: an incremental backup needs a bitmap")
		}
		args["bitmap"] = bitmapName
		// The bitmap is cleared only once the job succeeds. On failure the
		// dirty range stays, so the next run copies it too rather than
		// silently skipping what this attempt never wrote.
		args["bitmap-mode"] = "on-success"
	}
	if _, err := m.Run(ctx, "blockdev-backup", args); err != nil {
		return fmt.Errorf("qmp: starting the %s backup of %s: %w", sync, sourceNode, err)
	}
	return nil
}
