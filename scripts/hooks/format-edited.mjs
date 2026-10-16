// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { existsSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { readPayload, editedPath } from './lib/payload.mjs';

const HERE = dirname(fileURLToPath(import.meta.url));
const REPO = resolve(HERE, '..', '..');

// Windows delivers tool_input.file_path with backslashes and an uncertain
// drive-letter case (Z:\... vs z:\...); every exclusion below is a
// forward-slash pattern, so both are folded to one comparable form first —
// this is what the bash version's glob case statement silently failed to do.
export function normalizePath(rawPath) {
  return rawPath.replace(/\\/g, '/').replace(/^([A-Za-z]):/, (_, drive) => `${drive.toLowerCase()}:`);
}

// A plugin's Go backend and its committed bundle stay untouched; only its frontend/src shares
// the frontend's fmt config, so that one subtree is let through.
const EXCLUDE_PATTERNS = [
  /\/locales\/.*\.json$/,
  /\/i18n\/.*\.json$/,
  /\.umd\.cjs$/,
  /(^|\/)src\/plugins\/(?!Plugins\/[^/]+\/frontend\/src\/)/,
];

/** isExcluded reports whether a path (backslash or forward-slash) falls
 * under a formatting exclusion — locale/i18n JSON keeps PowerShell house
 * style, umd bundles are committed build output, and the plugin tree is
 * formatted only under frontend/src. */
export function isExcluded(rawPath) {
  const norm = normalizePath(rawPath);
  return EXCLUDE_PATTERNS.some((re) => re.test(norm));
}

const GO_EXT = /\.go$/;
const IN_TREE_EXT = /\.(?:ts|tsx|mts|cts|js|jsx|mjs|cjs|json|css|html)$/;
// Widgets, nodes and plugin frontends borrow src/frontend's fmt config from outside it, where
// its ignorePatterns do not reach, so only their source files are handed over.
const OUT_OF_TREE = /^(?:widgets|nodes|src\/plugins\/Plugins\/[^/]+\/frontend\/src)\/.*\.(?:ts|tsx|css)$/;

// Budgeted well under the 30s PostToolUse timeout, and under each other —
// lint-edited.mjs calls formatFile() before it does anything else, so a
// wedged formatter must not eat the budget the lint rules need. A timeout
// here is a silent skip (spawnSync just returns with .signal set), never a
// crash or stdout noise.
const GOFMT_TIMEOUT_MS = 5_000;
const FMT_TIMEOUT_MS = 10_000;

function formatGo(file) {
  spawnSync('gofmt', ['-w', file], { timeout: GOFMT_TIMEOUT_MS });
}

// oxfmt's config is the `fmt` block in src/frontend/vite.config.ts, so vp runs from there and the
// file goes over as an absolute path (vp refuses a `..` path for the out-of-tree trees).
function formatFrontend(file, repoRoot) {
  const frontendDir = join(repoRoot, 'src', 'frontend');
  const bin = join(frontendDir, 'node_modules', 'vite-plus', 'bin', 'vp');
  if (!existsSync(bin)) return;
  const root = normalizePath(repoRoot).replace(/\/$/, '');
  const norm = normalizePath(resolve(file));
  if (!norm.startsWith(`${root}/`)) return;
  const rel = norm.slice(root.length + 1);
  const inTree = rel.startsWith('src/frontend/') && IN_TREE_EXT.test(rel);
  if (!inTree && !OUT_OF_TREE.test(rel)) return;
  spawnSync(process.execPath, [bin, 'fmt', resolve(file)], { cwd: frontendDir, timeout: FMT_TIMEOUT_MS });
}

/** formatFile runs gofmt/oxfmt on an edited file, or does nothing when
 * the file is excluded or its formatter isn't installed. repoRoot defaults to
 * this repo and is only overridable so a test can inject a fake one. */
export function formatFile(file, repoRoot = REPO) {
  if (isExcluded(file)) return;
  if (GO_EXT.test(file)) {
    formatGo(file);
  } else {
    formatFrontend(file, repoRoot);
  }
}

function main() {
  let payload;
  try {
    payload = readPayload();
  } catch {
    return;
  }
  const file = editedPath(payload);
  if (!file) return;
  try {
    formatFile(file);
  } catch {
    // Formatting must never fail the tool call that already happened.
  }
}

// Guarded so tests can import the exported functions without a stdin read.
const isMain = process.argv[1] && resolve(process.argv[1]) === resolve(fileURLToPath(import.meta.url));
if (isMain) main();
