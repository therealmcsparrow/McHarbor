// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, rmSync, readFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

import { check as checkGo } from '../rules/go-patterns.mjs';
import { check as checkTs } from '../rules/ts-patterns.mjs';
import { check as checkCopyright } from '../rules/copyright.mjs';
import { check as checkLocale } from '../rules/locale-json.mjs';
import { changedLines, inChanged, isNewFile, recordTouched, hooksDir } from '../lib/payload.mjs';
import { run } from '../lint-edited.mjs';

const HERE = fileURLToPath(new URL('.', import.meta.url));
const REPO = join(HERE, '..', '..', '..');
const LINT_SCRIPT = join(REPO, 'scripts', 'hooks', 'lint-edited.mjs');

function names(findings) {
  return findings.map((f) => f.rule);
}

/* ─── go-patterns ────────────────────────────────────────────────── */

test('go/raw-err-to-client: fires on err.Error() in a *handler*.go file, even non-suffix', () => {
  const text = 'func x() {\n\twriteJSON(w, map[string]string{"reason": err.Error()})\n}\n';
  const findings = checkGo({ path: 'handler_propose.go', text, changedLines: 'all' });
  assert.ok(names(findings).includes('go/raw-err-to-client'));
});

test('go/raw-err-to-client: does not fire in src/agent, which replies over RPC', () => {
  const text = 'reply.Error = err.Error()\n';
  const findings = checkGo({ path: 'src/agent/compute_handlers_linux.go', text, changedLines: 'all' });
  assert.ok(!names(findings).includes('go/raw-err-to-client'));
});

test('go/wrap-with-w: fires when the Errorf arg is named err', () => {
  const text = 'fmt.Errorf("x: %v", err)\n';
  const findings = checkGo({ path: 'service.go', text, changedLines: 'all' });
  assert.ok(names(findings).includes('go/wrap-with-w'));
});

test('go/wrap-with-w: does not fire when the Errorf arg is not named err', () => {
  const text = 'fmt.Errorf("x %v", n)\n';
  const findings = checkGo({ path: 'service.go', text, changedLines: 'all' });
  assert.ok(!names(findings).includes('go/wrap-with-w'));
});

test('go/get-prefix: fires on any non-test .go file, not just service.go', () => {
  const text = 'func (s *Store) GetRun(ctx context.Context) (*Run, error) {\n';
  const findings = checkGo({ path: 'store.go', text, changedLines: 'all' });
  assert.ok(names(findings).includes('go/get-prefix'));
});

test('go/get-prefix: does not fire on a _test.go file', () => {
  const text = 'func (s *Store) GetRun(ctx context.Context) (*Run, error) {\n';
  const findings = checkGo({ path: 'store_test.go', text, changedLines: 'all' });
  assert.ok(!names(findings).includes('go/get-prefix'));
});

test('go/ignored-error: fires on anchored `_, _ =`', () => {
  const text = '_, _ = io.Copy(w, b)\n';
  const findings = checkGo({ path: 'service.go', text, changedLines: 'all' });
  assert.ok(names(findings).includes('go/ignored-error'));
});

test('go/ignored-error: does not false-positive on `path, _, _ := f()`', () => {
  const text = 'path, _, _ := f()\n';
  const findings = checkGo({ path: 'service.go', text, changedLines: 'all' });
  assert.ok(!names(findings).includes('go/ignored-error'));
});

test('go/checkorigin-true: fires on an unconditional CheckOrigin', () => {
  const text = 'CheckOrigin: func(*http.Request) bool { return true },\n';
  const findings = checkGo({ path: 'ws.go', text, changedLines: 'all' });
  assert.ok(names(findings).includes('go/checkorigin-true'));
  assert.match(findings.find((f) => f.rule === 'go/checkorigin-true').message, /IsAllowedWebSocketOrigin/);
});

test('go/sql-in-handler: fires on db./tx. calls but not r.URL.Query()', () => {
  const positive = checkGo({
    path: 'handler.go',
    text: 'rows, err := db.QueryContext(ctx, sqlStr)\n',
    changedLines: 'all',
  });
  assert.ok(names(positive).includes('go/sql-in-handler'));

  const negative = checkGo({
    path: 'handler.go',
    text: 'q := r.URL.Query()\n',
    changedLines: 'all',
  });
  assert.ok(!names(negative).includes('go/sql-in-handler'));
});

/* ─── ts-patterns ────────────────────────────────────────────────── */

