// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, rmSync, readFileSync, existsSync, utimesSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

import {
  collectTouched,
  resolveChangedLines,
  runChecks,
  classify,
  missingReplyLines,
  decide,
  gate,
  parseGofmtDiff,
  parseGoVet,
  parseTsc,
  parseSanityReport,
  shellWindows,
} from '../gate-touched.mjs';
import { recordShell } from '../record-shell.mjs';
import { startSession } from '../session-start.mjs';

const HERE = fileURLToPath(new URL('.', import.meta.url));
const REPO = join(HERE, '..', '..', '..');
const GATE_SCRIPT = join(REPO, 'scripts', 'hooks', 'gate-touched.mjs');

function tempRoot() {
  const root = mkdtempSync(join(tmpdir(), 'gate-touched-'));
  mkdirSync(join(root, '.results', 'hooks'), { recursive: true });
  return root;
}

function put(root, rel, text, mtimeMs) {
  const file = join(root, ...rel.split('/'));
  mkdirSync(join(file, '..'), { recursive: true });
  writeFileSync(file, text);
  if (mtimeMs !== undefined) utimesSync(file, mtimeMs / 1000, mtimeMs / 1000);
  return file;
}

function hooks(root, name) {
  return join(root, '.results', 'hooks', name);
}

// Every runner call answers "tool missing" unless the test overrides it.
function fakeRunner(overrides = {}) {
  const calls = [];
  const runner = async (spec) => {
    calls.push(spec);
    const key = Object.keys(overrides).find((k) => spec.label === k);
    if (key) return { code: 0, stdout: '', stderr: '', missing: false, timedOut: false, ...overrides[key](spec) };
    return { code: null, stdout: '', stderr: '', missing: true, timedOut: false };
  };
  return { runner, calls };
}

const noGit = () => ({ ok: false, stdout: '' });

/* ─── collectTouched ─────────────────────────────────────────────── */

