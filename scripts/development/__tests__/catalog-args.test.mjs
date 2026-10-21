// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';

const script = resolve(dirname(fileURLToPath(import.meta.url)), '..', 'catalog.sh');

function dryRun(args, env = {}) {
  const out = mkdtempSync(join(tmpdir(), 'catalog-args-'));
  try {
    const run = spawnSync('bash', [script, 'build', 'fictional-plugin', '--dry-run', ...args], {
      encoding: 'utf8',
      env: {
        ...process.env,
        NO_COLOR: '1',
        MCHARBOR_PLUGIN_OUTPUT_DIR: out,
        MCHARBOR_CATALOG_JOBS: '',
        MCHARBOR_CATALOG_TIMEOUT_MIN: '',
        MCHARBOR_PLUGIN_NO_CACHE: '',
        ...env,
      },
    });
    return { code: run.status, out: run.stdout, err: run.stderr };
  } finally {
    rmSync(out, { recursive: true, force: true });
  }
}

describe('catalog.sh build flags', () => {
  it('defaults to six jobs, a ten minute timeout and the BuildKit cache', () => {
    const r = dryRun([]);
    assert.equal(r.code, 0, r.err);
    assert.match(r.out, /jobs: 6, timeout per plugin: 10m/);
    assert.match(r.out, /cache: on/);
    assert.match(r.out, /dry run: fictional-plugin/);
  });

  it('takes -j and --jobs as the number of parallel builds', () => {
    assert.match(dryRun(['-j', '3']).out, /jobs: 3,/);
    assert.match(dryRun(['--jobs', '2']).out, /jobs: 2,/);
  });

  it('takes --timeout in minutes, and 0 means no limit', () => {
    assert.match(dryRun(['--timeout', '25']).out, /timeout per plugin: 25m/);
    assert.match(dryRun(['--timeout', '0']).out, /timeout per plugin: none/);
  });

  it('turns the BuildKit cache off with --no-cache', () => {
    assert.match(dryRun(['--no-cache']).out, /cache: off/);
  });

  it('reads the defaults from the environment when no flag is given', () => {
    const r = dryRun([], { MCHARBOR_CATALOG_JOBS: '4', MCHARBOR_CATALOG_TIMEOUT_MIN: '15' });
    assert.match(r.out, /jobs: 4, timeout per plugin: 15m/);
  });

  it('refuses a jobs count or a timeout that is not a number', () => {
    for (const args of [['-j', '0'], ['-j', 'many'], ['--timeout', 'soon']]) {
      const r = dryRun(args);
      assert.notEqual(r.code, 0, args.join(' '));
      assert.match(r.err, /needs/);
    }
  });
});
