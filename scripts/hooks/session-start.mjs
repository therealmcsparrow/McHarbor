// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { mkdirSync, writeFileSync, existsSync, rmSync, readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';
import { pruneHookState } from './lib/prune.mjs';

const HERE = dirname(fileURLToPath(import.meta.url));
const REPO = resolve(HERE, '..', '..');

const GIT_TIMEOUT_MS = 8_000;

export function defaultGit(root) {
  return (args) => {
    const result = spawnSync('git', args, { cwd: root, encoding: 'utf8', timeout: GIT_TIMEOUT_MS, maxBuffer: 64 * 1024 * 1024 });
    return { ok: result.status === 0, stdout: result.stdout ?? '' };
  };
}

// Porcelain -z puts a rename's origin path in the field after the entry, with
// no status prefix; both sides count as dirty.
export function parsePorcelainZ(stdout) {
  const fields = stdout.split('\0');
  const dirty = [];
  for (let i = 0; i < fields.length; i++) {
    const entry = fields[i];
    if (entry.length < 4) continue;
    const xy = entry.slice(0, 2);
    dirty.push(entry.slice(3));
    if (/[RC]/.test(xy) && i + 1 < fields.length && fields[i + 1]) {
      dirty.push(fields[i + 1]);
      i++;
    }
  }
  return dirty;
}

// Resume, compact and fork keep the session's touched set: truncating there
// would forget edits the Stop gate still owes a verdict on. A subagent never
// owns the session's state.
export function startSession(payload, { root = REPO, git = defaultGit(root), now = Date.now() } = {}) {
  if (payload.agent_id || !payload.session_id) return;
  const session = payload.session_id;
  const dir = join(root, '.results', 'hooks');
  mkdirSync(dir, { recursive: true });
  try {
    pruneHookState(dir, { now, keepPrefix: `${session}.` });
  } catch {
    // Housekeeping must never stop the session from starting.
  }
  const startFile = join(dir, `${session}.start`);
  const fresh = payload.source === undefined || payload.source === 'startup' || payload.source === 'clear';
  if (!fresh && existsSync(startFile)) return;

  writeFileSync(startFile, String(now));
  writeFileSync(join(dir, `${session}.touched`), '');
  for (const stale of ['lines.jsonl', 'gate-state.json', 'gate.md']) {
    rmSync(join(dir, `${session}.${stale}`), { force: true });
  }

  let status = { ok: false, dirty: [] };
  try {
    const result = git(['status', '--porcelain=v1', '-z', '-uall']);
    if (result.ok) status = { ok: true, dirty: parsePorcelainZ(result.stdout) };
  } catch {
    // The gate treats a missing snapshot as "every Bash edit is warn-only".
  }
  writeFileSync(join(dir, `${session}.start-status.json`), JSON.stringify(status));
}

function main() {
  let payload = {};
  try {
    const raw = readFileSync(0, 'utf8');
    payload = raw ? JSON.parse(raw) : {};
  } catch {
    return;
  }
  try {
    startSession(payload, { root: process.env.MCHARBOR_GATE_ROOT || REPO });
  } catch {
    // A SessionStart hook must never stop the session from starting.
  }
}

const isMain = process.argv[1] && resolve(process.argv[1]) === resolve(fileURLToPath(import.meta.url));
if (isMain) main();