test('i18n/bare-namespace: fires on a .ts file too, not only .tsx', () => {
  const text = "const { t } = useTranslation();\n";
  const findings = checkTs({ path: 'useThing.ts', text, changedLines: 'all' });
  assert.ok(names(findings).includes('i18n/bare-namespace'));
});

test('ui/raw-button: fires on a raw <button and names PlainButton in the message', () => {
  const text = '<button onClick={onClick}>Go</button>\n';
  const findings = checkTs({ path: 'Thing.tsx', text, changedLines: 'all' });
  const finding = findings.find((f) => f.rule === 'ui/raw-button');
  assert.ok(finding, 'expected ui/raw-button finding');
  assert.match(finding.message, /PlainButton/);
});

test('ui/inline-style: fires on style={{}} but not on style={cssVars(...)}', () => {
  const positive = checkTs({ path: 'Thing.tsx', text: 'style={{ width: x }}\n', changedLines: 'all' });
  assert.ok(names(positive).includes('ui/inline-style'));

  const negative = checkTs({ path: 'Thing.tsx', text: 'style={cssVars({ width: x })}\n', changedLines: 'all' });
  assert.ok(!names(negative).includes('ui/inline-style'));
});

test('ui/native-dialog: fires on window.confirm( and window.alert(', () => {
  const confirmFindings = checkTs({ path: 'Thing.tsx', text: "window.confirm('sure?')\n", changedLines: 'all' });
  assert.ok(names(confirmFindings).includes('ui/native-dialog'));
  assert.match(confirmFindings.find((f) => f.rule === 'ui/native-dialog').message, /<ConfirmDialog>/);

  const alertFindings = checkTs({ path: 'Thing.tsx', text: "window.alert('done')\n", changedLines: 'all' });
  assert.ok(names(alertFindings).includes('ui/native-dialog'));
});

test('errors/query-drops-envelope: fires on a queryFn that unwraps .data directly', () => {
  const text = "queryFn: () => api.get('/x').then((r) => r.data)\n";
  const findings = checkTs({ path: 'useThing.ts', text, changedLines: 'all' });
  assert.ok(names(findings).includes('errors/query-drops-envelope'));
  assert.match(findings.find((f) => f.rule === 'errors/query-drops-envelope').message, /assertSuccess/);
});

test('errors/generic-onerror: fires on a generic toast.error onError', () => {
  const text = "onError: () => toast.error('Failed'),\n";
  const findings = checkTs({ path: 'useThing.ts', text, changedLines: 'all' });
  assert.ok(names(findings).includes('errors/generic-onerror'));
  const message = findings.find((f) => f.rule === 'errors/generic-onerror').message;
  assert.match(message, /drop this onError/);
  assert.match(message, /meta[.]error/);
});

test('ts/double-cast: fires on `as unknown as`', () => {
  const text = 'const x = y as unknown as Thing;\n';
  const findings = checkTs({ path: 'Thing.ts', text, changedLines: 'all' });
  assert.ok(names(findings).includes('ts/double-cast'));
});

test('i18n/raw-date-format: fires on toLocaleDateString()', () => {
  const text = 'const s = date.toLocaleDateString();\n';
  const findings = checkTs({ path: 'Thing.tsx', text, changedLines: 'all' });
  assert.ok(names(findings).includes('i18n/raw-date-format'));
  assert.match(findings.find((f) => f.rule === 'i18n/raw-date-format').message, /formatDate/);
});

test('plugins/host-knows-slug: fires on a plugin slug literal in src/frontend/modules/**', () => {
  const text = "const slug = 'addon-kvm';\n";
  const findings = checkTs({
    path: join(REPO, 'src', 'frontend', 'modules', 'environments', 'Thing.tsx'),
    text,
    changedLines: 'all',
  });
  assert.ok(names(findings).includes('plugins/host-knows-slug'));
});

test('plugins/host-knows-slug: does not fire on a plugin naming its own slug', () => {
  const text = "const slug = 'addon-kvm';\n";
  const findings = checkTs({
    path: join(REPO, 'src', 'plugins', 'Plugins', 'addon-kvm', 'frontend', 'src', 'App.tsx'),
    text,
    changedLines: 'all',
  });
  assert.ok(!names(findings).includes('plugins/host-knows-slug'));
});

/* ─── copyright ──────────────────────────────────────────────────── */

