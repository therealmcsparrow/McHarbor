// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { readFileSync, existsSync } from 'node:fs';
import { dirname, join, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';
import {
  readPayload,
  editedPath,
  recordTouched,
  recordChangedLines,
  emitContext,
  changedLinesAcrossFormat,
  isNewFile,
} from './lib/payload.mjs';
import { formatFile } from './format-edited.mjs';
import { check as checkCopyright } from './rules/copyright.mjs';
import { check as checkGo } from './rules/go-patterns.mjs';
import { check as checkTs, keyUnusedFindings, rootFor } from './rules/ts-patterns.mjs';
import { check as checkLocale } from './rules/locale-json.mjs';

const HERE = dirname(fileURLToPath(import.meta.url));
const REPO = resolve(HERE, '..', '..');

// Extensions the copyright header rule (CLAUDE.md § Copyright header) applies
// to; JSON and Markdown carry no header at all.
const HEADER_EXTS = /\.(?:go|ts|tsx|mjs|cjs|js|jsx|css|sql|sh|ya?ml)$/;

// Budgeted alongside format-edited.mjs's own gofmt/prettier timeouts, well
// under the 30s PostToolUse timeout.
const VERIFY_KEYS_TIMEOUT_MS = 10_000;

function isLocaleJson(path) {
  return /[\\/](?:locales|i18n)[\\/][^\\/]+[\\/][^\\/]+\.json$/.test(path);
}

function toRelative(file) {
  return file.startsWith(REPO + sep) ? file.slice(REPO.length + 1).split(sep).join('/') : file;
}

function runVerifyKeys(payload, file) {
  if (!/\.tsx?$/.test(file)) return '';
  const verifier = join(REPO, 'scripts', 'sanity', 'i18n', 'verify-keys.mjs');
  if (!existsSync(verifier)) return '';
  const result = spawnSync(process.execPath, [verifier, '--hook'], {
    input: JSON.stringify(payload),
    cwd: REPO,
    encoding: 'utf8',
    timeout: VERIFY_KEYS_TIMEOUT_MS,
  });
  return (result.stderr ?? '').trim();
}

// replacedText is what this call took out of the file: the old_string(s) of an Edit, or
// the whole original of a Write over an existing file.
function replacedText(payload) {
  const input = payload.tool_input ?? {};
  if (payload.tool_name === 'Edit') return typeof input.old_string === 'string' ? input.old_string : '';
  if (payload.tool_name === 'MultiEdit') return (input.edits ?? []).map((e) => e.old_string ?? '').join('\n');
  if (payload.tool_name === 'Write') return payload.tool_response?.originalFile ?? '';
  return '';
}

export function run(payload) {
  const file = editedPath(payload);
  if (!file) return '';

  recordTouched(payload, file);

  if (!existsSync(file)) return '';

  const preText = readFileSync(file, 'utf8');

  // Claude Code runs every hook matching a PostToolUse event in parallel, so
  // a separate format hook's position in settings.json is not a sequencing
  // guarantee — only running the formatter from inside this process, before
  // anything below reads the file, is. See lib/payload.mjs's
  // changedLinesAcrossFormat for why the post-format text still needs the
  // pre-format text kept around.
  try {
    formatFile(file);
  } catch {
    // Formatting must never block linting.
  }

  const text = existsSync(file) ? readFileSync(file, 'utf8') : preText;
  const changed = changedLinesAcrossFormat(payload, preText, text);
  try {
    recordChangedLines(payload, file, changed, text);
  } catch {
    // The Stop gate degrades to warn-only for this file; linting goes on.
  }
  const findings = [];

  if (payload.tool_name === 'Write' && HEADER_EXTS.test(file) && isNewFile(payload)) {
    findings.push(...checkCopyright({ path: file, text, changedLines: changed }));
  }

  if (/\.go$/.test(file)) {
    findings.push(...checkGo({ path: file, text, changedLines: changed }));
  } else if (/\.tsx?$/.test(file)) {
    findings.push(...checkTs({ path: file, text, changedLines: changed }));
    const before = replacedText(payload);
    if (before) findings.push(...keyUnusedFindings(rootFor(file), before, text));
  } else if (/\.json$/.test(file) && isLocaleJson(file)) {
    findings.push(...checkLocale({ path: file, text, changedLines: changed }));
  }

  const relPath = toRelative(file);
  const lines = findings.map((f) => `${relPath}:${f.line} [${f.rule}] ${f.message}`);

  const verifyOut = runVerifyKeys(payload, file);
  if (verifyOut) lines.push(...verifyOut.split('\n'));

  return lines.slice(0, 40).join('\n');
}

function main() {
  let payload;
  try {
    payload = readPayload();
  } catch {
    return;
  }

  let text = '';
  try {
    text = run(payload);
  } catch {
    // A hook must never block the edit that already happened.
    return;
  }

  emitContext('PostToolUse', text);
}

// Guarded so tests can import { run } without triggering a stdin read.
const isMain = process.argv[1] && resolve(process.argv[1]) === resolve(fileURLToPath(import.meta.url));
if (isMain) main();
