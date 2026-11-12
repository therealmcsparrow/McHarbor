// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';

import { scan, check } from '../check-umd-bundle.mjs';

const CLEAN = `globalThis.process={env:{NODE_ENV:'production'}};(function(e,t){e.McharborPlugin_x=t(require("react"))})(this,function(r){return typeof process<'u'&&process.env&&process.env.I18NEXT_NO_SUPPORT_NOTICE,{mount:function(){}}});`;

test('a clean bundle has no findings', () => {
  assert.deepEqual(scan(CLEAN), []);
});

test('the unresolved Vite glob is flagged with its offset', () => {
  const src = `${CLEAN}var m={}.glob("./locales/*.json",{eager:!0});`;
  const found = scan(src);
  assert.equal(found.length, 1);
  assert.equal(found[0].id, 'unresolved-glob');
  assert.equal(found[0].offset, src.indexOf('{}.glob('));
});

test('a literal import.meta.glob and import.meta.env are flagged', () => {
  const ids = scan('a=import.meta.glob("x");b=import.meta.env.DEV;').map((f) => f.id);
  assert.deepEqual(ids, ['unresolved-glob', 'import-meta', 'import-meta']);
});

test('requiring a Node builtin is flagged, requiring react is not', () => {
  assert.equal(scan('require("fs")').length, 1);
  assert.equal(scan("require('node:path')").length, 1);
  assert.equal(scan('require("react");require("react-dom/client")').length, 0);
});

test('the CLI exits 1 on a broken bundle and 0 on a clean one', () => {
  const dir = mkdtempSync(join(tmpdir(), 'umd-'));
  try {
    const bad = join(dir, 'bad.umd.cjs');
    const good = join(dir, 'good.umd.cjs');
    writeFileSync(bad, `${CLEAN}var m={}.glob("./a/*.json");`);
    writeFileSync(good, CLEAN);
    const script = new URL('../check-umd-bundle.mjs', import.meta.url);
    const run = (f) => spawnSync(process.execPath, [script.pathname.replace(/^\/([A-Za-z]:)/, '$1'), f], { encoding: 'utf8' });
    const b = run(bad);
    assert.equal(b.status, 1);
    assert.match(b.stderr, /bad\.umd\.cjs:\d+ {2}unresolved-glob/);
    assert.equal(run(good).status, 0);
    assert.equal(check([join(dir, 'missing.umd.cjs')], () => {}), 2);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});
