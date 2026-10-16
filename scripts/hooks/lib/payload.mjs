// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { readFileSync, appendFileSync, mkdirSync, existsSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { spawnSync } from 'node:child_process';

export function readPayload() {
  const raw = readFileSync(0, 'utf8');
  return raw ? JSON.parse(raw) : {};
}

export function editedPath(p) {
  const input = p.tool_input ?? {};
  const file = input.file_path ?? input.path ?? '';
  return file ? resolve(p.cwd ?? process.cwd(), file) : '';
}

// The payload cwd follows the agent's own `cd`, so a hook fired after
// `cd src/frontend` would otherwise scatter session state under
// src/frontend/.results where the Stop gate never looks.
export function hooksDir(p) {
  const start = resolve(p.cwd ?? process.cwd());
  let dir = start;
  for (;;) {
    if (existsSync(join(dir, '.git'))) return join(dir, '.results', 'hooks');
    const parent = dirname(dir);
    if (parent === dir) return join(start, '.results', 'hooks');
    dir = parent;
  }
}

// A subagent's entries carry its agent_id after a tab, so SubagentStop can
// gate that subagent on its own files and never on a sibling's.
export function recordTouched(p, file) {
  const dir = hooksDir(p);
  mkdirSync(dir, { recursive: true });
  const line = p.agent_id ? `${file}\t${p.agent_id}` : file;
  appendFileSync(join(dir, `${p.session_id ?? 'unknown'}.touched`), line + '\n');
}

const MAX_RECORDED_TEXTS = 2000;
const MAX_TEXT_LENGTH = 500;

// Line numbers drift as later edits land, so the Stop gate re-locates the
// changed lines by their text; JSON Lines keeps concurrent subagent appends
// from clobbering each other the way a rewritten JSON object would.
export function recordChangedLines(p, file, changed, text) {
  const dir = hooksDir(p);
  mkdirSync(dir, { recursive: true });
  const { all = false, unknown = false, texts = [] } = changedTexts(p, changed, text);
  const capped = texts.slice(0, MAX_RECORDED_TEXTS).map((t) => t.slice(0, MAX_TEXT_LENGTH));
  const record = { file, all, unknown, texts: capped, agent: p.agent_id ?? null, at: Date.now() };
  appendFileSync(join(dir, `${p.session_id ?? 'unknown'}.lines.jsonl`), JSON.stringify(record) + '\n');
}

function stripCr(line) {
  return line.replace(/\r$/, '');
}

// Order-insensitive line diff: a new line counts as added unless the old text
// still has an unused copy of it. Quadratic LCS is not affordable on large
// files inside a 30 s hook, and a moved line is not a changed one anyway.
export function addedLines(oldText, newText) {
  const pool = new Map();
  for (const line of oldText.split('\n').map(stripCr)) pool.set(line, (pool.get(line) ?? 0) + 1);
  const added = [];
  for (const line of newText.split('\n').map(stripCr)) {
    const left = pool.get(line) ?? 0;
    if (left > 0) pool.set(line, left - 1);
    else if (line.trim()) added.push(line);
  }
  return added;
}

function patchAdditions(patch) {
  const out = [];
  for (const hunk of patch) {
    for (const line of hunk.lines ?? []) if (line.startsWith('+')) out.push(stripCr(line.slice(1)));
  }
  return out;
}

// all:true is reserved for a file this call created: one such record makes
// every line of the file the session's responsibility for the rest of the
// session, so a Write over an existing file or an Edit that could not be
// located records its real additions, or `unknown`, never all.
export function changedTexts(p, changed, postText) {
  const resp = p.tool_response ?? {};
  const patch = Array.isArray(resp.structuredPatch) ? resp.structuredPatch : null;
  if (p.tool_name === 'Write') {
    if (resp.type === 'create') return { all: true };
    if (typeof resp.originalFile === 'string') return { texts: addedLines(resp.originalFile, postText) };
    if (patch && patch.length) return { texts: patchAdditions(patch) };
    return { unknown: true };
  }
  if (changed instanceof Set) {
    const lines = postText.split('\n');
    return { texts: [...changed].sort((a, b) => a - b).map((n) => stripCr(lines[n - 1] ?? '')) };
  }
  if (patch && patch.length) return { texts: patchAdditions(patch) };
  return { unknown: true };
}

export function emitContext(event, text) {
  if (!text) return;
  process.stdout.write(JSON.stringify({ hookSpecificOutput: { hookEventName: event, additionalContext: text } }));
}

// A rule module receives changedLines and decides per-finding whether the
// line it matched is one the tool call actually touched.
export function inChanged(changedLines, line) {
  return changedLines === 'all' || changedLines.has(line);
}

// Edit/MultiEdit report the edited region as new_string, which is not a line
// range — so it is located in the post-edit text instead. A snippet that
// cannot be found (edited again by something else before this hook ran, or a
// multi-line snippet that doesn't align) falls back to 'all' rather than
// silently reporting nothing.
export function changedLines(p, postEditText) {
  const name = p.tool_name;
  if (name === 'Write') return 'all';

  const input = p.tool_input ?? {};
  const lines = postEditText.split('\n');
  const found = new Set();

  const locate = (newString, replaceAll) => {
    if (typeof newString !== 'string' || newString === '') return false;
    const snippet = newString.split('\n');
    let any = false;
    for (let i = 0; i < lines.length; i++) {
      if (!lines[i].includes(snippet[0])) continue;
      let ok = true;
      for (let j = 1; j < snippet.length; j++) {
        if (lines[i + j] !== snippet[j]) {
          ok = false;
          break;
        }
      }
      if (!ok) continue;
      for (let j = 0; j < snippet.length; j++) found.add(i + j + 1);
      any = true;
      if (!replaceAll) break;
    }
    return any;
  };

  let matched = false;
  if (name === 'MultiEdit') {
    for (const edit of input.edits ?? []) {
      if (locate(edit.new_string, edit.replace_all)) matched = true;
    }
  } else {
    matched = locate(input.new_string, input.replace_all);
  }

  return matched ? found : 'all';
}

// gofmt/prettier can rewrite whitespace or quoting on the very lines an edit
// just wrote, so new_string sometimes no longer appears verbatim in the
// post-format text. Falling straight back to 'all' there would over-report
// on every reformatted edit, so this re-locates new_string in the pre-format
// text instead and reuses those line numbers — valid whenever formatting
// left the line count unchanged, which covers reindentation and quote-style
// changes (gofmt, and prettier's common case). A line count change (rare —
// prettier reflowing a long line, or gofmt regrouping imports) still falls
// back to 'all' rather than mapping the wrong lines.
export function changedLinesAcrossFormat(p, preText, postText) {
  const direct = changedLines(p, postText);
  if (direct !== 'all') return direct;
  if (preText.split('\n').length !== postText.split('\n').length) return 'all';
  return changedLines(p, preText);
}

// Write's tool_response carries type "create"/"update" in current transcripts;
// older or replayed payloads may lack it, so git's view of the working tree is
// the fallback for "did this file exist before this call".
export function isNewFile(p) {
  const resp = p.tool_response ?? {};
  if (resp.type === 'create') return true;
  if (resp.type === 'update') return false;

  const file = editedPath(p);
  if (!file) return false;
  try {
    const result = spawnSync('git', ['ls-files', '--error-unmatch', file], {
      cwd: p.cwd ?? process.cwd(),
      timeout: 5_000,
    });
    return result.status !== 0;
  } catch {
    return false;
  }
}