test('copyright/missing: fires when a new file has no header', () => {
  const findings = checkCopyright({ path: 'handler_test.go', text: 'package foo\n' });
  assert.ok(names(findings).includes('copyright/missing'));
});

test('copyright/missing: does not fire when the header is present', () => {
  const text =
    '// Copyright (c) 2026 McSparrow. All rights reserved.\n' +
    '// McHarbor is licensed under the McHarbor License. See LICENSE for details.\n' +
    'package foo\n';
  const findings = checkCopyright({ path: 'handler_test.go', text });
  assert.ok(!names(findings).includes('copyright/missing'));
});

/* ─── locale-json ────────────────────────────────────────────────── */

test('i18n/mojibake: fires on a double-encoded locale value', () => {
  const text = '{\n  "title": "Donâ€™t panic"\n}\n';
  const findings = checkLocale({ text, changedLines: 'all' });
  assert.ok(names(findings).includes('i18n/mojibake'));
});

/* ─── lib/payload: changedLines ─────────────────────────────────── */

test('changedLines: Write always reports "all"', () => {
  const result = changedLines({ tool_name: 'Write', tool_input: { file_path: 'x.go' } }, 'anything\n');
  assert.equal(result, 'all');
});

test('changedLines: Edit locates new_string and only that line is inChanged', () => {
  const postText = 'a\nb\nc\nd\n';
  const result = changedLines(
    { tool_name: 'Edit', tool_input: { file_path: 'x.go', new_string: 'c', old_string: 'z' } },
    postText,
  );
  assert.notEqual(result, 'all');
  assert.ok(inChanged(result, 3));
  assert.ok(!inChanged(result, 1));
});

test('changedLines: falls back to "all" when the snippet cannot be located', () => {
  const result = changedLines(
    { tool_name: 'Edit', tool_input: { file_path: 'x.go', new_string: 'not-in-file', old_string: 'z' } },
    'a\nb\n',
  );
  assert.equal(result, 'all');
});

test('changedLines: MultiEdit unions every edit\'s located lines', () => {
  const postText = 'a\nb\nc\n';
  const result = changedLines(
    {
      tool_name: 'MultiEdit',
      tool_input: { file_path: 'x.go', edits: [{ new_string: 'a' }, { new_string: 'c' }] },
    },
    postText,
  );
  assert.ok(inChanged(result, 1));
  assert.ok(inChanged(result, 3));
  assert.ok(!inChanged(result, 2));
});

/* ─── lib/payload: isNewFile ─────────────────────────────────────── */

test('isNewFile: true when tool_response.type is "create"', () => {
  assert.equal(isNewFile({ tool_response: { type: 'create' } }), true);
});

test('isNewFile: false when tool_response.type is "update"', () => {
  assert.equal(isNewFile({ tool_response: { type: 'update' } }), false);
});

/* ─── lint-edited.mjs: end-to-end via run() ─────────────────────── */

