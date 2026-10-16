// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

import { isExcluded, formatFile } from '../format-edited.mjs';

const UNFORMATTED_TS = 'let x=1\n';

function withTempDir(fn) {
  const dir = mkdtempSync(join(tmpdir(), 'mcharbor-format-edited-'));
  try {
    return fn(dir);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

// Windows delivers tool_input.file_path with backslashes and an uppercase
// drive letter; the bash version's forward-slash-only glob missed every one
// of these and let prettier reformat locale JSON and plugin files.

test('isExcluded: locale JSON is excluded, backslash and forward-slash alike', () => {
  assert.equal(
    isExcluded('Z:\\projects\\web\\management\\McHarbor\\src\\frontend\\core\\i18n\\locales\\en\\common.json'),
    true,
  );
  assert.equal(isExcluded('src/frontend/core/i18n/locales/en/common.json'), true);
});

test('isExcluded: widget/node i18n JSON is excluded, backslash and forward-slash alike', () => {
  assert.equal(isExcluded('Z:\\projects\\web\\management\\McHarbor\\widgets\\foo\\i18n\\en.json'), true);
  assert.equal(isExcluded('widgets/foo/i18n/en.json'), true);
});

test('isExcluded: a .umd.cjs bundle is excluded, backslash and forward-slash alike', () => {
  assert.equal(
    isExcluded('Z:\\projects\\web\\management\\McHarbor\\src\\plugins\\Plugins\\addon-kvm\\frontend\\addon-kvm.umd.cjs'),
    true,
  );
  assert.equal(isExcluded('src/plugins/Plugins/addon-kvm/frontend/addon-kvm.umd.cjs'), true);
});

test('isExcluded: a plugin backend is excluded, its frontend/src is not, backslash and forward-slash alike', () => {
  assert.equal(isExcluded('Z:\\projects\\web\\management\\McHarbor\\src\\plugins\\Plugins\\addon-kvm\\backend\\main.go'), true);
  assert.equal(isExcluded('src/plugins/Plugins/addon-kvm/backend/main.go'), true);
  assert.equal(
    isExcluded('Z:\\projects\\web\\management\\McHarbor\\src\\plugins\\Plugins\\addon-kvm\\frontend\\src\\App.tsx'),
    false,
  );
  assert.equal(isExcluded('src/plugins/Plugins/addon-kvm/frontend/src/App.tsx'), false);
});

test('isExcluded: a normal src/frontend .tsx is NOT excluded, backslash and forward-slash alike', () => {
  assert.equal(
    isExcluded('Z:\\projects\\web\\management\\McHarbor\\src\\frontend\\modules\\environments\\Thing.tsx'),
    false,
  );
  assert.equal(isExcluded('src/frontend/modules/environments/Thing.tsx'), false);
});

test('isExcluded: is case-insensitive about the drive letter', () => {
  assert.equal(
    isExcluded('z:\\projects\\web\\management\\McHarbor\\src\\frontend\\core\\i18n\\locales\\en\\common.json'),
    true,
  );
});

// formatFile hands frontend files to `vp fmt` from src/frontend, where the fmt config lives. A
// fake vp stands in for the real one: it records its arguments into the file it was given.

const FAKE_VP = `const { appendFileSync } = require('node:fs');
appendFileSync(process.argv[process.argv.length - 1], '// formatted ' + process.argv.slice(2, -1).join(' ') + '\\n');
`;

function withFakeRepo(fn) {
  return withTempDir((root) => {
    const bin = join(root, 'src', 'frontend', 'node_modules', 'vite-plus', 'bin');
    mkdirSync(bin, { recursive: true });
    writeFileSync(join(bin, 'vp'), FAKE_VP);
    writeFileSync(join(root, 'src', 'frontend', 'package.json'), '{"type":"commonjs"}\n');
    writeFileSync(join(bin, 'package.json'), '{"type":"commonjs"}\n');
    return fn(root);
  });
}

function put(root, rel, text = UNFORMATTED_TS) {
  const file = join(root, ...rel.split('/'));
  mkdirSync(join(file, '..'), { recursive: true });
  writeFileSync(file, text);
  return file;
}

test('formatFile: runs vp fmt on an in-tree frontend file', () => {
  withFakeRepo((root) => {
    const file = put(root, 'src/frontend/modules/x/Thing.ts');
    formatFile(file, root);
    assert.equal(readFileSync(file, 'utf8'), `${UNFORMATTED_TS}// formatted fmt\n`);
  });
});

test('formatFile: formats widget, node and plugin frontend sources, but not their JSON', () => {
  withFakeRepo((root) => {
    const sources = ['widgets/w/W.tsx', 'nodes/n/index.ts', 'src/plugins/Plugins/addon-x/frontend/src/Page.tsx'].map((rel) => put(root, rel));
    const json = put(root, 'nodes/n/schema.json', '{}\n');
    for (const file of [...sources, json]) formatFile(file, root);
    for (const file of sources) assert.match(readFileSync(file, 'utf8'), /\/\/ formatted fmt/);
    assert.equal(readFileSync(json, 'utf8'), '{}\n');
  });
});

test('formatFile: leaves files outside the formatted trees alone', () => {
  withFakeRepo((root) => {
    const file = put(root, 'scripts/tool.ts');
    formatFile(file, root);
    assert.equal(readFileSync(file, 'utf8'), UNFORMATTED_TS);
  });
});

test('formatFile: does nothing when vite-plus is not installed', () => {
  withTempDir((root) => {
    const file = put(root, 'src/frontend/modules/x/Thing.ts');
    formatFile(file, root);
    assert.equal(readFileSync(file, 'utf8'), UNFORMATTED_TS);
  });
});
