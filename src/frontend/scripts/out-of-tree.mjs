// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { existsSync, readdirSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import path from 'node:path';

// Widgets, nodes and plugin frontends share this package's lint and fmt config but live outside
// it. vp refuses a `..` path and applies ignorePatterns only inside this package, so the source
// files are collected here and handed over as absolute paths, in batches that fit a Windows
// command line. Locale JSON, READMEs and build output never reach the tools this way.
const frontendDir = path.resolve(import.meta.dirname, '..');
const repoRoot = path.resolve(frontendDir, '..', '..');
const pluginsDir = path.join(repoRoot, 'src', 'plugins', 'Plugins');
const SOURCE = /\.(ts|tsx|css)$/;
const SKIP_DIRS = new Set(['node_modules', 'dist', 'coverage']);
const BATCH = 100;

const [tool, ...args] = process.argv.slice(2);
if (tool !== 'lint' && tool !== 'fmt') {
  console.error('usage: node scripts/out-of-tree.mjs <lint|fmt> [vp args…]');
  process.exit(2);
}

function walk(dir, out) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    if (entry.isDirectory()) {
      if (!SKIP_DIRS.has(entry.name)) walk(path.join(dir, entry.name), out);
    } else if (SOURCE.test(entry.name) && !(tool === 'lint' && entry.name.endsWith('.css'))) {
      out.push(path.join(dir, entry.name));
    }
  }
  return out;
}

const pluginSources = existsSync(pluginsDir)
  ? readdirSync(pluginsDir).map((slug) => path.join(pluginsDir, slug, 'frontend', 'src'))
  : [];
const roots = [path.join(repoRoot, 'widgets'), path.join(repoRoot, 'nodes'), ...pluginSources].filter((dir) =>
  existsSync(dir),
);
const files = roots.flatMap((dir) => walk(dir, []));

const vp = path.join(frontendDir, 'node_modules', 'vite-plus', 'bin', 'vp');
let status = 0;
for (let i = 0; i < files.length; i += BATCH) {
  const result = spawnSync(process.execPath, [vp, tool, ...args, ...files.slice(i, i + BATCH)], {
    cwd: frontendDir,
    stdio: 'inherit',
  });
  if (result.status !== 0) status = result.status ?? 1;
}
process.exit(status);