test('collectTouched: .touched list ∪ src/widgets/nodes/scripts files with mtime ≥ .start', () => {
  const root = tempRoot();
  try {
    const start = Date.now() - 60_000;
    const listed = put(root, 'docs/listed.md', 'x\n', start - 120_000);
    const bashEdited = put(root, 'src/backend/modules/x/service.go', 'package x\n', start + 1_000);
    const widgetEdited = put(root, 'widgets/foo/Foo.tsx', 'x\n', start + 1_000);
    put(root, 'src/backend/modules/x/old.go', 'package x\n', start - 120_000);
    put(root, 'src/frontend/node_modules/pkg/index.ts', 'x\n', start + 1_000);
    put(root, 'src/plugins/Plugins/addon-x/frontend/dist/addon-x.umd.cjs', 'x\n', start + 1_000);
    put(root, 'other/elsewhere.go', 'package y\n', start + 1_000);
    writeFileSync(hooks(root, 's.touched'), `${listed}\n${listed}\tagent-1\nP:\\outside\\repo.go\n`);

    const { files } = collectTouched({ root, sessionId: 's', startMs: start });
    assert.deepEqual(files.sort(), [bashEdited, listed, widgetEdited].sort());
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('collectTouched: with an agentId only that subagent\'s tagged entries count, and no mtime scan', () => {
  const root = tempRoot();
  try {
    const start = Date.now() - 60_000;
    const mine = put(root, 'src/a.go', 'package a\n');
    const sibling = put(root, 'src/b.go', 'package b\n');
    put(root, 'src/c.go', 'package c\n', start + 1_000);
    writeFileSync(hooks(root, 's.touched'), `${mine}\tagent-1\n${sibling}\tagent-2\n${sibling}\n`);

    const { files } = collectTouched({ root, sessionId: 's', agentId: 'agent-1', startMs: start });
    assert.deepEqual(files, [mine]);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

/* ─── resolveChangedLines ────────────────────────────────────────── */

test('resolveChangedLines: clean-at-start tracked file uses git diff -U0 HEAD hunks', () => {
  const root = tempRoot();
  try {
    const file = put(root, 'src/a.go', 'l1\nl2\nl3\nl4\n');
    const git = (args) => {
      if (args[0] === 'ls-files') return { ok: true, stdout: 'src/a.go\0' };
      if (args[0] === 'diff') {
        return { ok: true, stdout: 'diff --git a/src/a.go b/src/a.go\n--- a/src/a.go\n+++ b/src/a.go\n@@ -2,0 +3,2 @@\n+l3\n+l4\n' };
      }
      return { ok: false, stdout: '' };
    };
    const changed = resolveChangedLines({
      root,
      files: [file],
      startMs: Date.now() - 1_000,
      startStatus: { ok: true, dirty: [] },
      records: [],
      git,
    });
    assert.deepEqual([...changed.get(file)].sort(), [3, 4]);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('resolveChangedLines: dirty-at-start file with no Edit/Write record is warn-only; with one it is located by text', () => {
  const root = tempRoot();
  try {
    const bashOnly = put(root, 'src/a.go', 'package a\nfunc A() {}\n');
    const edited = put(root, 'src/b.go', 'package b\n\nfunc B() { doThing() }\n');
    const changed = resolveChangedLines({
      root,
      files: [bashOnly, edited],
      startMs: Date.now() - 1_000,
      startStatus: { ok: true, dirty: ['src/a.go', 'src/b.go'] },
      records: [{ file: edited, all: false, texts: ['func B() { doThing() }'], agent: null }],
      git: noGit,
    });
    assert.equal(changed.get(bashOnly), 'warn-only');
    assert.deepEqual([...changed.get(edited)], [3]);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('resolveChangedLines: an untracked file created after session start is "all"', () => {
  const root = tempRoot();
  try {
    const file = put(root, 'src/new.go', 'package n\n');
    const changed = resolveChangedLines({
      root,
      files: [file],
      startMs: Date.now() - 60_000,
      startStatus: { ok: true, dirty: [] },
      records: [],
      git: (args) => (args[0] === 'ls-files' ? { ok: true, stdout: '' } : { ok: true, stdout: '' }),
    });
    assert.equal(changed.get(file), 'all');
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

/* ─── runChecks (injectable runner) ──────────────────────────────── */

test('runChecks: a missing golangci-lint is reported as "skipped (tool missing)", never a finding', async () => {
  const root = tempRoot();
  try {
    put(root, 'src/backend/go.mod', 'module x\n');
    const file = put(root, 'src/backend/modules/a/service.go', 'package a\n');
    const { runner } = fakeRunner({ gofmt: () => ({ code: 0 }), 'go vet': () => ({ code: 0 }) });
    const result = await runChecks({ root, files: [file], runner, deadline: Date.now() + 10_000, sessionId: 's' });
    assert.ok(result.skipped.some((s) => /golangci-lint.*skipped \(tool missing\)/.test(s)), result.skipped.join('\n'));
    assert.equal(result.findings.length, 0);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('runChecks: a build-tag-gated module (scripts/lint/ruleguard) skips go vet / golangci-lint', async () => {
  const root = tempRoot();
  try {
    put(root, 'scripts/lint/ruleguard/go.mod', 'module gorules\n\ngo 1.27.1\n');
    const file = put(root, 'scripts/lint/ruleguard/rules.go', '//go:build ruleguard\n\npackage gorules\n');
    const { runner, calls } = fakeRunner({ gofmt: () => ({ code: 0 }) });
    const result = await runChecks({ root, files: [file], runner, deadline: Date.now() + 10_000, sessionId: 's' });
    const ruleguardCalls = calls.filter((c) => c.cwd === join(root, 'scripts', 'lint', 'ruleguard'));
    assert.equal(ruleguardCalls.length, 0, `expected no go vet / golangci-lint call for the module, got: ${JSON.stringify(ruleguardCalls)}`);
    assert.ok(result.notes.some((n) => /scripts\/lint\/ruleguard/.test(n) && /go:build/.test(n)), result.notes.join('\n'));
    assert.ok(!result.findings.some((f) => /build constraints exclude/.test(f.message)));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('runChecks: gofmt -d output becomes a line-level finding on the touched file', async () => {
  const root = tempRoot();
  try {
    put(root, 'src/backend/go.mod', 'module x\n');
    const file = put(root, 'src/backend/modules/a/service.go', 'package a\n\nfunc a() {\n  x := 1\n}\n');
    const fwd = file.replace(/\\/g, '/');
    const diff = `diff ${fwd}.orig ${fwd}\n--- ${fwd}.orig\n+++ ${fwd}\n@@ -1,5 +1,5 @@\n package a\n \n func a() {\n-  x := 1\n+\tx := 1\n }\n`;
    const { runner } = fakeRunner({ gofmt: () => ({ code: 1, stdout: diff }), 'go vet': () => ({ code: 0 }) });
    const result = await runChecks({ root, files: [file], runner, deadline: Date.now() + 10_000, sessionId: 's' });
    const f = result.findings.find((x) => x.tool === 'gofmt');
    assert.ok(f, 'expected a gofmt finding');
    assert.equal(f.line, 4);
    assert.equal(f.file, file);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('runChecks: the in-process T1 rules run on Bash-edited files too', async () => {
  const root = tempRoot();
  try {
    put(root, 'src/backend/go.mod', 'module x\n');
    const file = put(root, 'src/backend/modules/a/handler.go', 'package a\n\nfunc h() { respond(err.Error()) }\n');
    const { runner } = fakeRunner({ gofmt: () => ({ code: 0 }), 'go vet': () => ({ code: 0 }) });
    const result = await runChecks({ root, files: [file], runner, deadline: Date.now() + 10_000, sessionId: 's' });
    const f = result.findings.find((x) => x.rule === 'go/raw-err-to-client');
    assert.ok(f, 'expected go/raw-err-to-client');
    assert.equal(f.line, 3);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('runChecks: a plugin frontend source newer than its umd bundle is a finding', async () => {
  const root = tempRoot();
  try {
    const now = Date.now();
    put(root, 'src/plugins/Plugins/addon-x/frontend/dist/addon-x.umd.cjs', 'x\n', now - 60_000);
    const src = put(root, 'src/plugins/Plugins/addon-x/frontend/src/Page.tsx', 'x\n', now);
    const { runner } = fakeRunner();
    const result = await runChecks({ root, files: [src], runner, deadline: Date.now() + 10_000, sessionId: 's' });
    assert.ok(result.findings.some((f) => /plugin\/addon-x: frontend not rebuilt/.test(f.message) && f.fileLevel));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('runChecks: a plugin backend change adds the four-way version note', async () => {
  const root = tempRoot();
  try {
    put(root, 'src/plugins/Plugins/addon-x/backend/go.mod', 'module x\n');
    const file = put(root, 'src/plugins/Plugins/addon-x/backend/main.go', 'package main\n');
    const { runner } = fakeRunner({ gofmt: () => ({ code: 0 }), 'go vet': () => ({ code: 0 }) });
    const result = await runChecks({ root, files: [file], runner, deadline: Date.now() + 10_000, sessionId: 's' });
    assert.ok(result.notes.some((n) => /addon-x/.test(n) && /versioning\.md/.test(n)));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('runChecks: in-tree and widget/node/plugin-frontend files are linted in one oxlint call from src/frontend, as absolute paths', async () => {
  const root = tempRoot();
  try {
    // vp refuses a `..` path, so out-of-tree files reach Oxlint as absolute paths from src/frontend,
    // where the shared lint config lives.
    put(root, 'src/frontend/node_modules/vite-plus/bin/vp', '// fake\n');
    const inTree = put(root, 'src/frontend/core/Foo.tsx', 'export const w = 1;\n');
    const widget = put(root, 'widgets/foo/Foo.tsx', 'export const x = 1;\n');
    const node = put(root, 'nodes/bar/index.ts', 'export const y = 1;\n');
    const plugin = put(root, 'src/plugins/Plugins/addon-x/frontend/src/Page.tsx', 'export const z = 1;\n');
    const { runner, calls } = fakeRunner({ oxlint: () => ({ code: 0, stdout: '{"diagnostics":[]}' }) });
    await runChecks({ root, files: [inTree, widget, node, plugin], runner, deadline: Date.now() + 10_000, sessionId: 's' });
    const lintCalls = calls.filter((c) => c.label === 'oxlint');
    assert.equal(lintCalls.length, 1, `expected exactly one oxlint call, got: ${JSON.stringify(calls.map((c) => c.label))}`);
    const [call] = lintCalls;
    assert.equal(call.cwd, join(root, 'src', 'frontend'));
    assert.equal(call.args[1], 'lint');
    assert.ok(call.args.includes('json'), 'oxlint must report as JSON');
    for (const file of [inTree, widget, node, plugin]) assert.ok(call.args.includes(file), `${file} missing from oxlint args`);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('runChecks: a large touched set is linted in batches, so the command line never overflows', async () => {
  const root = tempRoot();
  try {
    put(root, 'src/frontend/node_modules/vite-plus/bin/vp', '// fake\n');
    const files = Array.from({ length: 150 }, (_, i) => put(root, `src/frontend/core/F${i}.tsx`, 'export const w = 1;\n'));
    const { runner, calls } = fakeRunner({ oxlint: () => ({ code: 0, stdout: '{"diagnostics":[]}' }) });
    await runChecks({ root, files, runner, deadline: Date.now() + 10_000, sessionId: 's' });
    const lintCalls = calls.filter((c) => c.label === 'oxlint');
    assert.equal(lintCalls.length, 2);
    for (const file of files) assert.ok(lintCalls.some((c) => c.args.includes(file)), `${file} was never linted`);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('runChecks: oxlint diagnostics become findings on the right file, line and rule', async () => {
  const root = tempRoot();
  try {
    put(root, 'src/frontend/node_modules/vite-plus/bin/vp', '// fake\n');
    const inTree = put(root, 'src/frontend/core/Foo.tsx', 'export const w = 1;\n');
    const plugin = put(root, 'src/plugins/Plugins/addon-x/frontend/src/Page.tsx', 'export const z = 1;\n');
    const stdout = JSON.stringify({
      diagnostics: [
        { message: 'Use <Button>', code: 'mcharbor(no-restricted-syntax)', severity: 'error', filename: 'core/Foo.tsx', labels: [{ span: { line: 7 } }] },
        { message: 'deps', code: 'react-hooks(exhaustive-deps)', severity: 'warning', filename: plugin, labels: [{ span: { line: 3 } }] },
      ],
    });
    const { runner } = fakeRunner({ oxlint: () => ({ code: 1, stdout }) });
    const result = await runChecks({ root, files: [inTree, plugin], runner, deadline: Date.now() + 10_000, sessionId: 's' });
    const lint = result.findings.filter((f) => f.tool === 'oxlint');
    assert.equal(lint.length, 2);
    assert.deepEqual([lint[0].file, lint[0].line, lint[0].rule, lint[0].severity], [inTree, 7, 'mcharbor(no-restricted-syntax)', 'error']);
    assert.deepEqual([lint[1].file, lint[1].line, lint[1].severity], [plugin, 3, 'warn']);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

/* ─── parsers ────────────────────────────────────────────────────── */

test('parseGofmtDiff: maps removed lines to their current-file line numbers', () => {
  const out = '--- /r/a.go.orig\n+++ /r/a.go\n@@ -3,4 +3,4 @@\n func a() {\n-  x := 1\n+\tx := 1\n \ty := 2\n-  z := 3\n+\tz := 3\n';
  const findings = parseGofmtDiff(out);
  assert.deepEqual(findings.map((f) => f.line), [4, 6]);
});

test('parseGoVet: reads relative .\\ paths and vet.exe-prefixed lines against the module dir', () => {
  const out = '# x/modules/a\nvet.exe: .\\modules\\a\\a.go:6:13: string literal not terminated\nmodules/a/b.go:9:2: fmt.Printf format %d has arg "s" of wrong type string\n';
  const findings = parseGoVet(out, 'Z:\\repo\\src\\backend');
  assert.equal(findings.length, 2);
  assert.equal(findings[0].line, 6);
  assert.match(findings[0].file, /src[\\/]backend[\\/]modules[\\/]a[\\/]a\.go$/);
  assert.equal(findings[1].line, 9);
});

test('parseTsc: reads path(line,col): error lines relative to src/frontend', () => {
  const out = 'modules/x/Y.tsx(12,3): error TS2322: Type string is not assignable to type number.\n';
  const [f] = parseTsc(out, 'Z:\\repo\\src\\frontend');
  assert.equal(f.line, 12);
  assert.match(f.file, /modules[\\/]x[\\/]Y\.tsx$/);
  assert.match(f.message, /TS2322/);
});

test('parseSanityReport: only FAIL sections become findings, with path:line when present', () => {
  const md = [
    '### 2. Subscriptions - REVIEW (1 hits)',
    '',
    '- `src/frontend/a.tsx` - setTimeout',
    '',
    '### 12. Bare useTranslation() - FAIL (2 hits)',
    '',
    '- `src/frontend/core/x.ts:51` - const { t } = useTranslation();',
    '- `src/frontend/core/y.ts` - whole file',
  ].join('\n');
  const findings = parseSanityReport(md, 'react-standards', 'Z:\\repo');
  assert.equal(findings.length, 2);
  assert.equal(findings[0].line, 51);
  assert.equal(findings[1].line, null);
  assert.match(findings[0].rule, /react-standards#12/);
});

/* ─── classify ───────────────────────────────────────────────────── */

test('classify: errors on changed lines block; on unchanged lines, warn-only files, or outside the set they warn', () => {
  const a = 'Z:\\r\\a.go';
  const b = 'Z:\\r\\b.go';
  const changed = new Map([
    [a, new Set([4])],
    [b, 'warn-only'],
  ]);
  const findings = [
    { file: a, line: 4, tool: 'gofmt', rule: 'gofmt', message: 'm', severity: 'error' },
    { file: a, line: 9, tool: 'gofmt', rule: 'gofmt', message: 'm', severity: 'error' },
    { file: b, line: 1, tool: 'go vet', rule: 'vet', message: 'm', severity: 'error' },
    { file: 'Z:\\r\\c.go', line: 1, tool: 'tsc', rule: 'tsc', message: 'm', severity: 'error' },
    { file: a, line: 4, tool: 'rules', rule: 'go/get-prefix', message: 'm', severity: 'warn' },
  ];
  const { blocking, warnings } = classify(findings, changed, new Set([a, b]));
  assert.equal(blocking.length, 1);
  assert.equal(blocking[0].line, 4);
  assert.equal(warnings.length, 4);
});

/* ─── decide ─────────────────────────────────────────────────────── */

const failing = [{ file: 'Z:\\r\\a.go', rel: 'src/a.go', line: 4, tool: 'gofmt', rule: 'gofmt', message: 'not gofmt-formatted', severity: 'error' }];

test('decide: a failing result with stop_hook_active false blocks', () => {
  const out = decide({ blocking: failing, missing: [], stopHookActive: false, canBlock: true, report: 'R' });
  assert.deepEqual(out, { decision: 'block', reason: 'R' });
});

test('decide: the same result with stop_hook_active true is a systemMessage and never a block', () => {
  const out = decide({ blocking: failing, missing: [], stopHookActive: true, canBlock: true, report: 'R' });
  assert.deepEqual(out, { systemMessage: 'R' });
  assert.equal(out.decision, undefined);
});

test('decide: a SubagentStop that cannot attribute files reports but never blocks', () => {
  const out = decide({ blocking: failing, missing: [], stopHookActive: false, canBlock: false, report: 'R' });
  assert.deepEqual(out, { systemMessage: 'R' });
});

test('decide: nothing blocking and nothing missing prints nothing', () => {
  assert.equal(decide({ blocking: [], missing: [], stopHookActive: false, canBlock: true, report: 'R' }), null);
});

/* ─── Test floor / Surfaces ──────────────────────────────────────── */

test('missingReplyLines: code touched and no Test floor / Surfaces lines names both rules', () => {
  const missing = missingReplyLines({ event: 'Stop', message: 'Done. Everything works.', codeTouched: true });
  const text = missing.map((m) => `${m.label} ${m.rule}`).join('\n');
  assert.match(text, /Test floor:/);
  assert.match(text, /test-floor\.md/);
  assert.match(text, /Surfaces:/);
  assert.match(text, /cli-and-reporting\.md/);
});

test('missingReplyLines: bold markdown labels are accepted', () => {
  const message = '**Test floor:** added x.test.ts, passes.\n**Surfaces**: CLI — not applicable, no operator action.';
  assert.deepEqual(missingReplyLines({ event: 'Stop', message, codeTouched: true }), []);
});

test('missingReplyLines: a question to the user, no code touched, or a SubagentStop never asks', () => {
  assert.deepEqual(missingReplyLines({ event: 'Stop', message: 'Should I also add the CLI command?', codeTouched: true }), []);
  assert.deepEqual(missingReplyLines({ event: 'Stop', message: 'Done.', codeTouched: false }), []);
  assert.deepEqual(missingReplyLines({ event: 'SubagentStop', message: 'Done.', codeTouched: true }), []);
});

/* ─── gate() end to end with injected runner ─────────────────────── */

async function withSession(fn) {
  const root = tempRoot();
  try {
    await fn(root);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

test('gate: a missing reply line blocks with a reason naming test-floor.md and cli-and-reporting.md, and writes gate.md', async () => {
  await withSession(async (root) => {
    const file = put(root, 'scripts/x/tool.mjs', 'export const x = 1;\n');
    writeFileSync(hooks(root, 's.touched'), `${file}\n`);
    const { runner } = fakeRunner();
    const { output } = await gate(
      { session_id: 's', hook_event_name: 'Stop', stop_hook_active: false, last_assistant_message: 'All done.' },
      { root, runner, git: noGit },
    );
    assert.equal(output.decision, 'block');
    assert.match(output.reason, /test-floor\.md/);
    assert.match(output.reason, /cli-and-reporting\.md/);
    assert.ok(existsSync(hooks(root, 's.gate.md')));
  });
});

test('gate: nothing changed since the last gate run is silent', async () => {
  await withSession(async (root) => {
    const file = put(root, 'scripts/x/tool.mjs', 'export const x = 1;\n', Date.now() - 60_000);
    writeFileSync(hooks(root, 's.touched'), `${file}\n`);
    const { runner } = fakeRunner();
    const payload = { session_id: 's', hook_event_name: 'Stop', stop_hook_active: false, last_assistant_message: 'All done.' };
    const first = await gate(payload, { root, runner, git: noGit });
    assert.equal(first.output.decision, 'block');
    const second = await gate(payload, { root, runner, git: noGit });
    assert.equal(second.output, null);
  });
});

test('gate: last_assistant_message falls back to the transcript when the payload lacks it', async () => {
  await withSession(async (root) => {
    const file = put(root, 'scripts/x/tool.mjs', 'export const x = 1;\n');
    writeFileSync(hooks(root, 's.touched'), `${file}\n`);
    const transcript = join(root, 't.jsonl');
    const entry = {
      type: 'assistant',
      message: { role: 'assistant', content: [{ type: 'text', text: 'Test floor: none owed.\nSurfaces: CLI n/a, Reports n/a.' }] },
    };
    writeFileSync(transcript, JSON.stringify({ type: 'user', message: { content: 'hi' } }) + '\n' + JSON.stringify(entry) + '\n');
    const { runner } = fakeRunner();
    const { output } = await gate(
      { session_id: 's', hook_event_name: 'Stop', stop_hook_active: false, transcript_path: transcript },
      { root, runner, git: noGit },
    );
    assert.equal(output, null);
  });
});

test('gate: SubagentStop blocks on the subagent\'s own changed line, not on a sibling\'s file', async () => {
  await withSession(async (root) => {
    put(root, 'src/backend/go.mod', 'module x\n');
    const mine = put(root, 'src/backend/modules/a/handler.go', 'package a\n\nfunc h() { respond(err.Error()) }\n');
    const sibling = put(root, 'src/backend/modules/b/handler.go', 'package b\n\nfunc h() { respond(err.Error()) }\n');
    writeFileSync(hooks(root, 's.touched'), `${mine}\tagent-1\n${sibling}\tagent-2\n`);
    writeFileSync(
      hooks(root, 's.lines.jsonl'),
      [mine, sibling].map((file, i) => JSON.stringify({ file, all: true, texts: [], agent: `agent-${i + 1}` })).join('\n') + '\n',
    );
    const { runner, calls } = fakeRunner({ gofmt: () => ({ code: 0 }), 'go vet': () => ({ code: 0 }) });
    const { output } = await gate(
      { session_id: 's', hook_event_name: 'SubagentStop', stop_hook_active: false, agent_id: 'agent-1' },
      { root, runner, git: noGit },
    );
    assert.equal(output.decision, 'block');
    assert.match(output.reason, /modules\/a\/handler\.go:3/);
    assert.doesNotMatch(output.reason, /modules\/b\/handler\.go/);
    assert.ok(existsSync(hooks(root, 's.agent-1.gate.md')), 'SubagentStop writes its own report');
    assert.ok(!existsSync(hooks(root, 's.gate.md')), 'and leaves the main thread\'s report alone');
    assert.match(output.reason, /\.results\/hooks\/s\.agent-1\.gate\.md/);
    assert.ok(calls.every((c) => !c.args.some((arg) => /modules[\\/]b/.test(arg))));
  });
});

test('gate: an internal error becomes a systemMessage, never a block', async () => {
  await withSession(async (root) => {
    const file = put(root, 'src/backend/modules/a/service.go', 'package a\n');
    writeFileSync(hooks(root, 's.touched'), `${file}\n`);
    const runner = async () => {
      throw new Error('boom');
    };
    const { output } = await gate({ session_id: 's', hook_event_name: 'Stop', stop_hook_active: false }, { root, runner, git: noGit });
    assert.equal(output.decision, undefined);
    assert.match(output.systemMessage, /boom|internal/i);
  });
});

/* ─── CLI ────────────────────────────────────────────────────────── */

test('CLI: an empty touched set runs nothing, prints nothing and exits within 1 s', () => {
  const root = tempRoot();
  try {
    const started = Date.now();
    const result = spawnSync(process.execPath, [GATE_SCRIPT], {
      input: JSON.stringify({ session_id: 'empty-session', hook_event_name: 'Stop', stop_hook_active: false }),
      encoding: 'utf8',
      env: { ...process.env, MCHARBOR_GATE_ROOT: root },
    });
    const elapsed = Date.now() - started;
    assert.equal(result.status, 0);
    assert.equal(result.stdout, '');
    assert.ok(elapsed < 1_000, `took ${elapsed} ms`);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('CLI: malformed stdin is reported as a systemMessage and exits 0', () => {
  const root = tempRoot();
  try {
    const result = spawnSync(process.execPath, [GATE_SCRIPT], {
      input: '{not json',
      encoding: 'utf8',
      env: { ...process.env, MCHARBOR_GATE_ROOT: root },
    });
    assert.equal(result.status, 0);
    const parsed = JSON.parse(result.stdout);
    assert.ok(parsed.systemMessage);
    assert.equal(parsed.decision, undefined);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

/* ─── session-start ──────────────────────────────────────────────── */

test('session-start: writes .start, truncates .touched and records the porcelain snapshot', () => {
  const root = tempRoot();
  try {
    writeFileSync(hooks(root, 's.touched'), 'old\n');
    writeFileSync(hooks(root, 's.lines.jsonl'), '{}\n');
    const git = () => ({ ok: true, stdout: ' M src/a.go\0?? src/new file.ts\0R  src/to.go\0src/from.go\0' });
    startSession({ session_id: 's', source: 'startup' }, { root, git, now: 1234 });
    assert.equal(readFileSync(hooks(root, 's.start'), 'utf8'), '1234');
    assert.equal(readFileSync(hooks(root, 's.touched'), 'utf8'), '');
    assert.ok(!existsSync(hooks(root, 's.lines.jsonl')));
    const status = JSON.parse(readFileSync(hooks(root, 's.start-status.json'), 'utf8'));
    assert.equal(status.ok, true);
    assert.deepEqual(status.dirty.sort(), ['src/a.go', 'src/from.go', 'src/new file.ts', 'src/to.go']);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('session-start: resume and compact keep an existing session\'s state; a subagent payload does nothing', () => {
  const root = tempRoot();
  try {
    writeFileSync(hooks(root, 's.start'), '1');
    writeFileSync(hooks(root, 's.touched'), 'keep\n');
    const git = () => ({ ok: true, stdout: '' });
    startSession({ session_id: 's', source: 'resume' }, { root, git, now: 99 });
    startSession({ session_id: 's', source: 'compact' }, { root, git, now: 99 });
    startSession({ session_id: 's', source: 'startup', agent_id: 'a1' }, { root, git, now: 99 });
    assert.equal(readFileSync(hooks(root, 's.start'), 'utf8'), '1');
    assert.equal(readFileSync(hooks(root, 's.touched'), 'utf8'), 'keep\n');
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

/* ─── Fix round 1 ────────────────────────────────────────────────── */

const RAW_ERR_GO = 'package a\n\nfunc h() { respond(err.Error()) }\n';

function sweepSession(root, { windows = [] } = {}) {
  const start = Date.now() - 60_000;
  writeFileSync(hooks(root, 's.start'), String(start));
  writeFileSync(hooks(root, 's.start-status.json'), JSON.stringify({ ok: true, dirty: [] }));
  writeFileSync(hooks(root, 's.touched'), '');
  const lines = [];
  for (const [i, [from, to]] of windows.entries()) {
    lines.push(JSON.stringify({ id: `t${i}`, phase: 'start', at: from, agent: null }));
    lines.push(JSON.stringify({ id: `t${i}`, phase: 'end', at: to, duration: to - from, agent: null }));
  }
  writeFileSync(hooks(root, 's.shell.jsonl'), lines.join('\n') + (lines.length ? '\n' : ''));
  return start;
}

// Untracked, not ignored, not dirty at start: the file resolves to 'all', so
// only shell-window attribution decides whether its finding may block.
const untrackedGit = (args) => (args[0] === 'check-ignore' ? { ok: false, code: 1, stdout: '' } : { ok: true, code: 0, stdout: '' });

test('round1/1: a sweep-only file edited outside every shell window never blocks and never demands reply lines', async () => {
  await withSession(async (root) => {
    sweepSession(root);
    put(root, 'src/backend/go.mod', 'module x\n');
    put(root, 'src/backend/modules/a/handler.go', RAW_ERR_GO);
    const { runner } = fakeRunner({ gofmt: () => ({ code: 0 }), 'go vet': () => ({ code: 0 }) });
    const { output } = await gate(
      { session_id: 's', hook_event_name: 'Stop', stop_hook_active: false, last_assistant_message: 'Sure, here is the answer.' },
      { root, runner, git: untrackedGit },
    );
    assert.equal(output, null);
  });
});

test("round1/1: the same sweep-only file blocks when its mtime falls inside one of this session's shell windows", async () => {
  await withSession(async (root) => {
    const now = Date.now();
    sweepSession(root, { windows: [[now - 5_000, now + 1_000]] });
    put(root, 'src/backend/go.mod', 'module x\n');
    put(root, 'src/backend/modules/a/handler.go', RAW_ERR_GO, now - 2_000);
    const { runner } = fakeRunner({ gofmt: () => ({ code: 0 }), 'go vet': () => ({ code: 0 }) });
    const { output } = await gate(
      { session_id: 's', hook_event_name: 'Stop', stop_hook_active: false, last_assistant_message: 'Test floor: n/a.\nSurfaces: n/a.' },
      { root, runner, git: untrackedGit },
    );
    assert.equal(output?.decision, 'block');
    assert.match(output.reason, /go\/raw-err-to-client/);
  });
});

test("round1/1: sweep hits listed in another session's recent .touched are dropped", () => {
  const root = tempRoot();
  try {
    const start = Date.now() - 60_000;
    const theirs = put(root, 'src/theirs.go', 'package t\n', start + 1_000);
    const mine = put(root, 'src/mine.go', 'package m\n', start + 1_000);
    writeFileSync(hooks(root, 'other-session.touched'), `${theirs}\n`);
    const { files } = collectTouched({ root, sessionId: 's', startMs: start });
    assert.deepEqual(files, [mine]);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('round1/1: shellWindows pairs start/end by id, falls back to duration, and pads by 2 s', () => {
  const windows = shellWindows([
    { id: 'a', phase: 'start', at: 10_000 },
    { id: 'a', phase: 'end', at: 20_000, duration: 9_000 },
    { id: 'b', phase: 'end', at: 50_000, duration: 5_000 },
  ]);
  assert.deepEqual(windows, [
    [8_000, 22_000],
    [43_000, 52_000],
  ]);
});

test('round1/1: record-shell appends start and end records to <session>.shell.jsonl', () => {
  const root = tempRoot();
  try {
    recordShell({ session_id: 's', hook_event_name: 'PreToolUse', tool_name: 'Bash', tool_use_id: 'u1' }, { root, now: 100 });
    recordShell(
      { session_id: 's', hook_event_name: 'PostToolUse', tool_name: 'PowerShell', tool_use_id: 'u1', duration_ms: 40, agent_id: 'ag' },
      { root, now: 150 },
    );
    const recs = readFileSync(hooks(root, 's.shell.jsonl'), 'utf8').trim().split('\n').map((l) => JSON.parse(l));
    assert.deepEqual(recs, [
      { id: 'u1', phase: 'start', at: 100, duration: null, agent: null },
      { id: 'u1', phase: 'end', at: 150, duration: 40, agent: 'ag' },
    ]);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('round1/2: an untracked gitignored file is never "all" — without records it is warn-only', () => {
  const root = tempRoot();
  try {
    const file = put(root, 'src/plugins/Plugins/addon-x/backend/main.go', 'package main\n');
    const git = (args) => {
      if (args[0] === 'check-ignore') return { ok: true, code: 0, stdout: 'src/plugins/Plugins/addon-x/backend/main.go\0' };
      return { ok: true, code: 0, stdout: '' };
    };
    const changed = resolveChangedLines({
      root,
      files: [file],
      startMs: Date.now() - 60_000,
      startStatus: { ok: true, dirty: [] },
      records: [],
      git,
    });
    assert.equal(changed.get(file), 'warn-only');
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('round1/3: records that are only "unknown" leave the file warn-only', () => {
  const root = tempRoot();
  try {
    const file = put(root, 'src/a.go', 'package a\nfunc A() {}\n');
    const changed = resolveChangedLines({
      root,
      files: [file],
      startMs: Date.now() - 1_000,
      startStatus: { ok: true, dirty: ['src/a.go'] },
      records: [{ file, all: false, unknown: true, texts: [], agent: null }],
      git: noGit,
    });
    assert.equal(changed.get(file), 'warn-only');
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('round1/4: a plugin frontend edited this session and not rebuilt blocks even when its lines are warn-only', async () => {
  const root = tempRoot();
  try {
    const now = Date.now();
    put(root, 'src/plugins/Plugins/addon-x/frontend/dist/addon-x.umd.cjs', 'x\n', now - 120_000);
    const src = put(root, 'src/plugins/Plugins/addon-x/frontend/src/Page.tsx', 'x\n', now);
    const { runner } = fakeRunner();
    const result = await runChecks({
      root,
      files: [src],
      runner,
      deadline: now + 10_000,
      sessionId: 's',
      changed: new Map([[src, 'warn-only']]),
      startMs: now - 60_000,
    });
    const { blocking } = classify(result.findings, new Map([[src, 'warn-only']]), new Set([src]));
    assert.ok(blocking.some((f) => f.rule === 'plugin/frontend-not-rebuilt'));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('round1/4: a bundle rebuilt beside frontend/ (vite outDir ../frontend) counts as rebuilt', async () => {
  const root = tempRoot();
  try {
    const now = Date.now();
    const src = put(root, 'src/plugins/Plugins/addon-x/frontend/src/Page.tsx', 'x\n', now - 120_000);
    put(root, 'src/plugins/Plugins/addon-x/frontend/other-plugin.umd.cjs', 'x\n', now - 300_000);
    put(root, 'src/plugins/Plugins/addon-x/frontend/addon-x.umd.cjs', 'x\n', now);
    const { runner } = fakeRunner();
    const result = await runChecks({
      root,
      files: [src],
      runner,
      deadline: now + 10_000,
      sessionId: 's',
      changed: new Map([[src, 'warn-only']]),
      startMs: now - 600_000,
    });
    assert.ok(!result.findings.some((f) => f.rule === 'plugin/frontend-not-rebuilt'));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('round1/4: a foreign (unattributed) plugin frontend edit still does not block', () => {
  const src = 'Z:\\r\\src\\plugins\\Plugins\\addon-x\\frontend\\src\\Page.tsx';
  const finding = { file: src, line: null, fileLevel: true, force: true, tool: 'plugin-build', rule: 'plugin/frontend-not-rebuilt', message: 'm', severity: 'error' };
  const { blocking } = classify([finding], new Map([[src, 'foreign']]), new Set([src]));
  assert.equal(blocking.length, 0);
});

test('round1/4: notes alone are emitted as a systemMessage', () => {
  const out = decide({ blocking: [], missing: [], notes: ['plugin/addon-x: backend changed'], stopHookActive: false, canBlock: true, report: 'R' });
  assert.deepEqual(out, { systemMessage: 'R' });
});

/* ─── .results/hooks pruning (30 days) ───────────────────────────── */

const DAY_MS = 24 * 60 * 60_000;

function seedHookState(root, now) {
  const old = now - 40 * DAY_MS;
  const fresh = now - 2 * DAY_MS;
  for (const name of ['old.touched', 'old.lines.jsonl', 'old.shell.jsonl', 'old.gate.md', 'old.gate-state.json', 'old.start', 'old.start-status.json']) {
    put(root, `.results/hooks/${name}`, 'x', old);
  }
  put(root, '.results/hooks/old.a1.gate.md', 'x', old);
  put(root, '.results/hooks/sanity-old-agent_a1/report.md', 'x', old);
  utimesSync(join(root, '.results', 'hooks', 'sanity-old-agent_a1'), old / 1000, old / 1000);
  put(root, '.results/hooks/fresh.touched', 'x', fresh);
  put(root, '.results/hooks/manual-gate.md', 'x', old);
  put(root, '.results/hooks/manual-gate.json', '{}', old);
}

test('prune: session-start removes state files and sanity dirs older than 30 days and keeps the rest', () => {
  const root = tempRoot();
  try {
    const now = Date.now();
    seedHookState(root, now);
    startSession({ session_id: 's', source: 'startup' }, { root, git: () => ({ ok: false, stdout: '' }), now });
    for (const name of ['old.touched', 'old.lines.jsonl', 'old.shell.jsonl', 'old.gate.md', 'old.gate-state.json', 'old.start', 'old.start-status.json', 'old.a1.gate.md', 'sanity-old-agent_a1']) {
      assert.ok(!existsSync(hooks(root, name)), `${name} should be pruned`);
    }
    assert.ok(existsSync(hooks(root, 'fresh.touched')));
    assert.ok(existsSync(hooks(root, 'manual-gate.md')));
    assert.ok(existsSync(hooks(root, 'manual-gate.json')));
    assert.ok(existsSync(hooks(root, 's.start')));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('prune: the gate prunes too, never the running session\'s files, and does bounded work per run', async () => {
  await withSession(async (root) => {
    const now = Date.now();
    seedHookState(root, now);
    put(root, '.results/hooks/s.lines.jsonl', 'x', now - 40 * DAY_MS);
    for (let i = 0; i < 250; i++) put(root, `.results/hooks/bulk${i}.touched`, 'x', now - 40 * DAY_MS);
    const { runner } = fakeRunner();
    await gate({ session_id: 's', hook_event_name: 'Stop', stop_hook_active: false, last_assistant_message: 'ok' }, { root, runner, git: noGit });
    assert.ok(existsSync(hooks(root, 's.lines.jsonl')), 'the running session keeps its own files');
    assert.ok(existsSync(hooks(root, 'fresh.touched')));
    const bulkLeft = Array.from({ length: 250 }, (_, i) => i).filter((i) => existsSync(hooks(root, `bulk${i}.touched`))).length;
    assert.ok(bulkLeft > 0 && bulkLeft < 250, `one run removes some but not all of the backlog (left ${bulkLeft})`);
  });
});
