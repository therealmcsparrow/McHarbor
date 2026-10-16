// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build seccomp

package main

// seccompBuiltIn records that this binary was compiled with libcontainer's
// seccomp support.
//
// The distinction matters and must never be invisible. Built WITHOUT the
// seccomp tag, libcontainer applies no syscall filter at all — user namespace
// isolation still applies, so containers are not unprotected, but the syscall
// restriction operators reasonably assume is there simply is not. The
// capability probe reports this value and the node capability panel shows it.
const seccompBuiltIn = true