test('run(): reports copyright/missing for a Write of a new file with no header', () => {
  const dir = mkdtempSync(join(tmpdir(), 'lint-edited-'));
  try {
    const file = join(dir, 'handler_new_test.go');
    writeFileSync(file, 'package foo\n');
    const resultsDir = join(dir, '.results', 'hooks');
    const payload = {
      session_id: 'test-session',
      cwd: dir,
      tool_name: 'Write',
      tool_input: { file_path: file, content: 'package foo\n' },
      tool_response: { type: 'create' },
    };
    const out = run(payload);
    assert.match(out, /copyright\/missing/);
    assert.match(out, /handler_new_test\.go:1/);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test('run(): records the touched file to .results/hooks/<session_id>.touched', () => {
  const dir = mkdtempSync(join(tmpdir(), 'lint-edited-'));
  try {
    const file = join(dir, 'plain.go');
    writeFileSync(file, 'package foo\n// Copyright (c) 2026 McSparrow. All rights reserved.\n');
    const payload = {
      session_id: 'touch-session',
      cwd: dir,
      tool_name: 'Edit',
      tool_input: { file_path: file, old_string: 'package foo', new_string: 'package foo' },
    };
    run(payload);
    const touchedFile = join(dir, '.results', 'hooks', 'touch-session.touched');
    const content = readFileSync(touchedFile, 'utf8');
    assert.ok(content.includes(file));
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test('run(): formats before linting (parallel-hook fix) and still scopes findings to the edited line after gofmt reindents it', () => {
  const dir = mkdtempSync(join(tmpdir(), 'lint-edited-'));
  try {
    const file = join(dir, 'store.go');
    // func A's ignored-error line is pre-existing, already gofmt-clean
    // (tab-indented) and untouched by this edit. func B's is the "just
    // edited" line — space-indented, which gofmt rewrites to a tab — so
    // new_string (space-indented) no longer appears verbatim in the
    // post-format text and changedLinesAcrossFormat must fall back to
    // locating it in the pre-format text.
    const content =
      'package foo\n\nfunc A() {\n\t_, _ = doSomething()\n}\n\nfunc B() {\n    _, _ = another()\n}\n';
    writeFileSync(file, content);
    const payload = {
      session_id: 'gofmt-session',
      cwd: dir,
      tool_name: 'Edit',
      tool_input: {
        file_path: file,
        old_string: 'func B() {\n}\n',
        new_string: 'func B() {\n    _, _ = another()\n}\n',
      },
    };
    const out = run(payload);
    // The edited line (func B, line 8) is reported...
    assert.match(out, /store\.go:8 \[go\/ignored-error\]/);
    // ...but the pre-existing, untouched line (func A, line 4) is not —
    // proving this scoped to the edit rather than falling back to 'all'.
    assert.doesNotMatch(out, /store\.go:4 \[go\/ignored-error\]/);
    // And formatting actually ran from inside run(), not a separate hook:
    // the on-disk file now uses a tab on line 8, not the original spaces.
    const formatted = readFileSync(file, 'utf8');
    assert.ok(formatted.split('\n')[7].startsWith('\t'), 'expected gofmt to have reindented line 8 with a tab');
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

/* ─── proof: synthetic stdin payload emits the correct hook JSON shape ── */

test('CLI: a synthetic PostToolUse Edit payload on stdin emits hookSpecificOutput JSON', () => {
  const dir = mkdtempSync(join(tmpdir(), 'lint-edited-cli-'));
  try {
    const file = join(dir, 'thing.ts');
    writeFileSync(file, "const { t } = useTranslation();\n");
    const payload = {
      session_id: 'cli-session',
      cwd: dir,
      tool_name: 'Edit',
      tool_input: {
        file_path: file,
        old_string: 'const { t } = useTranslation();',
        new_string: 'const { t } = useTranslation();',
      },
    };
    const result = spawnSync(process.execPath, [LINT_SCRIPT], {
      input: JSON.stringify(payload),
      encoding: 'utf8',
    });
    assert.equal(result.status, 0);
    const parsed = JSON.parse(result.stdout);
    assert.equal(parsed.hookSpecificOutput.hookEventName, 'PostToolUse');
    assert.match(parsed.hookSpecificOutput.additionalContext, /i18n\/bare-namespace/);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test('CLI: a payload with no findings prints nothing and still exits 0', () => {
  const dir = mkdtempSync(join(tmpdir(), 'lint-edited-cli-'));
  try {
    const file = join(dir, 'clean.ts');
    writeFileSync(file, "const x = 1;\n");
    const payload = {
      session_id: 'cli-session-2',
      cwd: dir,
      tool_name: 'Edit',
      tool_input: { file_path: file, old_string: 'const x = 1;', new_string: 'const x = 1;' },
    };
    const result = spawnSync(process.execPath, [LINT_SCRIPT], {
      input: JSON.stringify(payload),
      encoding: 'utf8',
    });
    assert.equal(result.status, 0);
    assert.equal(result.stdout, '');
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

/* ─── Task 2 additions: agent attribution and changed-line records ─── */

test('recordTouched: tags the entry with agent_id when the payload carries one', () => {
  const dir = mkdtempSync(join(tmpdir(), 'lint-edited-agent-'));
  try {
    const file = join(dir, 'a.go');
    recordTouched({ session_id: 's1', cwd: dir, agent_id: 'agent-42' }, file);
    recordTouched({ session_id: 's1', cwd: dir }, file);
    const lines = readFileSync(join(dir, '.results', 'hooks', 's1.touched'), 'utf8').split('\n');
    assert.equal(lines[0], `${file}\tagent-42`);
    assert.equal(lines[1], file);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test('hooksDir: resolves to the git top-level when the payload cwd is a subdirectory', () => {
  const dir = mkdtempSync(join(tmpdir(), 'lint-edited-top-'));
  try {
    mkdirSync(join(dir, '.git'));
    mkdirSync(join(dir, 'src', 'frontend'), { recursive: true });
    assert.equal(hooksDir({ cwd: join(dir, 'src', 'frontend') }), join(dir, '.results', 'hooks'));
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test('run(): appends a lines.jsonl record with the edited line text and the agent_id', () => {
  const dir = mkdtempSync(join(tmpdir(), 'lint-edited-lines-'));
  try {
    const file = join(dir, 'notes.txt');
    writeFileSync(file, 'alpha\nbravo changed\ncharlie\n');
    run({
      session_id: 'lines-session',
      cwd: dir,
      agent_id: 'agent-7',
      tool_name: 'Edit',
      tool_input: { file_path: file, old_string: 'bravo', new_string: 'bravo changed' },
    });
    const raw = readFileSync(join(dir, '.results', 'hooks', 'lines-session.lines.jsonl'), 'utf8').trim();
    const rec = JSON.parse(raw);
    assert.equal(rec.file, file);
    assert.equal(rec.all, false);
    assert.deepEqual(rec.texts, ['bravo changed']);
    assert.equal(rec.agent, 'agent-7');
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

function lastRecord(dir, session) {
  const lines = readFileSync(join(dir, '.results', 'hooks', `${session}.lines.jsonl`), 'utf8').trim().split('\n');
  return JSON.parse(lines[lines.length - 1]);
}

test('run(): a Write that creates a file records all:true in lines.jsonl', () => {
  const dir = mkdtempSync(join(tmpdir(), 'lint-edited-lines-'));
  try {
    const file = join(dir, 'notes.txt');
    writeFileSync(file, 'one\n');
    run({
      session_id: 'w-session',
      cwd: dir,
      tool_name: 'Write',
      tool_input: { file_path: file, content: 'one\n' },
      tool_response: { type: 'create', originalFile: null, structuredPatch: [] },
    });
    const rec = lastRecord(dir, 'w-session');
    assert.equal(rec.all, true);
    assert.equal(rec.agent, null);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test('run(): a Write over an existing file records only the structuredPatch additions, not all:true', () => {
  const dir = mkdtempSync(join(tmpdir(), 'lint-edited-lines-'));
  try {
    const file = join(dir, 'notes.txt');
    writeFileSync(file, 'one\ntwo changed\nthree\n');
    run({
      session_id: 'wu-session',
      cwd: dir,
      tool_name: 'Write',
      tool_input: { file_path: file, content: 'one\ntwo changed\nthree\n' },
      tool_response: {
        type: 'update',
        originalFile: 'one\ntwo\nthree\n',
        structuredPatch: [{ oldStart: 1, oldLines: 3, newStart: 1, newLines: 3, lines: [' one', '-two', '+two changed', ' three'] }],
      },
    });
    const rec = lastRecord(dir, 'wu-session');
    assert.equal(rec.all, false);
    assert.deepEqual(rec.texts, ['two changed']);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test('run(): a Write update with only originalFile diffs it against the new content', () => {
  const dir = mkdtempSync(join(tmpdir(), 'lint-edited-lines-'));
  try {
    const file = join(dir, 'notes.txt');
    writeFileSync(file, 'one\nnew line\nthree\n');
    run({
      session_id: 'wo-session',
      cwd: dir,
      tool_name: 'Write',
      tool_input: { file_path: file, content: 'one\nnew line\nthree\n' },
      tool_response: { type: 'update', originalFile: 'one\nthree\n' },
    });
    const rec = lastRecord(dir, 'wo-session');
    assert.equal(rec.all, false);
    assert.deepEqual(rec.texts, ['new line']);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test('run(): a Write with no tool_response, or an Edit whose new_string cannot be located, records unknown — never all', () => {
  const dir = mkdtempSync(join(tmpdir(), 'lint-edited-lines-'));
  try {
    const file = join(dir, 'notes.txt');
    writeFileSync(file, 'alpha\n');
    run({ session_id: 'u-session', cwd: dir, tool_name: 'Write', tool_input: { file_path: file, content: 'alpha\n' } });
    let rec = lastRecord(dir, 'u-session');
    assert.equal(rec.all, false);
    assert.equal(rec.unknown, true);
    run({
      session_id: 'u-session',
      cwd: dir,
      tool_name: 'Edit',
      tool_input: { file_path: file, old_string: 'x', new_string: 'not in the file' },
    });
    rec = lastRecord(dir, 'u-session');
    assert.equal(rec.all, false);
    assert.equal(rec.unknown, true);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});
