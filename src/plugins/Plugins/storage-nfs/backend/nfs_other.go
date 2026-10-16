// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build !linux

// Non-Linux stub for nfs_client.go. The plugin's mount logic
// uses syscall.Mount which is Linux-only. This stub keeps the
// plugin loadable on Darwin/Windows for development builds
// but every StorageAdapterClient method returns
// ErrUnsupportedPlatform so the host's plugin loader can
// surface a clear error instead of failing to compile.

package main

import (
	"context"
	"errors"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// ErrUnsupportedPlatform is returned by every StorageAdapterClient
// method when the plugin is loaded on a non-Linux host.
var ErrUnsupportedPlatform = errors.New("nfs: storage-nfs requires Linux (syscall.Mount); this host is unsupported")

type nfsClient struct{}

func NewNFSClient() *nfsClient { return &nfsClient{} }

func (c *nfsClient) Test(_ context.Context, _ map[string]string) error {
	return ErrUnsupportedPlatform
}

func (c *nfsClient) TestSteps(_ context.Context, _ map[string]string) ([]plugin.TestStep, error) {
	return nil, ErrUnsupportedPlatform
}

func (c *nfsClient) Upload(_ context.Context, _ map[string]string, _ string, _ interface{}) error {
	return ErrUnsupportedPlatform
}

func (c *nfsClient) Download(_ context.Context, _ map[string]string, _ string) (interface{}, error) {
	return nil, ErrUnsupportedPlatform
}

func (c *nfsClient) Delete(_ context.Context, _ map[string]string, _ string) error {
	return ErrUnsupportedPlatform
}

func (c *nfsClient) List(_ context.Context, _ map[string]string, _ string) ([]plugin.StorageObject, error) {
	return nil, ErrUnsupportedPlatform
}

// Quota mirrors the Linux client's capacity reporter. Nothing is
// mounted on this platform, so there is no filesystem to measure.
func (c *nfsClient) Quota(_ context.Context, _ map[string]string) (plugin.StorageQuota, error) {
	return plugin.StorageQuota{}, ErrUnsupportedPlatform
}
