// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package qmp

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// The verbs a live migration without shared storage needs: TLS credentials,
// an NBD server on the target, a mirror into it from the source, and the
// incoming side of the RAM transfer. Kept apart from commands.go because none
// of them is used anywhere else, and every one of them opens a network path.

// AddTLSCreds registers an x509 credential object that QEMU loads from dir.
//
// dir holds ca-cert.pem plus server-* or client-* files, as QEMU names them.
// verify-peer is always on: a migration channel that accepted any client
// would let anyone on the network feed a guest's memory image.
func (m *Monitor) AddTLSCreds(ctx context.Context, id, dir, endpoint string) error {
	if endpoint != "server" && endpoint != "client" {
		return fmt.Errorf("qmp: %q is not a TLS endpoint", endpoint)
	}
	_, err := m.Run(ctx, "object-add", map[string]interface{}{
		"qom-type":    "tls-creds-x509",
		"id":          id,
		"dir":         dir,
		"endpoint":    endpoint,
		"verify-peer": true,
	})
	return err
}

// DelObject removes a QOM object such as a TLS credential.
func (m *Monitor) DelObject(ctx context.Context, id string) error {
	_, err := m.Run(ctx, "object-del", map[string]interface{}{"id": id})
	return err
}

// StartNBDServer opens the NBD server a live target receives mirrored disks on,
// always behind the given TLS credential.
func (m *Monitor) StartNBDServer(ctx context.Context, host string, port int, tlsCreds string) error {
	if tlsCreds == "" {
		return errors.New("qmp: refusing to start an NBD server without TLS")
	}
	_, err := m.Run(ctx, "nbd-server-start", map[string]interface{}{
		"addr": map[string]interface{}{
			"type": "inet",
			"data": map[string]interface{}{"host": host, "port": strconv.Itoa(port)},
		},
		"tls-creds": tlsCreds,
	})
	return err
}

// StopNBDServer closes the NBD server and every export on it.
func (m *Monitor) StopNBDServer(ctx context.Context) error {
	_, err := m.Run(ctx, "nbd-server-stop", nil)
	return err
}

// ExportNBD publishes a block node writable on the running NBD server.
//
// block-export-add replaced nbd-server-add in QEMU 5.2; the older verb is the
// fallback so a host on an LTS QEMU can still be a migration target.
func (m *Monitor) ExportNBD(ctx context.Context, node, name string) error {
	if m.Supports(ctx, "block-export-add") {
		_, err := m.Run(ctx, "block-export-add", map[string]interface{}{
			"type":      "nbd",
			"id":        name,
			"node-name": node,
			"name":      name,
			"writable":  true,
		})
		return err
	}
	_, err := m.Run(ctx, "nbd-server-add", map[string]interface{}{
		"device":   node,
		"name":     name,
		"writable": true,
	})
	return err
}

// AddNBDBlockdev opens a peer's NBD export as a local block node, over TLS.
func (m *Monitor) AddNBDBlockdev(ctx context.Context, node, host string, port int, export, tlsCreds string) error {
	if tlsCreds == "" {
		return errors.New("qmp: refusing to reach an NBD export without TLS")
	}
	_, err := m.Run(ctx, "blockdev-add", map[string]interface{}{
		"driver":    "nbd",
		"node-name": node,
		"server": map[string]interface{}{
			"type": "inet",
			"host": host,
			"port": strconv.Itoa(port),
		},
		"export":    export,
		"tls-creds": tlsCreds,
	})
	return err
}

// Mirror starts a full mirror of device into target, driven as job jobID.
func (m *Monitor) Mirror(ctx context.Context, jobID, device, target string) error {
	_, err := m.Run(ctx, "blockdev-mirror", map[string]interface{}{
		"job-id": jobID,
		"device": device,
		"target": target,
		"sync":   "full",
	})
	return err
}

// BlockJobCancel stops a job. On a mirror in the ready state this is the
// clean end: the source stops writing to the target without pivoting onto it.
func (m *Monitor) BlockJobCancel(ctx context.Context, jobID string) error {
	_, err := m.Run(ctx, "block-job-cancel", map[string]interface{}{"device": jobID})
	return err
}

// AwaitJobsReady waits until every named job reports ready, failing as soon as
// one of them fails. A mirror is ready once the target has caught up and QEMU
// is copying new writes as they happen.
func (m *Monitor) AwaitJobsReady(ctx context.Context, jobIDs []string, onProgress func(done, total int64)) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
		jobs, err := m.BlockJobs(ctx)
		if err != nil {
			return fmt.Errorf("qmp: polling the mirrors: %w", err)
		}
		byID := make(map[string]BlockJob, len(jobs))
		for _, job := range jobs {
			byID[job.Device] = job
		}
		ready := 0
		var done, total int64
		for _, id := range jobIDs {
			job, ok := byID[id]
			if !ok {
				return fmt.Errorf("qmp: mirror %s ended before it was ready", id)
			}
			if job.Error != "" {
				return fmt.Errorf("qmp: mirror %s failed: %s", id, job.Error)
			}
			done += job.Offset
			total += job.Len
			if job.Status == "ready" {
				ready++
			}
		}
		if onProgress != nil {
			onProgress(done, total)
		}
		if ready == len(jobIDs) {
			return nil
		}
	}
}

// SetMigrateTLS makes the next migration use the named TLS credential.
func (m *Monitor) SetMigrateTLS(ctx context.Context, tlsCreds string) error {
	_, err := m.Run(ctx, "migrate-set-parameters", map[string]interface{}{"tls-creds": tlsCreds})
	return err
}

// MigrateIncoming starts listening for a migration on a guest started with
// `-incoming defer`.
func (m *Monitor) MigrateIncoming(ctx context.Context, uri string) error {
	_, err := m.Run(ctx, "migrate-incoming", map[string]interface{}{"uri": uri})
	return err
}

// AwaitMigration polls an outbound migration until it completes, fails or is
// cancelled, reporting its progress on the way.
func (m *Monitor) AwaitMigration(ctx context.Context, onProgress func(MigrateStatus)) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
		status, err := m.MigrateStatus(ctx)
		if err != nil {
			return fmt.Errorf("qmp: polling the migration: %w", err)
		}
		if onProgress != nil {
			onProgress(status)
		}
		switch status.Status {
		case "completed":
			return nil
		case "failed":
			return errors.New("qmp: the migration failed")
		case "cancelled":
			return errors.New("qmp: the migration was cancelled")
		}
	}
}
