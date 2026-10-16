// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build !linux

package networks

// newNativeHostLinker is nil off Linux: macvlan host links exist only on a Linux Docker host.
func newNativeHostLinker() *nativeHostLinker { return nil }
