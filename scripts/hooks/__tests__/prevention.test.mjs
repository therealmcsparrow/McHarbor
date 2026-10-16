// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

import { check as checkTs, removedKeys, unreferencedKeys, clearCodeIndex } from '../rules/ts-patterns.mjs';
import { check as checkLocale } from '../rules/locale-json.mjs';
import { runChecks, classify } from '../gate-touched.mjs';
import { run } from '../lint-edited.mjs';

const HERE = fileURLToPath(new URL('.', import.meta.url));
const REPO = join(HERE, '..', '..', '..');
const VERIFY_KEYS = join(REPO, 'scripts', 'sanity', 'i18n', 'verify-keys.mjs');

function tempRoot(prefix = 'prevention-') {
  const root = mkdtempSync(join(tmpdir(), prefix));
  mkdirSync(join(root, '.results', 'hooks'), { recursive: true });
  return root;
}

function put(root, rel, text) {
  clearCodeIndex();
  const file = join(root, ...rel.split('/'));
  mkdirSync(join(file, '..'), { recursive: true });
  writeFileSync(file, text);
  return file;
}

function rules(findings) {
  return findings.map((f) => f.rule);
}

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

const HEADER = '// Copyright (c) 2026 McSparrow. All rights reserved.\n// McHarbor is licensed under the McHarbor License. See LICENSE for details.\n';

/* ─── ts-patterns: comments ──────────────────────────────────────── */

test('i18n/bare-namespace: useTranslation() inside a line or block comment is not a call', () => {
  const text = [
    '// The host binds i18n, so useTranslation() calls inside ConfigForm resolve.',
    '/**',
    ' * useTranslation() here would be bare.',
    ' */',
    "const url = 'http://example'; // useTranslation() in a trailing comment",
  ].join('\n');
  const findings = checkTs({ path: 'src/plugins/Plugins/comm-x/frontend/src/index.tsx', text, changedLines: 'all' });
  assert.ok(!rules(findings).includes('i18n/bare-namespace'), JSON.stringify(findings));
});

test('i18n/bare-namespace: still fires on a real bare call after a comment on the same line', () => {
  const text = '/* a */ const { t } = useTranslation();\n';
  const findings = checkTs({ path: 'src/frontend/modules/a/A.tsx', text, changedLines: 'all' });
  assert.ok(rules(findings).includes('i18n/bare-namespace'));
});

/* ─── ts-patterns: re-export shim ────────────────────────────────── */

