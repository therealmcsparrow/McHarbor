// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux && !seccomp

package main

// seccompBuiltIn is false: this binary was compiled without libcontainer's
// seccomp support, so containers run with no syscall filter. See the
// counterpart in compute_seccomp_on.go — the capability probe surfaces this so
// the posture is visible rather than assumed.
const seccompBuiltIn = false
