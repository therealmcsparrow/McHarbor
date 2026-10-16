// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package hostmnt reads the agent's own mount table from a thread proven to be in
// the agent's mount namespace, because /proc/self shows the thread-group leader's.
package hostmnt