test('ts/reexport-shim: a touched file that only re-exports and that nothing imports is a finding', () => {
  const root = tempRoot();
  try {
    put(root, 'src/frontend/resources/components/abac/Thing.tsx', `${HEADER}export function Thing() { return null; }\n`);
    put(root, 'src/frontend/modules/abac/pages/Page.tsx', `${HEADER}import { Thing } from '@resources/components/abac/Thing';\n`);
    const shimText = `${HEADER}export * from '@resources/components/abac/Thing';\n`;
    const shim = put(root, 'src/frontend/modules/abac/components/Thing.tsx', shimText);
    const findings = checkTs({ path: shim, text: shimText, changedLines: 'all', root });
    const f = findings.find((x) => x.rule === 'ts/reexport-shim');
    assert.ok(f, JSON.stringify(findings));
    assert.equal(f.severity, 'error');
    assert.equal(f.line, 3);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('ts/reexport-shim: no finding when something imports the shim (relative or alias), or the file has real code', () => {
  const root = tempRoot();
  try {
    const shimText = `${HEADER}export {\n  Thing,\n  type ThingProps,\n} from '@resources/components/abac/Thing';\n`;
    const shim = put(root, 'src/frontend/modules/abac/components/Thing.tsx', shimText);
    put(root, 'src/frontend/modules/abac/pages/Page.tsx', `${HEADER}import { Thing } from '../components/Thing';\n`);
    assert.ok(!rules(checkTs({ path: shim, text: shimText, changedLines: 'all', root })).includes('ts/reexport-shim'));

    const shim2Text = `${HEADER}export * from './impl';\n`;
    const shim2 = put(root, 'src/frontend/modules/abac/hooks/index.ts', shim2Text);
    put(root, 'src/frontend/modules/abac/pages/Other.tsx', `${HEADER}import { useX } from '@modules/abac/hooks';\n`);
    assert.ok(!rules(checkTs({ path: shim2, text: shim2Text, changedLines: 'all', root })).includes('ts/reexport-shim'));

    const realText = `${HEADER}export * from './impl';\nexport const x = 1;\n`;
    const real = put(root, 'src/frontend/modules/abac/hooks/mixed.ts', realText);
    assert.ok(!rules(checkTs({ path: real, text: realText, changedLines: 'all', root })).includes('ts/reexport-shim'));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('ts/reexport-shim: an untouched shim (no changed line) is not reported', () => {
  const root = tempRoot();
  try {
    const shimText = `${HEADER}export * from '@resources/components/abac/Thing';\n`;
    const shim = put(root, 'src/frontend/modules/abac/components/Thing.tsx', shimText);
    assert.ok(!rules(checkTs({ path: shim, text: shimText, changedLines: new Set([99]), root })).includes('ts/reexport-shim'));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

/* ─── ts-patterns: removed keys ──────────────────────────────────── */

test('removedKeys / unreferencedKeys: a t() key dropped from the diff and referenced nowhere else is unused', () => {
  const root = tempRoot();
  try {
    put(root, 'src/frontend/modules/a/Other.tsx', `${HEADER}t('kept.elsewhere');\n`);
    put(root, 'src/backend/modules/a/codes.go', 'package a\n\nconst reason = "backend.reason"\n');
    const before = "t('gone.key'); t('common:kept.elsewhere'); t('backend.reason'); t('still.here');";
    const after = "t('still.here');";
    const removed = removedKeys(before, after);
    assert.deepEqual([...removed].sort(), ['backend.reason', 'gone.key', 'kept.elsewhere']);
    const { unused, complete } = unreferencedKeys(root, removed);
    assert.equal(complete, true);
    assert.deepEqual(unused, ['gone.key']);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('lint-edited run(): an Edit that removes the last t() reference to a key warns i18n/key-unused', () => {
  const root = tempRoot('prevention-lint-');
  try {
    mkdirSync(join(root, '.git'));
    const file = put(root, 'src/frontend/modules/a/A.tsx', `${HEADER}export const x = t('fresh.key');\n`);
    const out = run({
      session_id: 'k',
      cwd: root,
      tool_name: 'Edit',
      tool_input: { file_path: file, old_string: "t('old.label')", new_string: "t('fresh.key')" },
    });
    assert.match(out, /i18n\/key-unused/);
    assert.match(out, /old\.label/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

/* ─── locale-json: untranslated changed values ───────────────────── */

function localeRoot() {
  const root = tempRoot('prevention-locale-');
  put(root, 'src/frontend/core/i18n/locales/en/demo.json', [
    '{',
    '    "dialog":  {',
    '        "body":  "Remove this backup from the remote target",',
    '        "short":  "Overview",',
    '        "brand":  "Docker",',
    '        "frozen":  "Save and restart now"',
    '    },',
    '    "done":  "Translated already"',
    '}',
    '',
  ].join('\n'));
  put(root, '.instructions/sanity/baselines/i18n-untranslated.txt', '# header\nnl|demo|dialog.frozen\n');
  put(root, 'scripts/sanity/i18n-untranslated/check.ps1', "$dntWords = @(\n  'Docker', 'Kubernetes'\n)\n");
  return root;
}

test('i18n/untranslated: a changed nl value identical to en blocks at 3+ words, warns at 1-2, skips machine and baselined values', () => {
  const root = localeRoot();
  try {
    const text = [
      '{',
      '    "dialog":  {',
      '        "body":  "Remove this backup from the remote target",',
      '        "short":  "Overview",',
      '        "brand":  "Docker",',
      '        "frozen":  "Save and restart now"',
      '    },',
      '    "done":  "Al vertaald"',
      '}',
      '',
    ].join('\n');
    const path = put(root, 'src/frontend/core/i18n/locales/nl/demo.json', text);
    const findings = checkLocale({ path, text, changedLines: 'all' }).filter((f) => f.rule === 'i18n/untranslated');
    const byLine = Object.fromEntries(findings.map((f) => [f.line, f.severity]));
    assert.deepEqual(byLine, { 3: 'error', 4: 'warn' });
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('i18n/untranslated: only changed lines count, and the en bundle itself is never checked', () => {
  const root = localeRoot();
  try {
    const nlText = '{\n    "dialog":  {\n        "body":  "Remove this backup from the remote target"\n    }\n}\n';
    const nl = put(root, 'src/frontend/core/i18n/locales/nl/demo.json', nlText);
    assert.equal(checkLocale({ path: nl, text: nlText, changedLines: new Set([1]) }).length, 0);
    const en = join(root, 'src', 'frontend', 'core', 'i18n', 'locales', 'en', 'demo.json');
    assert.equal(checkLocale({ path: en, text: nlText, changedLines: 'all' }).length, 0);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('i18n/untranslated: a plugin bundle compares against its own en and uses the group-qualified baseline namespace', () => {
  const root = localeRoot();
  try {
    put(root, 'src/plugins/Plugins/addon-x/i18n/en/vm.json', '{\n    "a":  "Start the virtual machine now",\n    "b":  "Stop the virtual machine now"\n}\n');
    put(root, '.instructions/sanity/baselines/i18n-untranslated.txt', 'de|src/plugins/Plugins/addon-x/i18n/vm|b\n');
    const text = '{\n    "a":  "Start the virtual machine now",\n    "b":  "Stop the virtual machine now"\n}\n';
    const path = put(root, 'src/plugins/Plugins/addon-x/i18n/de/vm.json', text);
    const findings = checkLocale({ path, text, changedLines: 'all' }).filter((f) => f.rule === 'i18n/untranslated');
    assert.deepEqual(findings.map((f) => f.line), [2]);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

/* ─── gate: sanity set and duplication ───────────────────────────── */

test('gate: the scoped sanity run includes modularity and no longer passes duplication file paths', async () => {
  const root = tempRoot();
  try {
    put(root, 'scripts/sanity/run-all.ps1', '#\n');
    const file = put(root, 'src/frontend/modules/a/A.tsx', `${HEADER}export const a = 1;\n`);
    const { runner, calls } = fakeRunner({ sanity: () => ({ code: 0 }) });
    await runChecks({ root, files: [file], runner, deadline: Date.now() + 10_000, sessionId: 's' });
    const sanity = calls.find((c) => c.label === 'sanity');
    assert.ok(sanity, 'expected a sanity call');
    const checks = sanity.args[sanity.args.indexOf('-Check') + 1].split(',');
    assert.ok(checks.includes('modularity'), checks.join(','));
    assert.ok(!checks.includes('duplication'), checks.join(','));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

function dupRoot() {
  const root = tempRoot();
  put(root, 'scripts/sanity/duplication/detect.mjs', '// stub\n');
  const agent = put(root, 'src/agent/twin_test.go', 'package main\n');
  const host = put(root, 'src/backend/modules/system/twin_test.go', 'package system\n');
  const other = put(root, 'src/backend/modules/other/x.go', 'package other\n');
  return { root, agent, host, other };
}

const DRIFT_JSON = JSON.stringify({
  failing: [],
  extended: [],
  drifted: [
    {
      family: 'agent-twin',
      reference: 'src/backend/modules/system/twin_test.go',
      members: [{ path: 'src/agent/twin_test.go', similarity: 0.7313, baseline: 0.6111, delta: 0.1202, drifted: true }],
      drifted: [{ path: 'src/agent/twin_test.go', similarity: 0.7313, baseline: 0.6111, delta: 0.1202, drifted: true }],
    },
  ],
});

test('gate: duplication runs over the whole tree and a drifted family with a touched member is a blocking file-level finding', async () => {
  const { root, agent } = dupRoot();
  try {
    const { runner, calls } = fakeRunner({ duplication: () => ({ code: 0, stdout: DRIFT_JSON }) });
    const result = await runChecks({ root, files: [agent], runner, deadline: Date.now() + 10_000, sessionId: 's' });
    const call = calls.find((c) => c.label === 'duplication');
    assert.ok(call, 'expected a duplication call');
    assert.ok(!call.args.includes('--paths'), 'duplication must not be scoped to the touched files');
    const f = result.findings.find((x) => x.rule === 'duplication/drift');
    assert.ok(f, JSON.stringify(result.findings));
    assert.equal(f.file, agent);
    assert.equal(f.severity, 'error');
    const changed = new Map([[agent, new Set([1])]]);
    const { blocking } = classify(result.findings, changed, new Set([agent]));
    assert.ok(blocking.some((b) => b.rule === 'duplication/drift'));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('gate: a drifted family none of whose members was touched produces no finding', async () => {
  const { root, other } = dupRoot();
  try {
    const { runner } = fakeRunner({ duplication: () => ({ code: 0, stdout: DRIFT_JSON }) });
    const result = await runChecks({ root, files: [other], runner, deadline: Date.now() + 10_000, sessionId: 's' });
    assert.ok(!result.findings.some((x) => x.tool === 'duplication'));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('gate: a removed t() key with no other reference warns from the before-text the caller supplies', async () => {
  const root = tempRoot();
  try {
    const file = put(root, 'src/frontend/modules/a/A.tsx', `${HEADER}export const x = t('kept.key');\n`);
    const { runner } = fakeRunner();
    const beforeText = () => `${HEADER}export const x = t('kept.key') + t('dropped.key');\n`;
    const result = await runChecks({ root, files: [file], runner, deadline: Date.now() + 10_000, sessionId: 's', beforeText });
    const f = result.findings.find((x) => x.rule === 'i18n/key-unused');
    assert.ok(f, JSON.stringify(result.findings));
    assert.equal(f.severity, 'warn');
    assert.match(f.message, /dropped\.key/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

/* ─── verify-keys ────────────────────────────────────────────────── */

function verifyRoot() {
  const root = tempRoot('prevention-vk-');
  put(root, 'src/frontend/core/i18n/locales/en/demo.json', '{\n    "columns":  {\n        "exit":  "exit",\n        "time":  "Time",\n        "kind":  "kind"\n    }\n}\n');
  put(root, 'scripts/sanity/i18n/echo-accepted.json', '{\n  "demo:columns.kind": { "value": "kind", "reason": "mid-sentence noun" }\n}\n');
  put(root, 'src/plugins/Plugins/storage-ftp/plugin.yaml', 'slug: storage-ftp\ni18n_namespaces:\n  - "storage-ftp"\n  - "ftpextra"\n');
  put(root, 'src/plugins/Plugins/storage-ftp/i18n/en/common.json', '{\n    "config":  {\n        "heading":  "FTP server"\n    }\n}\n');
  return root;
}

function verify(root, file) {
  const res = spawnSync(process.execPath, [VERIFY_KEYS, '--json', file], {
    encoding: 'utf8',
    env: { ...process.env, MCHARBOR_VERIFY_KEYS_ROOT: root },
  });
  return JSON.parse(res.stdout || '[]');
}

test('verify-keys: an en value equal to its own last key segment is an echo warning unless echo-accepted lists it', () => {
  const root = verifyRoot();
  try {
    const file = put(root, 'src/frontend/modules/a/A.tsx', `${HEADER}const { t } = useTranslation('demo');\nt('columns.exit'); t('columns.time'); t('columns.kind');\n`);
    const records = verify(root, file);
    const echoes = records.filter((r) => r.kind === 'echo');
    assert.deepEqual(echoes.map((r) => r.key), ['columns.exit']);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('verify-keys: a plugin namespace declared in plugin.yaml resolves against the plugin\'s i18n/en bundles', () => {
  const root = verifyRoot();
  try {
    const file = put(
      root,
      'src/plugins/Plugins/storage-ftp/frontend/src/index.tsx',
      `${HEADER}const { t } = useTranslation('storage-ftp');\nt('config.heading');\nconst { t: tx } = useTranslation('ftpextra');\ntx('config.heading');\n`,
    );
    const records = verify(root, file);
    assert.deepEqual(records.filter((r) => r.kind === 'namespace' || r.kind === 'missing'), []);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
