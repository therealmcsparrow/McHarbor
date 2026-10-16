// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { readdirSync, statSync, rmSync } from 'node:fs';
import { join } from 'node:path';

export const HOOK_STATE_TTL_MS = 30 * 24 * 60 * 60_000;
export const HOOK_PRUNE_MAX_REMOVALS = 200;

function mtime(path) {
  try {
    return statSync(path).mtimeMs;
  } catch {
    return 0;
  }
}

// Removes files older than ttlMs under dir (recursing when asked), stopping after
// maxRemovals so a long-neglected directory is cleared over several runs, never in
// one blocking pass. Returns how many entries it removed.
export function pruneOlderThan(dir, { now = Date.now(), ttlMs = HOOK_STATE_TTL_MS, recurse = false, maxRemovals = HOOK_PRUNE_MAX_REMOVALS, match = () => true } = {}) {
  let removed = 0;
  const walk = (current) => {
    let entries;
    try {
      entries = readdirSync(current, { withFileTypes: true });
    } catch {
      return;
    }
    for (const entry of entries) {
      if (removed >= maxRemovals) return;
      const full = join(current, entry.name);
      if (entry.isDirectory()) {
        if (recurse) walk(full);
        continue;
      }
      if (match(entry.name) && now - mtime(full) > ttlMs) {
        rmSync(full, { force: true });
        removed++;
      }
    }
  };
  walk(dir);
  return removed;
}

const SESSION_FILE = /\.(?:touched|lines\.jsonl|shell\.jsonl|gate\.md|gate-state\.json|start|start-status\.json)$/;

// Prunes a session's leftovers from .results/hooks: the per-session state files and
// the sanity-* scratch directories, each judged by its own mtime. manual-base has
// its own prune and manual-gate.* are single files that get overwritten in place.
export function pruneHookState(hooksDir, { now = Date.now(), ttlMs = HOOK_STATE_TTL_MS, maxRemovals = HOOK_PRUNE_MAX_REMOVALS, keepPrefix = null } = {}) {
  let removed = pruneOlderThan(hooksDir, {
    now,
    ttlMs,
    maxRemovals,
    match: (name) => SESSION_FILE.test(name) && !(keepPrefix && name.startsWith(keepPrefix)),
  });
  let entries = [];
  try {
    entries = readdirSync(hooksDir, { withFileTypes: true });
  } catch {
    return removed;
  }
  for (const entry of entries) {
    if (removed >= maxRemovals) break;
    if (!entry.isDirectory() || !entry.name.startsWith('sanity-')) continue;
    const full = join(hooksDir, entry.name);
    if (now - mtime(full) > ttlMs) {
      rmSync(full, { recursive: true, force: true });
      removed++;
    }
  }
  return removed;
}
