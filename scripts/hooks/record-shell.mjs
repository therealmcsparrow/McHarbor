// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { appendFileSync, mkdirSync, readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const HERE = dirname(fileURLToPath(import.meta.url));
const REPO = resolve(HERE, '..', '..');

// The Stop gate attributes a Bash/PowerShell-edited file to this session only
// when its mtime falls inside one of these windows; without them a user's IDE
// edit would be gated as if the agent had made it.
export function recordShell(payload, { root = REPO, now = Date.now() } = {}) {
  if (!payload.session_id) return;
  const event = payload.hook_event_name;
  const phase = event === 'PreToolUse' ? 'start' : event === 'PostToolUse' || event === 'PostToolUseFailure' ? 'end' : null;
  if (!phase) return;
  const dir = join(root, '.results', 'hooks');
  mkdirSync(dir, { recursive: true });
  const record = {
    id: payload.tool_use_id ?? null,
    phase,
    at: now,
    duration: typeof payload.duration_ms === 'number' ? payload.duration_ms : null,
    agent: payload.agent_id ?? null,
  };
  appendFileSync(join(dir, `${payload.session_id}.shell.jsonl`), JSON.stringify(record) + '\n');
}

function main() {
  try {
    const raw = readFileSync(0, 'utf8');
    recordShell(raw ? JSON.parse(raw) : {}, { root: process.env.MCHARBOR_GATE_ROOT || REPO });
  } catch {
    // A recorder must never interfere with the shell call it observes.
  }
}

const isMain = process.argv[1] && resolve(process.argv[1]) === resolve(fileURLToPath(import.meta.url));
if (isMain) main();
