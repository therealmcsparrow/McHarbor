// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, rmSync, existsSync, readFileSync, utimesSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

import { defaultGit } from '../session-start.mjs';
import { parseCliArgs, resolveCliScope, cliChangedLines, runCli, saveCliBaselines } from '../gate-touched.mjs';

const HERE = fileURLToPath(new URL('.', import.meta.url));
const GATE_SCRIPT = join(HERE, '..', 'gate-touched.mjs');

function tempRoot() {
  const root = mkdtempSync(join(tmpdir(), 'gate-cli-'));
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

const noGit = () => ({ ok: false, stdout: '' });

function fakeRunner(overrides = {}) {
  return async (spec) => {
    const hit = overrides[spec.label];
    if (hit) return { code: 0, stdout: '', stderr: '', missing: false, timedOut: false, ...hit(spec) };
    return { code: null, stdout: '', stderr: '', missing: true, timedOut: false };
  };
}

function gofmtDiff(file) {
  const fwd = file.replace(/\\/g, '/');
  return [`diff ${fwd}.orig ${fwd}`, `--- ${fwd}.orig`, `+++ ${fwd}`, '@@ -1,5 +1,5 @@', ' package a', ' ', ' func a() {', '-  x := 1', '+\tx := 1', ' }', ''].join('\n');
}

const cleanRunner = () => fakeRunner({ gofmt: () => ({ code: 0 }), 'go vet': () => ({ code: 0 }), 'golangci-lint': () => ({ code: 0, stdout: '{"Issues":[]}' }) });

test('parseCliArgs: defaults to the last-run scope', () => {
  assert.deepEqual(parseCliArgs(['--cli']), { mode: 'default', paths: [], since: null, error: null });
});

test('parseCliArgs: --paths takes commas and repeated values', () => {
  const a = parseCliArgs(['--cli', '--paths', 'a.go,b.tsx', 'c.ts']);
  assert.equal(a.mode, 'paths');
  assert.deepEqual(a.paths, ['a.go', 'b.tsx', 'c.ts']);
});

test('parseCliArgs: --since and --git-status set their mode, conflicts and junk are errors', () => {
  assert.equal(parseCliArgs(['--cli', '--since', 'HEAD~3']).since, 'HEAD~3');
  assert.equal(parseCliArgs(['--cli', '--git-status']).mode, 'git-status');
  assert.match(parseCliArgs(['--cli', '--since', '--git-status']).error, /--since needs/);
  assert.match(parseCliArgs(['--cli', '--git-status', '--since', 'x']).error, /choose one/);
  assert.match(parseCliArgs(['--cli', '--paths']).error, /--paths needs/);
  assert.match(parseCliArgs(['--cli', '--bogus']).error, /unknown argument/);
});

test('resolveCliScope: explicit paths keep real files and drop missing, excluded and non-source ones', () => {
  const root = tempRoot();
  try {
    const go = put(root, 'src/backend/modules/a/service.go', 'package a\n');
    put(root, 'src/frontend/node_modules/x/index.ts', 'x\n');
    put(root, 'README.txt', 'x\n');
    const scope = resolveCliScope({
      root,
      args: parseCliArgs(['--cli', '--paths', 'src/backend/modules/a/service.go,src/frontend/node_modules/x/index.ts,README.txt,nope.go']),
      git: noGit,
    });
    assert.deepEqual(scope.files, [go]);
    assert.ok(scope.notes.some((n) => n.startsWith('nope.go')));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('resolveCliScope: --since with an ISO time sweeps by mtime', () => {
  const root = tempRoot();
  try {
    const now = Date.now();
    put(root, 'src/old.go', 'package a\n', now - 5 * 3600_000);
    const fresh = put(root, 'src/new.go', 'package a\n', now - 60_000);
    const iso = new Date(now - 3600_000).toISOString();
    const scope = resolveCliScope({ root, args: parseCliArgs(['--cli', '--since', iso]), git: noGit, now });
    assert.deepEqual(scope.files, [fresh]);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('resolveCliScope: --since with a git ref lists diff plus untracked, and a bad ref scopes nothing', () => {
  const root = tempRoot();
  try {
    const a = put(root, 'src/a.go', 'package a\n');
    const b = put(root, 'src/b.go', 'package a\n');
    const git = (args) => {
      if (args[0] === 'rev-parse') return { ok: args[3] === 'good^{commit}', stdout: '' };
      if (args[0] === 'diff') return { ok: true, stdout: 'src/a.go\0src/gone.go\0' };
      if (args[0] === 'ls-files') return { ok: true, stdout: 'src/b.go\0' };
      return { ok: false, stdout: '' };
    };
    const ok = resolveCliScope({ root, args: parseCliArgs(['--cli', '--since', 'good']), git });
    assert.deepEqual(ok.files, [a, b]);
    assert.equal(ok.baseRef, 'good');
    const bad = resolveCliScope({ root, args: parseCliArgs(['--cli', '--since', 'nonsense']), git });
    assert.deepEqual(bad.files, []);
    assert.ok(bad.notes[0].includes('not a git ref'));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('resolveCliScope: --git-status reads porcelain, both sides of a rename, dropping deleted files', () => {
  const root = tempRoot();
  try {
    const a = put(root, 'src/a.go', 'package a\n');
    const c = put(root, 'src/c.go', 'package a\n');
    const git = () => ({ ok: true, stdout: ' M src/a.go\0?? src/c.go\0 D src/deleted.go\0' });
    const scope = resolveCliScope({ root, args: parseCliArgs(['--cli', '--git-status']), git });
    assert.deepEqual(scope.files, [a, c]);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('resolveCliScope: default mode is 24 h on the first run, then the recorded marker', () => {
  const root = tempRoot();
  try {
    const now = Date.now();
    put(root, 'src/two-days.go', 'package a\n', now - 48 * 3600_000);
    const hourAgo = put(root, 'src/hour.go', 'package a\n', now - 3600_000);
    const first = resolveCliScope({ root, args: parseCliArgs(['--cli']), git: noGit, now });
    assert.deepEqual(first.files, [hourAgo]);
    assert.match(first.label, /first manual run/);
    writeFileSync(join(root, '.results', 'hooks', 'manual-gate.json'), JSON.stringify({ at: now - 60_000 }));
    const later = resolveCliScope({ root, args: parseCliArgs(['--cli']), git: noGit, now });
    assert.deepEqual(later.files, []);
    assert.match(later.label, /last manual gate run/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('cliChangedLines: hunks bound a tracked file, everything else is all lines', () => {
  const root = tempRoot();
  try {
    const tracked = put(root, 'src/a.go', 'package a\n');
    const fresh = put(root, 'src/b.go', 'package a\n');
    const git = (args) => {
      if (args[0] === 'ls-files') return { ok: true, stdout: 'src/a.go\0' };
      return { ok: true, stdout: 'diff --git a/src/a.go b/src/a.go\n--- a/src/a.go\n+++ b/src/a.go\n@@ -3,0 +4,2 @@\n+x\n+y\n' };
    };
    const changed = cliChangedLines({ root, files: [tracked, fresh], git });
    assert.deepEqual([...changed.get(tracked)], [4, 5]);
    assert.equal(changed.get(fresh), 'all');
    assert.equal(cliChangedLines({ root, files: [tracked], git: noGit }).get(tracked), 'all');
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('runCli: a finding in a scoped file exits 1, names it, snapshots it, and leaves the marker', async () => {
  const root = tempRoot();
  try {
    put(root, 'src/backend/go.mod', 'module x\n');
    const file = put(root, 'src/backend/modules/a/service.go', 'package a\n\nfunc a() {\n  x := 1\n}\n');
    const lines = [];
    const runner = fakeRunner({ gofmt: () => ({ code: 1, stdout: gofmtDiff(file) }), 'go vet': () => ({ code: 0 }), 'golangci-lint': () => ({ code: 0, stdout: '{"Issues":[]}' }) });
    const code = await runCli(['--cli', '--git-status'], { root, git: () => ({ ok: true, stdout: '?? src/backend/modules/a/service.go\0' }), runner, out: (s) => lines.push(s) });
    assert.equal(code, 1);
    const text = lines.join('\n');
    assert.match(text, /1 blocking finding/);
    assert.match(text, /src\/backend\/modules\/a\/service\.go:4 \[gofmt\/gofmt\]/);
    assert.ok(!existsSync(join(root, '.results', 'hooks', 'manual-gate.json')));
    assert.ok(existsSync(join(root, '.results', 'hooks', 'manual-base', 'src', 'backend', 'modules', 'a', 'service.go')));
    assert.ok(existsSync(join(root, '.results', 'hooks', 'manual-gate.md')));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('runCli: a clean --paths run exits 0 without moving the marker; a clean default run moves it', async () => {
  const root = tempRoot();
  try {
    put(root, 'src/backend/go.mod', 'module x\n');
    put(root, 'src/backend/modules/a/service.go', 'package a\n');
    const opts = { root, git: noGit, runner: cleanRunner(), out: () => {} };
    const marker = join(root, '.results', 'hooks', 'manual-gate.json');
    assert.equal(await runCli(['--cli', '--paths', 'src/backend/modules/a/service.go'], opts), 0);
    assert.ok(!existsSync(marker));
    const before = Date.now();
    assert.equal(await runCli(['--cli'], opts), 0);
    assert.ok(JSON.parse(readFileSync(marker, 'utf8')).at >= before - 1000);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('runCli: a usage error exits 2, and a check that crashed exits 2 rather than claiming clean', async () => {
  const root = tempRoot();
  try {
    const lines = [];
    assert.equal(await runCli(['--cli', '--bogus'], { root, out: (s) => lines.push(s) }), 2);
    assert.match(lines.join('\n'), /usage:/);
    put(root, 'src/backend/go.mod', 'module x\n');
    put(root, 'src/backend/modules/a/service.go', 'package a\n');
    const boom = fakeRunner({
      gofmt: () => {
        throw new Error('spawn exploded');
      },
    });
    const code = await runCli(['--cli', '--paths', 'src/backend/modules/a/service.go'], { root, git: noGit, runner: boom, out: (s) => lines.push(s) });
    assert.equal(code, 2);
    assert.match(lines.join('\n'), /Internal errors/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('hook contract: --cli never reads stdin and a bad flag exits 2 from the real script', () => {
  const result = spawnSync(process.execPath, [GATE_SCRIPT, '--cli', '--bogus'], { encoding: 'utf8', input: '', timeout: 20_000 });
  assert.equal(result.status, 2);
  assert.match(result.stdout, /unknown argument/);
});

function twoHunkDiff(file) {
  const fwd = file.replace(/\\/g, '/');
  return [`diff ${fwd}.orig ${fwd}`, `--- ${fwd}.orig`, `+++ ${fwd}`, '@@ -1,8 +1,8 @@', ' package a', ' ', ' func a() {', '-  x := 1', '+\tx := 1', ' }', ' ', '-  y := 2', '+\ty := 2', ' }', ''].join('\n');
}

test('runCli: after a completed run only lines changed since it can block, and an unchanged file is skipped', async () => {
  const root = tempRoot();
  try {
    put(root, 'src/backend/go.mod', 'module x\n');
    const file = put(root, 'src/backend/modules/a/service.go', 'package a\n\nfunc a() {\n  x := 1\n}\n');
    const rel = 'src/backend/modules/a/service.go';
    const runner = fakeRunner({ gofmt: () => ({ code: 1, stdout: twoHunkDiff(file) }), 'go vet': () => ({ code: 0 }), 'golangci-lint': () => ({ code: 0, stdout: '{"Issues":[]}' }) });
    const old = new Date(Date.now() - 3600_000).toISOString();
    const lines = [];
    const opts = { root, git: defaultGit(root), runner, out: (s) => lines.push(s) };
    assert.equal(await runCli(['--cli', '--paths', rel], opts), 1);
    writeFileSync(file, 'package a\n\nfunc a() {\n  x := 1\n}\n\nfunc b() {\n  y := 2\n}\n');
    lines.length = 0;
    assert.equal(await runCli(['--cli', '--since', old], opts), 1);
    const text = lines.join('\n');
    assert.match(text, /service\.go:7 \[gofmt/);
    assert.ok(!/service\.go:4 \[gofmt/.test(text));
    lines.length = 0;
    assert.equal(await runCli(['--cli', '--since', old], opts), 0);
    assert.match(lines.join('\n'), /unchanged since the last manual run/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('runCli: a legacy finding in a long-dirty file lets the default scope go clean on the second run', async () => {
  const root = tempRoot();
  try {
    put(root, 'src/backend/go.mod', 'module x\n');
    const file = put(root, 'src/backend/modules/a/service.go', 'package a\n\nfunc a() {\n  x := 1\n}\n', Date.now() - 1000);
    const runner = fakeRunner({ gofmt: () => ({ code: 1, stdout: gofmtDiff(file) }), 'go vet': () => ({ code: 0 }), 'golangci-lint': () => ({ code: 0, stdout: '{"Issues":[]}' }) });
    const opts = { root, git: defaultGit(root), runner, out: () => {} };
    assert.equal(await runCli(['--cli'], opts), 1);
    const later = (Date.now() + 5000) / 1000;
    utimesSync(file, later, later);
    assert.equal(await runCli(['--cli'], { ...opts, now: () => Date.now() + 10_000 }), 0);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('saveCliBaselines: entries older than 30 days are pruned and oversized files are not copied', () => {
  const root = tempRoot();
  try {
    const stale = put(root, '.results/hooks/manual-base/src/old.go', 'package a\n', Date.now() - 40 * 86400_000);
    const big = put(root, 'src/big.go', 'x'.repeat(1_100_000));
    saveCliBaselines({ root, files: [big] });
    assert.ok(!existsSync(stale));
    assert.ok(!existsSync(join(root, '.results', 'hooks', 'manual-base', 'src', 'big.go')));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('runCli: --paths that resolves to no checkable file exits 2, never a clean 0', async () => {
  const root = tempRoot();
  try {
    put(root, 'notes.txt', 'x\n');
    const lines = [];
    const code = await runCli(['--cli', '--paths', 'nope.go,notes.txt'], { root, git: noGit, runner: cleanRunner(), out: (s) => lines.push(s) });
    assert.equal(code, 2);
    assert.match(lines.join('\n'), /nothing ran/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('runCli: only a default run moves the shared marker; -Since ISO, -GitStatus and -Paths leave it', async () => {
  const root = tempRoot();
  try {
    put(root, 'src/backend/go.mod', 'module x\n');
    put(root, 'src/backend/modules/a/service.go', 'package a\n');
    const marker = join(root, '.results', 'hooks', 'manual-gate.json');
    const git = () => ({ ok: true, stdout: '?? src/backend/modules/a/service.go\0' });
    const opts = { root, git, runner: cleanRunner(), out: () => {} };
    const old = new Date(Date.now() - 3600_000).toISOString();
    assert.equal(await runCli(['--cli', '--since', old], opts), 0);
    assert.ok(!existsSync(marker));
    assert.equal(await runCli(['--cli', '--git-status'], opts), 0);
    assert.ok(!existsSync(marker));
    assert.equal(await runCli(['--cli', '--since', old], opts), 0);
    assert.ok(!existsSync(marker));
    assert.equal(await runCli(['--cli'], opts), 0);
    assert.ok(existsSync(marker));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('runCli: an exit-1 report says the findings will not be re-reported by the next default run', async () => {
  const root = tempRoot();
  try {
    put(root, 'src/backend/go.mod', 'module x\n');
    const file = put(root, 'src/backend/modules/a/service.go', 'package a\n\nfunc a() {\n  x := 1\n}\n');
    const runner = fakeRunner({ gofmt: () => ({ code: 1, stdout: gofmtDiff(file) }), 'go vet': () => ({ code: 0 }), 'golangci-lint': () => ({ code: 0, stdout: '{"Issues":[]}' }) });
    const lines = [];
    assert.equal(await runCli(['--cli', '--paths', 'src/backend/modules/a/service.go'], { root, git: noGit, runner, out: (s) => lines.push(s) }), 1);
    assert.match(lines.join('\n'), /will not be re-reported by the next default run unless the lines change again/);
    const clean = [];
    await runCli(['--cli', '--paths', 'src/backend/modules/a/service.go'], { root, git: noGit, runner: cleanRunner(), out: (s) => clean.push(s) });
    assert.ok(!/will not be re-reported/.test(clean.join('\n')));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
