// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build !linux

package main

// Compute is Linux-host-only, so a non-Linux agent is never re-executed as a
// container init. The branch in main() still compiles — it is unconditional
// there by design, so nobody can remove it from the Linux path by accident —
// and simply never fires.

func isContainerInitInvocation(_ []string) bool { return false }

func runContainerInit() {}
