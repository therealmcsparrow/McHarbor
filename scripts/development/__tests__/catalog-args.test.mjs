// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
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

function statusOf(plugins) {
  const root = mkdtempSync(join(tmpdir(), 'catalog-status-'));
  try {
    const catalog = join(root, 'catalog');
    mkdirSync(catalog);
    for (const p of plugins) {
      const dir = join(root, 'src', 'plugins', 'Plugins', p.slug);
      mkdirSync(dir, { recursive: true });
      writeFileSync(join(dir, 'plugin.yaml'), `name: ${p.slug}\nversion:  "${p.manifest}"\nmcharbor: ">=2026.11.01"\n`);
      const row = (version, build) => ({ version, build, download_url: 'https://example.invalid/x.tar.gz' });
      writeFileSync(
        join(catalog, `${p.slug}.json`),
        JSON.stringify({
          slug: p.slug,
          versions: (p.stable || []).map(([v, b]) => row(v, b)),
          prerelease_versions: (p.pre || []).map(([v, b]) => row(v, b)),
        }),
      );
    }
    writeFileSync(join(root, 'plugins.list'), plugins.map((p) => p.slug).join('\n') + '\n');
    const run = spawnSync('bash', [script, 'status'], {
      encoding: 'utf8',
      env: {
        ...process.env,
        NO_COLOR: '1',
        MCHARBOR_REPO_ROOT: root,
        MCHARBOR_PLUGIN_LIST: join(root, 'plugins.list'),
        MCHARBOR_PLUGIN_OUTPUT_DIR: catalog,
      },
    });
    assert.equal(run.status, 0, run.stderr);
    const states = {};
    for (const line of run.stdout.split('\n')) {
      const m = line.match(/^(\S+)\s+(\d{12}|missing|unreadable|none)\s+(current|STALE|MISSING)\s*$/);
      if (m) states[m[1]] = { build: m[2], state: m[3] };
    }
    return states;
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

const fresh = { slug: 'fresh', manifest: '2026.11.01', stable: [['2026.11.01', '202611070100']] };

describe('catalog.sh status judges the row a rebuild refreshes', () => {
  it('reads a prerelease-only entry as current, not missing', () => {
    const s = statusOf([fresh, { slug: 'pre-only', manifest: '2026.11.02-rc.1', pre: [['2026.11.02-rc.1', '202611070100']] }]);
    assert.deepEqual(s['pre-only'], { build: '202611070100', state: 'current' });
  });

  it('reads a prerelease row left behind by an earlier sweep as stale', () => {
    const s = statusOf([fresh, { slug: 'old-pre', manifest: '2026.11.02-rc.1', pre: [['2026.11.02-rc.1', '202611010100']] }]);
    assert.deepEqual(s['old-pre'], { build: '202611010100', state: 'STALE' });
  });

  it('does not read an older stable row beside a newer prerelease row as stale', () => {
    const s = statusOf([
      fresh,
      {
        slug: 'both',
        manifest: '2026.11.02-rc.1',
        stable: [['2026.11.01', '202611010100']],
        pre: [['2026.11.02-rc.1', '202611070100']],
      },
    ]);
    assert.deepEqual(s.both, { build: '202611070100', state: 'current' });
  });

  it('reads a stable row as stale when the manifest is the stable version', () => {
    const s = statusOf([
      fresh,
      { slug: 'stable-old', manifest: '2026.11.01', stable: [['2026.11.01', '202611010100']] },
    ]);
    assert.deepEqual(s['stable-old'], { build: '202611010100', state: 'STALE' });
  });

  it('reads a manifest version the catalog has no row for as missing', () => {
    const s = statusOf([fresh, { slug: 'unbuilt', manifest: '2026.11.03', stable: [['2026.11.01', '202611070100']] }]);
    assert.equal(s.unbuilt.state, 'MISSING');
  });
});

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
