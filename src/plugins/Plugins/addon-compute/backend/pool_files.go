// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
)

// ErrPoolBrowseUnavailable means no agent bridge is wired, so there is no host to
// browse. The operator is told so instead of being shown a browser that answers nothing.
var ErrPoolBrowseUnavailable = errors.New("this McHarbor does not provide the agent bridge")

// ErrPoolNotBrowsable means the pool's backend has no directory tree.
var ErrPoolNotBrowsable = errors.New("this pool's backend stores volumes as block devices, so it has no files to browse")

// poolFiles returns the agent bridge, or ErrPoolBrowseUnavailable without one.
func (s *Service) poolFiles() (*agentBridge, error) {
	if s.rpc == nil {
		return nil, ErrPoolBrowseUnavailable
	}
	return s.rpc, nil
}

// BrowsePool lists one directory inside a pool.
//
// The pool is read from the database rather than taken from the request, so a
// caller cannot name a target of their choosing and have the agent open it.
func (s *Service) BrowsePool(ctx context.Context, poolID, dir string) ([]PoolFile, error) {
	files, err := s.poolFiles()
	if err != nil {
		return nil, err
	}
	pool, err := s.GetPool(ctx, poolID)
	if err != nil {
		return nil, err
	}
	if pool.Backend == BackendLVM {
		return nil, ErrPoolNotBrowsable
	}
	entries, err := files.PoolBrowse(ctx, pool.EnvironmentID, toPoolRef(pool), dir)
	if err != nil {
		return nil, fmt.Errorf("browsing pool %s: %w", poolID, err)
	}
	return entries, nil
}

// PoolFileOperation performs one read, write, mkdir, delete or rename in a
// pool and returns the bytes a read produced.
func (s *Service) PoolFileOperation(ctx context.Context, poolID string, req PoolFileRequest) ([]byte, error) {
	files, err := s.poolFiles()
	if err != nil {
		return nil, err
	}
	pool, err := s.GetPool(ctx, poolID)
	if err != nil {
		return nil, err
	}
	if pool.Backend == BackendLVM {
		return nil, ErrPoolNotBrowsable
	}
	content, err := files.PoolFile(ctx, pool.EnvironmentID, toPoolRef(pool), req)
	if err != nil {
		return nil, fmt.Errorf("pool %s file %s: %w", poolID, req.Operation, err)
	}
	return content, nil
}

// toPoolRef narrows a stored pool to the four fields the agent needs to find
// it. Nothing else crosses: the row also carries capacity samples and an error
// string, and neither is the agent's business.
func toPoolRef(pool Pool) PoolRef {
	return PoolRef{
		ID:      pool.ID,
		Name:    pool.Name,
		Backend: pool.Backend,
		Target:  pool.Target,
	}
}
