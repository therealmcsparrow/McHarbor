// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { readFileSync, writeFileSync, existsSync, statSync, readdirSync, mkdirSync, rmSync, copyFileSync } from 'node:fs';
import { dirname, join, resolve, relative, isAbsolute } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawn } from 'node:child_process';
import { check as checkGo } from './rules/go-patterns.mjs';
import { check as checkTs, keyUnusedFindings } from './rules/ts-patterns.mjs';
import { check as checkLocale } from './rules/locale-json.mjs';
import { defaultGit, parsePorcelainZ } from './session-start.mjs';
import { pruneOlderThan, pruneHookState } from './lib/prune.mjs';

const HERE = dirname(fileURLToPath(import.meta.url));
const REPO = resolve(HERE, '..', '..');

// The Stop hook timeout is 120 s and the impeccable Stop hook runs beside
// this one, so the checks get 50 s and the safety net fires at 55 s.
const BUDGET_MS = 50_000;
const WATCHDOG_MS = 55_000;

const OXLINT_BATCH = 100;

const RUNNER_TIMEOUTS = {
  gofmt: 10_000,
  'golangci-lint': 45_000,
  'go vet': 40_000,
  oxlint: 30_000,
  tsc: 40_000,
  'verify-keys': 15_000,
  'sync-locales': 10_000,
  sanity: 30_000,
  duplication: 20_000,
};

// Every file in this module carries `//go:build ruleguard`, so `go vet`/`golangci-lint`
// see an empty build set there ("build constraints exclude all Go files") without that
// tag; the file is already type-checked whenever gocritic's ruleguard check loads it
// from the backend/agent/cli configs, so running either tool here directly adds nothing.
const BUILD_TAG_GATED_MODULES = new Set(['scripts/lint/ruleguard']);

// duplication is not here: run-all hands -Paths a list of files, and detect.mjs walks
// -Paths as directories, so it fingerprinted nothing on every gate run; and a family's
// drift is only visible against the members this session did not touch. It runs over
// the whole tree below instead (~4 s).
const SANITY_CHECKS =
  'copyright,code-comments,ui-consistency,react-standards,error-visualization,plugin-host-coupling,plugin-isolation,modularity';

const SCAN_ROOTS = ['src', 'widgets', 'nodes', 'scripts'];
const SKIP_DIRS = new Set(['node_modules', 'dist', 'build', 'coverage', 'vendor', 'playwright-report', 'test-results']);
const SCAN_EXT = /\.(?:go|tsx?|jsx?|mjs|cjs|css|sql|ps1|sh|ya?ml)$/;
const CODE_EXT = /\.(?:go|tsx?|jsx?|mjs|cjs|css|sql|ps1|sh)$/;
const LOCALE_JSON = /(?:^|\/)(?:locales|i18n)\/[^/]+\/[^/]+\.json$/;
const EXCLUDED_REL = /(?:^|\/)(?:node_modules|dist|build|coverage|vendor|\.git|\.results|\.references|\.superpowers)\//;
const MAX_REASON_CHARS = 8_000;
const MAX_LISTED_BLOCKING = 25;

const IS_WIN = process.platform === 'win32';

/* ─── paths ──────────────────────────────────────────────────────── */

function relTo(root, file) {
  if (!file) return null;
  const r = relative(root, resolve(root, file));
  if (!r || r.startsWith('..') || isAbsolute(r)) return null;
  return r.split('\\').join('/');
}

function keyOf(rel) {
  return IS_WIN ? rel.toLowerCase() : rel;
}

function hooksDirOf(root) {
  return join(root, '.results', 'hooks');
}

function mtimeOf(file) {
  try {
    return statSync(file).mtimeMs;
  } catch {
    return 0;
  }
}

function readText(file) {
  try {
    return readFileSync(file, 'utf8');
  } catch {
    return null;
  }
}

/* ─── touched set ────────────────────────────────────────────────── */

export function readTouchedEntries(root, sessionId) {
  const text = readText(join(hooksDirOf(root), `${sessionId}.touched`)) ?? '';
  const entries = [];
  for (const raw of text.split('\n')) {
    const line = raw.replace(/\r$/, '');
    if (!line.trim()) continue;
    const [file, agent] = line.split('\t');
    entries.push({ file, agent: agent || null });
  }
  return entries;
}

function scanModified(root, startMs, add) {
  const walk = (dir) => {
    let ents;
    try {
      ents = readdirSync(dir, { withFileTypes: true });
    } catch {
      return;
    }
    for (const e of ents) {
      if (e.isDirectory()) {
        if (SKIP_DIRS.has(e.name) || e.name.startsWith('.')) continue;
        walk(join(dir, e.name));
      } else if (e.isFile()) {
        const full = join(dir, e.name);
        const r = relTo(root, full);
        if (!r || !(SCAN_EXT.test(r) || LOCALE_JSON.test(r))) continue;
        if (mtimeOf(full) >= startMs) add(full);
      }
    }
  };
  for (const top of SCAN_ROOTS) walk(join(root, top));
}

// The Edit/Write list alone misses `sed -i` and Set-Content edits; the mtime
// sweep is what catches them. A subagent's own set is the tagged list only —
// an mtime hit cannot be attributed to one agent.
export function collectTouched({ root, sessionId, agentId = null, startMs = null }) {
  const byKey = new Map();
  const listed = new Set();
  const normalise = (file) => {
    const r = relTo(root, file);
    if (!r || EXCLUDED_REL.test(r) || /\.umd\.cjs$/.test(r)) return null;
    return r;
  };
  const add = (file) => {
    const r = normalise(file);
    if (!r) return null;
    const abs = join(root, ...r.split('/'));
    if (!existsSync(abs)) return null;
    byKey.set(keyOf(r), abs);
    return keyOf(r);
  };
  for (const entry of readTouchedEntries(root, sessionId)) {
    if (agentId && entry.agent !== agentId) continue;
    const k = add(entry.file);
    if (k) listed.add(k);
  }
  const sweepOnly = new Set();
  if (!agentId && startMs) {
    const theirs = otherSessionsTouched(root, sessionId, startMs, normalise);
    scanModified(root, startMs, (file) => {
      const r = normalise(file);
      if (!r || listed.has(keyOf(r)) || theirs.has(keyOf(r))) return;
      const k = add(file);
      if (k) sweepOnly.add(byKey.get(k));
    });
  }
  return { files: [...byKey.values()].sort(), sweepOnly };
}

// A concurrent session in the same checkout edits through its own tools; its
// files are its gate's business, not this one's.
function otherSessionsTouched(root, sessionId, sinceMs, normalise) {
  const keys = new Set();
  let names = [];
  try {
    names = readdirSync(hooksDirOf(root));
  } catch {
    return keys;
  }
  for (const name of names) {
    if (!name.endsWith('.touched') || name === `${sessionId}.touched`) continue;
    const full = join(hooksDirOf(root), name);
    if (mtimeOf(full) < sinceMs) continue;
    for (const entry of readTouchedEntries(root, name.slice(0, -'.touched'.length))) {
      const r = normalise(entry.file);
      if (r) keys.add(keyOf(r));
    }
  }
  return keys;
}

const SHELL_PAD_MS = 2_000;
const UNPAIRED_SHELL_MS = 600_000;

export function readShellRecords(root, sessionId) {
  const text = readText(join(hooksDirOf(root), `${sessionId}.shell.jsonl`)) ?? '';
  const out = [];
  for (const line of text.split('\n')) {
    if (!line.trim()) continue;
    try {
      out.push(JSON.parse(line));
    } catch {
      // A torn append; the other windows still count.
    }
  }
  return out;
}

// A start without an end is a call still running or one that died without a
// PostToolUse; it is held open for the 10-minute Bash ceiling.
export function shellWindows(records, now = Date.now()) {
  const pairs = new Map();
  let anon = 0;
  for (const r of records) {
    const id = r.id ?? `anon-${anon++}`;
    if (!pairs.has(id)) pairs.set(id, {});
    pairs.get(id)[r.phase === 'start' ? 'start' : 'end'] = r;
  }
  const windows = [];
  for (const { start, end } of pairs.values()) {
    const from = start?.at ?? (end ? end.at - (typeof end.duration === 'number' ? end.duration : UNPAIRED_SHELL_MS) : null);
    const to = end?.at ?? Math.min((start?.at ?? now) + UNPAIRED_SHELL_MS, now);
    if (from === null || to === null) continue;
    windows.push([from - SHELL_PAD_MS, to + SHELL_PAD_MS]);
  }
  return windows.sort((a, b) => a[0] - b[0]);
}

/* ─── changed lines ──────────────────────────────────────────────── */

export function readRecords(root, sessionId) {
  const text = readText(join(hooksDirOf(root), `${sessionId}.lines.jsonl`)) ?? '';
  const out = [];
  for (const line of text.split('\n')) {
    if (!line.trim()) continue;
    try {
      out.push(JSON.parse(line));
    } catch {
      // A torn append from a killed hook; the other records still count.
    }
  }
  return out;
}

export function parseUnifiedNewLines(stdout) {
  const map = new Map();
  let current = null;
  for (const line of stdout.split('\n')) {
    if (line.startsWith('+++ ')) {
      let p = line.slice(4).trim().replace(/^"|"$/g, '');
      current = p === '/dev/null' ? null : p.replace(/^b\//, '');
      if (current && !map.has(keyOf(current))) map.set(keyOf(current), new Set());
      continue;
    }
    const m = line.match(/^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@/);
    if (m && current) {
      const start = Number(m[1]);
      const count = m[2] === undefined ? 1 : Number(m[2]);
      for (let i = 0; i < count; i++) map.get(keyOf(current)).add(start + i);
    }
  }
  return map;
}

function chunk(list, size) {
  const out = [];
  for (let i = 0; i < list.length; i += size) out.push(list.slice(i, i + size));
  return out;
}

// An 'unknown' record (a Write with no usable tool_response, an Edit whose
// new_string could not be located) says the file changed but not where, so it
// contributes nothing; a file with only such records stays warn-only.
function linesFromRecords(abs, recs) {
  if (!recs || !recs.length) return null;
  if (recs.some((r) => r.all)) return 'all';
  const known = recs.filter((r) => !r.unknown);
  if (!known.length) return null;
  recs = known;
  const texts = new Set();
  for (const r of recs) {
    for (const t of r.texts ?? []) {
      const trimmed = String(t).trim();
      if (/[A-Za-z0-9]/.test(trimmed)) texts.add(trimmed);
    }
  }
  const current = readText(abs);
  if (current === null) return null;
  const set = new Set();
  current.split('\n').forEach((line, i) => {
    if (texts.has(line.replace(/\r$/, '').trim())) set.add(i + 1);
  });
  return set;
}

// Per file: 'all' (created this session), a Set of changed line numbers, or
// 'warn-only' when the session cannot tell its own lines from the dirty
// tree's (G-5: findings there warn, never block).
export function resolveChangedLines({ root, files, startMs, startStatus, records, git }) {
  const recsByKey = new Map();
  for (const rec of records ?? []) {
    const r = relTo(root, rec.file);
    if (!r) continue;
    const k = keyOf(r);
    if (!recsByKey.has(k)) recsByKey.set(k, []);
    recsByKey.get(k).push(rec);
  }
  const fromRecords = (abs, r) => linesFromRecords(abs, recsByKey.get(keyOf(r))) ?? 'warn-only';

  const result = new Map();
  const statusOk = Boolean(startStatus && startStatus.ok);
  const dirty = new Set((startStatus?.dirty ?? []).map(keyOf));
  const rels = new Map(files.map((f) => [f, relTo(root, f)]));

  let tracked = new Set();
  let ignored = new Set();
  let ignoreOk = false;
  let diffs = new Map();
  let diffOk = false;
  if (statusOk) {
    const clean = [...rels.values()].filter((r) => r && !dirty.has(keyOf(r)));
    diffOk = true;
    for (const part of chunk(clean, 100)) {
      if (!part.length) continue;
      const ls = git(['ls-files', '-z', '--', ...part]);
      if (!ls.ok) {
        diffOk = false;
        break;
      }
      for (const p of ls.stdout.split('\0')) if (p) tracked.add(keyOf(p));
    }
    if (diffOk) {
      for (const part of chunk([...clean].filter((r) => tracked.has(keyOf(r))), 100)) {
        if (!part.length) continue;
        const d = git(['diff', '-U0', '--no-color', '--no-ext-diff', 'HEAD', '--', ...part]);
        if (!d.ok) {
          diffOk = false;
          break;
        }
        for (const [k, set] of parseUnifiedNewLines(d.stdout)) diffs.set(k, set);
      }
      // sed -i recreates a file, so on some filesystems its birthtime resets
      // and a years-old gitignored file would look new; ignored files are
      // therefore never 'all'. check-ignore exits 1 when nothing matches.
      ignoreOk = true;
      for (const part of chunk([...clean].filter((r) => !tracked.has(keyOf(r))), 100)) {
        if (!part.length) continue;
        const ci = git(['check-ignore', '-z', '--', ...part]);
        if (!ci.ok && ci.code !== 1) {
          ignoreOk = false;
          break;
        }
        for (const p of (ci.stdout ?? '').split(/[\0\n]/)) if (p) ignored.add(keyOf(p));
      }
    }
  }

  for (const abs of files) {
    const r = rels.get(abs);
    if (!r) continue;
    if (!statusOk || !diffOk || dirty.has(keyOf(r))) {
      result.set(abs, fromRecords(abs, r));
    } else if (tracked.has(keyOf(r))) {
      result.set(abs, diffs.get(keyOf(r)) ?? new Set());
    } else if (ignoreOk && !ignored.has(keyOf(r))) {
      // -uall lists every untracked, non-ignored file, so one missing from
      // the start snapshot did not exist when the session began.
      result.set(abs, 'all');
    } else {
      result.set(abs, fromRecords(abs, r));
    }
  }
  return result;
}

/* ─── parsers ────────────────────────────────────────────────────── */

// gofmt -d diffs the current file (old side) against the formatted one, so
// every removed line is a current-file line that gofmt would rewrite.
export function parseGofmtDiff(stdout) {
  const findings = [];
  let file = null;
  let oldLine = 0;
  let inHunk = false;
  for (const line of stdout.split('\n')) {
    if (line.startsWith('diff ')) {
      inHunk = false;
      continue;
    }
    if (line.startsWith('--- ')) {
      inHunk = false;
      continue;
    }
    if (line.startsWith('+++ ')) {
      file = line.slice(4).trim();
      inHunk = false;
      continue;
    }
    const m = line.match(/^@@ -(\d+)(?:,\d+)? \+\d+(?:,\d+)? @@/);
    if (m) {
      oldLine = Number(m[1]);
      inHunk = true;
      continue;
    }
    if (!inHunk || !file) continue;
    if (line.startsWith('-')) {
      findings.push({ file, line: oldLine, tool: 'gofmt', rule: 'gofmt', message: 'not gofmt-formatted (run gofmt -w)', severity: 'error' });
      oldLine++;
    } else if (line.startsWith('+')) {
      const last = findings[findings.length - 1];
      if (!last || last.file !== file || last.line !== oldLine - 1) {
        findings.push({ file, line: Math.max(oldLine, 1), tool: 'gofmt', rule: 'gofmt', message: 'not gofmt-formatted (run gofmt -w)', severity: 'error' });
      }
    } else if (line.startsWith(' ')) {
      oldLine++;
    }
  }
  return findings;
}

const GO_POS = /(?:^|\s)((?:[A-Za-z]:)?[^\s:]*?\.go):(\d+)(?::(\d+))?:\s+(.*)$/;

export function parseGoVet(output, moduleDir, tool = 'go vet') {
  const findings = [];
  for (const raw of output.split('\n')) {
    const line = raw.replace(/\r$/, '');
    if (line.startsWith('#')) continue;
    const m = line.match(GO_POS);
    if (!m) continue;
    const file = resolve(moduleDir, m[1]);
    findings.push({ file, line: Number(m[2]), tool, rule: tool, message: m[4].trim(), severity: 'error' });
  }
  return findings;
}

export function parseGolangci(stdout, moduleDir) {
  const start = stdout.indexOf('{');
  if (start < 0) return null;
  let data;
  const end = stdout.indexOf('\n', start);
  try {
    data = JSON.parse(end < 0 ? stdout.slice(start) : stdout.slice(start, end));
  } catch {
    try {
      data = JSON.parse(stdout.slice(start));
    } catch {
      return null;
    }
  }
  return (data.Issues ?? []).map((i) => ({
    file: resolve(moduleDir, i.Pos?.Filename ?? ''),
    line: i.Pos?.Line ?? null,
    tool: 'golangci-lint',
    rule: i.FromLinter ?? 'golangci-lint',
    message: i.Text ?? '',
    severity: i.Severity === 'warning' ? 'warn' : 'error',
  }));
}

export function parseTsc(stdout, frontendDir) {
  const findings = [];
  for (const raw of stdout.split('\n')) {
    const m = raw.replace(/\r$/, '').match(/^(.+?)\((\d+),(\d+)\):\s+error\s+(TS\d+):\s+(.*)$/);
    if (!m) continue;
    findings.push({
      file: resolve(frontendDir, m[1]),
      line: Number(m[2]),
      tool: 'tsc',
      rule: m[4],
      message: `${m[4]}: ${m[5]}`,
      severity: 'error',
    });
  }
  return findings;
}

export function parseOxlint(stdout, frontendDir) {
  const data = JSON.parse(stdout);
  return (data.diagnostics ?? []).map((d) => ({
    file: resolve(frontendDir, d.filename),
    line: d.labels?.[0]?.span?.line ?? null,
    tool: 'oxlint',
    rule: d.code ?? 'oxlint',
    message: d.message,
    severity: d.severity === 'error' ? 'error' : 'warn',
  }));
}

const VERIFY_KEYS_FAILING = new Set(['missing', 'object', 'namespace', 'defaultValue', 'prefix', 'orphan']);

export function parseVerifyKeys(stdout, root) {
  return JSON.parse(stdout).map((r) => ({
    file: join(root, ...String(r.file).split('/')),
    line: r.line ?? null,
    tool: 'verify-keys',
    rule: `i18n/${r.kind}`,
    message: r.message,
    severity: VERIFY_KEYS_FAILING.has(r.kind) ? 'error' : 'warn',
  }));
}

// A sanity report lists hits under "### <id>. <title> - FAIL (n hits)" as
// "- `path:line` - text" or "- `path` - text"; only FAIL sections count.
export function parseSanityReport(md, check, root) {
  const findings = [];
  let section = null;
  for (const raw of md.split('\n')) {
    const line = raw.replace(/\r$/, '');
    const h = line.match(/^###\s+(\S+?)\.\s+(.*?)\s+-\s+([A-Z]+)\s+\(\d+ hits?\)/);
    if (h) {
      section = h[3] === 'FAIL' ? { id: h[1], title: h[2] } : null;
      continue;
    }
    if (line.startsWith('## ')) section = null;
    if (!section) continue;
    const hit = line.match(/^-\s+`([^`]+?)(?::(\d+))?`\s*(?:-\s*(.*))?$/);
    if (!hit) continue;
    findings.push({
      file: join(root, ...hit[1].split('/')),
      line: hit[2] ? Number(hit[2]) : null,
      tool: 'sanity',
      rule: `${check}#${section.id}`,
      message: `${section.title}${hit[3] ? ` — ${hit[3].trim()}` : ''}`,
      severity: 'error',
    });
  }
  return findings;
}

/**
 * duplicationFindings turns detect.mjs --json into findings on the touched files only:
 * a drifted accepted family, an undocumented clone cluster, or a family that gained an
 * undeclared member, each attributed to whichever of its members this session touched.
 */
export function duplicationFindings(data, touched, startMs = null) {
  const byRel = new Map(touched.map((x) => [keyOf(x.rel), x.abs]));
  const findings = [];
  const emit = (rel, rule, message) => {
    const abs = byRel.get(keyOf(rel));
    if (!abs) return;
    findings.push({
      file: abs,
      line: null,
      fileLevel: true,
      // A twin is usually dirty at session start, so its lines are warn-only; drift on a
      // family member this session wrote is still this session's to fix.
      force: startMs !== null && mtimeOf(abs) >= startMs,
      tool: 'duplication',
      rule,
      message,
      severity: 'error',
    });
  };
  for (const w of data.drifted ?? []) {
    const members = [w.reference, ...(w.members ?? []).map((m) => m.path)];
    const moved = (w.drifted ?? []).map((m) => `${m.path} ${m.baseline}→${m.similarity}`).join('; ');
    for (const rel of members) {
      emit(rel, 'duplication/drift', `accepted family "${w.family}" drifted against ${w.reference} (${moved}, tolerance 0.03) — apply the change to every sibling, or record the new baseline if the divergence is intended (plugin-authoring.md rules 5-6)`);
    }
  }
  for (const c of data.failing ?? []) {
    const paths = (c.files ?? []).map((f) => f.path);
    for (const rel of paths) emit(rel, 'duplication/undocumented-clone', `undocumented clone family: ${paths.join(', ')} — factor it out or accept it in scripts/sanity/duplication/accepted.json`);
  }
  for (const c of data.extended ?? []) {
    for (const rel of (c.files ?? []).map((f) => f.path)) {
      emit(rel, 'duplication/undeclared-member', `accepted family "${c.family}" gained undeclared member(s): ${(c.unmatched ?? []).join(', ')} — list them in accepted.json and record the baseline`);
    }
  }
  return findings;
}

/* ─── runner ─────────────────────────────────────────────────────── */

// Fire-and-forget: a synchronous taskkill would stall every other runner's
// timers for up to its own timeout. go vet and powershell spawn children, so
// the whole tree (Windows) or process group (POSIX, spawned detached) goes.
function killTree(child) {
  try {
    if (IS_WIN) {
      const killer = spawn('taskkill', ['/pid', String(child.pid), '/T', '/F'], { windowsHide: true, stdio: 'ignore' });
      killer.on('error', () => {});
      killer.unref();
    } else {
      process.kill(-child.pid, 'SIGKILL');
    }
  } catch {
    try {
      child.kill('SIGKILL');
    } catch {
      // Already gone.
    }
  }
}

const OUTPUT_CAP = 16 * 1024 * 1024;

export function defaultRunner(spec) {
  return new Promise((done) => {
    let child;
    try {
      child = spawn(spec.cmd, spec.args, {
        cwd: spec.cwd,
        env: spec.env ?? process.env,
        windowsHide: true,
        detached: !IS_WIN,
        stdio: ['ignore', 'pipe', 'pipe'],
      });
    } catch (e) {
      done({ code: null, stdout: '', stderr: String(e?.message ?? e), missing: e?.code === 'ENOENT', timedOut: false });
      return;
    }
    let stdout = '';
    let stderr = '';
    let timedOut = false;
    let settled = false;
    const finish = (extra) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      done({ code: null, stdout, stderr, missing: false, timedOut, ...extra });
    };
    child.stdout.on('data', (d) => {
      if (stdout.length < OUTPUT_CAP) stdout += d;
    });
    child.stderr.on('data', (d) => {
      if (stderr.length < OUTPUT_CAP) stderr += d;
    });
    const timer = setTimeout(() => {
      timedOut = true;
      killTree(child);
      finish({});
    }, Math.max(1_000, spec.timeoutMs));
    child.on('error', (e) => finish({ missing: e.code === 'ENOENT', stderr: stderr + String(e.message) }));
    child.on('close', (code) => finish({ code }));
  });
}

/* ─── checks ─────────────────────────────────────────────────────── */

function findModule(root, file) {
  let dir = dirname(file);
  for (;;) {
    if (existsSync(join(dir, 'go.mod'))) return dir;
    const r = relTo(root, dir);
    if (r === null || dir === root) return null;
    const parent = dirname(dir);
    if (parent === dir) return null;
    dir = parent;
  }
}

function pkgArg(moduleDir, dir) {
  const r = relative(moduleDir, dir).split('\\').join('/');
  return r ? `./${r}` : '.';
}

function outcome(label, res) {
  if (res.missing) return `${label}: skipped (tool missing)`;
  if (res.timedOut) return `${label}: skipped (timed out)`;
  return null;
}

export async function runChecks({ root, files, runner = defaultRunner, deadline, sessionId, changed = null, scope = 'main', startMs = null, timeoutScale = 1, beforeText = null }) {
  const findings = [];
  const skipped = [];
  const notes = [];
  const internal = [];
  const timings = {};
  const rels = files.map((f) => ({ abs: f, rel: relTo(root, f) })).filter((x) => x.rel);
  const changedOf = (abs) => (changed ? changed.get(abs) : 'all');
  const goEnv = { ...process.env, GOTOOLCHAIN: 'auto', GOWORK: 'off' };

  const call = async (spec) => {
    const timeoutMs = Math.min((RUNNER_TIMEOUTS[spec.label] ?? 30_000) * timeoutScale, Math.max(1_000, deadline - Date.now()));
    const started = Date.now();
    const res = await runner({ ...spec, timeoutMs });
    timings[spec.label] = (timings[spec.label] ?? 0) + (Date.now() - started);
    return res;
  };
  const skip = (text) => {
    if (!skipped.includes(text)) skipped.push(text);
  };
  const guard = (label, fn) =>
    fn().catch((e) => {
      internal.push(`${label}: ${e?.message ?? e}`);
    });

  const tasks = [];

  const inProcess = [];
  for (const { abs, rel } of rels) {
    const cl = changedOf(abs);
    if (cl === 'warn-only' || cl === 'foreign' || cl === undefined) continue;
    const isGo = /\.go$/.test(rel);
    const isTs = /\.tsx?$/.test(rel);
    const isLocale = /\.json$/.test(rel) && LOCALE_JSON.test(rel);
    if (!isGo && !isTs && !isLocale) continue;
    const text = readText(abs);
    if (text === null) continue;
    const checkFn = isGo ? checkGo : isTs ? checkTs : checkLocale;
    for (const f of checkFn({ path: abs, text, changedLines: cl, root })) {
      inProcess.push({ file: abs, line: f.line, tool: 'rules', rule: f.rule, message: f.message, severity: f.severity === 'error' ? 'error' : 'warn' });
    }
    // A key whose last call site this change removed: only the caller knows what the
    // file looked like before (a git blob or the manual-run snapshot).
    if (isTs && beforeText) {
      const before = beforeText(abs);
      if (typeof before === 'string') {
        for (const f of keyUnusedFindings(root, before, text)) {
          inProcess.push({ file: abs, line: f.line, tool: 'rules', rule: f.rule, message: f.message, severity: 'warn' });
        }
      }
    }
  }
  findings.push(...inProcess);

  const detector = join(root, 'scripts', 'sanity', 'duplication', 'detect.mjs');
  const familyCandidates = rels.filter((x) => /^(?:src|widgets|nodes)\/.*\.(?:go|tsx?)$/.test(x.rel));
  if (familyCandidates.length && existsSync(detector)) {
    tasks.push(
      guard('duplication', async () => {
        const res = await call({ label: 'duplication', cmd: process.execPath, args: [detector, '--root', root, '--json'], cwd: root });
        const why = outcome('duplication', res);
        if (why) return skip(why);
        let data;
        try {
          data = JSON.parse(res.stdout || '{}');
        } catch {
          notes.push(`duplication: detect.mjs exited ${res.code} without JSON`);
          return;
        }
        findings.push(...duplicationFindings(data, familyCandidates, startMs));
      }),
    );
  }

  const goFiles = rels.filter((x) => /\.go$/.test(x.rel));
  if (goFiles.length) {
    tasks.push(
      guard('gofmt', async () => {
        const res = await call({ label: 'gofmt', cmd: 'gofmt', args: ['-d', ...goFiles.map((x) => x.abs)], cwd: root });
        const why = outcome('gofmt', res);
        if (why) return skip(why);
        const diffFindings = parseGofmtDiff(res.stdout ?? '').map((f) => ({ ...f, file: resolve(root, f.file) }));
        findings.push(...diffFindings, ...parseGoVet(res.stderr ?? '', root, 'gofmt'));
      }),
    );

    const modules = new Map();
    for (const x of goFiles) {
      const mod = findModule(root, x.abs);
      if (!mod) {
        notes.push(`${x.rel}: no go.mod above it, so go vet / golangci-lint did not run on it`);
        continue;
      }
      if (!modules.has(mod)) modules.set(mod, new Set());
      modules.get(mod).add(pkgArg(mod, dirname(x.abs)));
    }
    for (const [mod, pkgSet] of modules) {
      const pkgs = [...pkgSet].sort();
      const modRel = relTo(root, mod) ?? mod;
      if (BUILD_TAG_GATED_MODULES.has(modRel)) {
        notes.push(`${modRel}: every file carries a //go:build tag, so go vet / golangci-lint would see an empty build set here; skipped`);
        continue;
      }
      tasks.push(
        guard(`golangci-lint ${modRel}`, async () => {
          const res = await call({
            label: 'golangci-lint',
            cmd: 'golangci-lint',
            args: ['run', '--allow-parallel-runners', '--output.json.path', 'stdout', ...pkgs],
            cwd: mod,
            env: goEnv,
          });
          const why = outcome('golangci-lint', res);
          if (why) return skip(why);
          const parsed = parseGolangci(res.stdout ?? '', mod);
          if (parsed === null) {
            if (res.code !== 0) notes.push(`golangci-lint (${modRel}) exited ${res.code} without JSON: ${(res.stderr ?? '').split('\n')[0]}`);
            return;
          }
          findings.push(...parsed);
        }),
      );
      tasks.push(
        guard(`go vet ${modRel}`, async () => {
          const res = await call({ label: 'go vet', cmd: 'go', args: ['vet', ...pkgs], cwd: mod, env: goEnv });
          const why = outcome('go vet', res);
          if (why) return skip(why);
          const parsed = parseGoVet(`${res.stdout ?? ''}\n${res.stderr ?? ''}`, mod);
          if (res.code !== 0 && !parsed.length) {
            notes.push(`go vet (${modRel}) exited ${res.code}: ${(res.stderr ?? '').trim().split('\n').slice(0, 3).join(' | ')}`);
          }
          findings.push(...parsed);
        }),
      );
    }
  }

  const frontendDir = join(root, 'src', 'frontend');
  const feTs = rels.filter((x) => /^src\/frontend\/.*\.tsx?$/.test(x.rel));
  // Widgets, nodes and plugin frontends share src/frontend's lint config. vp refuses a `..`
  // path, so every file goes over as an absolute path from src/frontend in one Oxlint call.
  const outOfTreeTs = rels.filter(
    (x) =>
      /^widgets\/.*\.tsx?$/.test(x.rel) ||
      /^nodes\/.*\.tsx?$/.test(x.rel) ||
      /^src\/plugins\/Plugins\/[^/]+\/frontend\/src\/.*\.tsx?$/.test(x.rel),
  );
  const lintTs = [...feTs, ...outOfTreeTs];
  if (lintTs.length) {
    tasks.push(
      guard('oxlint', async () => {
        const bin = join(frontendDir, 'node_modules', 'vite-plus', 'bin', 'vp');
        if (!existsSync(bin)) return skip('oxlint: skipped (tool missing)');
        // Batched: a session that touches hundreds of files (a reformat) otherwise overflows the
        // Windows command line with ENAMETOOLONG and lints nothing.
        for (let i = 0; i < lintTs.length; i += OXLINT_BATCH) {
          const res = await call({
            label: 'oxlint',
            cmd: process.execPath,
            args: [bin, 'lint', '--format', 'json', '--no-error-on-unmatched-pattern', ...lintTs.slice(i, i + OXLINT_BATCH).map((x) => x.abs)],
            cwd: frontendDir,
          });
          const why = outcome('oxlint', res);
          if (why) return skip(why);
          try {
            findings.push(...parseOxlint(res.stdout ?? '', frontendDir));
          } catch {
            notes.push(`oxlint exited ${res.code} without JSON: ${(res.stderr ?? '').trim().split('\n')[0]}`);
          }
        }
      }),
    );
  }
  if (feTs.length) {
    tasks.push(
      guard('tsc', async () => {
        const bin = join(frontendDir, 'node_modules', 'typescript', 'bin', 'tsc');
        if (!existsSync(bin)) return skip('tsc: skipped (tool missing)');
        const res = await call({ label: 'tsc', cmd: process.execPath, args: [bin, '-b', '--pretty', 'false'], cwd: frontendDir });
        const why = outcome('tsc', res);
        if (why) return skip(why);
        const parsed = parseTsc(res.stdout ?? '', frontendDir);
        if (res.code !== 0 && !parsed.length) notes.push(`tsc -b exited ${res.code}: ${(res.stdout || res.stderr || '').trim().split('\n')[0]}`);
        findings.push(...parsed);
      }),
    );
  }

  const tsAll = rels.filter((x) => /\.tsx?$/.test(x.rel) && !/\.(?:test|stories)\.tsx?$|\.d\.ts$/.test(x.rel));
  const verifier = join(root, 'scripts', 'sanity', 'i18n', 'verify-keys.mjs');
  if (tsAll.length && existsSync(verifier)) {
    tasks.push(
      guard('verify-keys', async () => {
        const res = await call({ label: 'verify-keys', cmd: process.execPath, args: [verifier, '--json', ...tsAll.map((x) => x.abs)], cwd: root });
        const why = outcome('verify-keys', res);
        if (why) return skip(why);
        try {
          findings.push(...parseVerifyKeys(res.stdout || '[]', root));
        } catch {
          notes.push(`verify-keys exited ${res.code} without JSON`);
        }
      }),
    );
  }

  const locales = rels.filter((x) => /^src\/frontend\/core\/i18n\/locales\/.+\.json$/.test(x.rel));
  const syncer = join(root, 'scripts', 'sanity', 'i18n', 'sync-locales.mjs');
  if (locales.length && existsSync(syncer)) {
    tasks.push(
      guard('sync-locales', async () => {
        const res = await call({ label: 'sync-locales', cmd: process.execPath, args: [syncer], cwd: root });
        const why = outcome('sync-locales', res);
        if (why) return skip(why);
        if (res.code === 1) {
          const drift = (res.stdout ?? '').trim().split('\n').filter(Boolean);
          findings.push({
            file: locales[0].abs,
            line: null,
            fileLevel: true,
            tool: 'sync-locales',
            rule: 'i18n/locale-parity',
            message: `locale bundles drifted from en (${drift.slice(0, 6).join('; ')}${drift.length > 6 ? '; …' : ''}) — node scripts/sanity/i18n/sync-locales.mjs --write`,
            severity: 'error',
          });
        }
      }),
    );
  }

  const sanityRels = rels.map((x) => x.rel).filter((r) => /^(?:src|widgets|nodes|scripts|docker)\//.test(r));
  const runAll = join(root, 'scripts', 'sanity', 'run-all.ps1');
  if (sanityRels.length && existsSync(runAll)) {
    tasks.push(
      guard('sanity', async () => {
        const outRel = `.results/hooks/sanity-${sessionId}-${scope.replace(/[^A-Za-z0-9_-]/g, '_')}`;
        const outDir = join(root, ...outRel.split('/'));
        mkdirSync(outDir, { recursive: true });
        for (const name of readdirSync(outDir)) if (name.startsWith('gate-')) rmSync(join(outDir, name), { force: true });
        let joined = sanityRels.join(',');
        if (joined.length > 20_000) {
          joined = joined.slice(0, joined.lastIndexOf(',', 20_000));
          notes.push('sanity: the touched path list was truncated to fit the command line');
        }
        const res = await call({
          label: 'sanity',
          cmd: IS_WIN ? 'powershell.exe' : 'pwsh',
          args: ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', runAll, '-Check', SANITY_CHECKS, '-Paths', joined, '-FailOn', 'fail', '-NoSummaryFile', '-OutDir', outRel, '-Stamp', 'gate'],
          cwd: root,
        });
        const why = outcome(IS_WIN ? 'sanity (powershell.exe)' : 'sanity (pwsh)', res);
        if (why) return skip(why);
        if (res.code === 2) notes.push(`sanity: a check crashed — see ${outRel}/`);
        for (const check of SANITY_CHECKS.split(',')) {
          const md = readText(join(outDir, `gate-${check}.md`));
          if (md) findings.push(...parseSanityReport(md, check, root));
        }
      }),
    );
  }

  const pluginFe = new Map();
  const pluginBe = new Set();
  for (const { abs, rel } of rels) {
    const fe = rel.match(/^src\/plugins\/Plugins\/([^/]+)\/frontend\/src\//);
    if (fe) {
      const prev = pluginFe.get(fe[1]);
      const m = mtimeOf(abs);
      if (!prev || m > prev.mtime) pluginFe.set(fe[1], { abs, mtime: m });
    }
    const be = rel.match(/^src\/plugins\/Plugins\/([^/]+)\/backend\//);
    if (be) pluginBe.add(be[1]);
  }
  for (const [slug, newest] of pluginFe) {
    // Plugin vite configs write the bundle beside frontend/ (outDir '../frontend'),
    // not into dist/, so both places count; only the plugin's own <slug>.umd.cjs does.
    const feDir = join(root, 'src', 'plugins', 'Plugins', slug, 'frontend');
    const distDir = join(feDir, 'dist');
    const bundlePaths = [];
    for (const [dir, accept] of [
      [distDir, (n) => n.endsWith('.umd.cjs')],
      [feDir, (n) => n === `${slug}.umd.cjs`],
    ]) {
      try {
        for (const n of readdirSync(dir)) if (accept(n)) bundlePaths.push(join(dir, n));
      } catch {
        // A plugin without that directory simply has no bundle there.
      }
    }
    const built = Math.max(0, ...bundlePaths.map((p) => mtimeOf(p)));
    if (built < newest.mtime) {
      findings.push({
        file: newest.abs,
        line: null,
        fileLevel: true,
        // Plugin sources are untracked, so their lines are usually warn-only;
        // an unbuilt bundle is still a real miss whenever this session wrote
        // the source, whatever its line attribution says.
        force: startMs !== null && newest.mtime >= startMs,
        tool: 'plugin-build',
        rule: 'plugin/frontend-not-rebuilt',
        message: `plugin/${slug}: frontend not rebuilt (npm run build in that plugin)`,
        severity: 'error',
      });
    }
  }
  for (const slug of [...pluginBe].sort()) {
    notes.push(
      `plugin/${slug}: backend changed — a version bump must move the DB row, the <slug>@<version> install dir, plugin.yaml and the .so Version symbol together, through the Store's Update (.claude/rules/versioning.md)`,
    );
  }

  await Promise.all(tasks);
  return { findings, skipped, notes, internal, timings };
}

/* ─── classify / reply lines / decide ────────────────────────────── */

export function classify(findings, changed, fileSet) {
  const blocking = [];
  const warnings = [];
  const inSet = new Map([...fileSet].map((f) => [keyOf(resolve(f)), resolve(f)]));
  for (const f of findings) {
    const canonical = inSet.get(keyOf(resolve(f.file)));
    if (f.severity !== 'error' || !canonical) {
      warnings.push(f);
      continue;
    }
    const cl = changed.get(canonical) ?? changed.get(f.file);
    const onChanged =
      (f.force && cl !== 'foreign') ||
      cl === 'all' ||
      (f.fileLevel && cl !== 'warn-only' && cl !== 'foreign' && cl !== undefined) ||
      (cl instanceof Set && f.line != null && cl.has(f.line));
    (onChanged ? blocking : warnings).push({ ...f, file: canonical });
  }
  return { blocking, warnings };
}

const TEST_FLOOR = /test floor[*_\s]*[:：—–-]/i;
const SURFACES = /surfaces[*_\s]*[:：—–-]/i;

function isQuestion(message) {
  const lines = message.split('\n').map((l) => l.trim()).filter(Boolean);
  const last = (lines[lines.length - 1] ?? '').replace(/[*_)\]\s`]+$/, '');
  return last.endsWith('?');
}

export function missingReplyLines({ event, message, codeTouched }) {
  if (event !== 'Stop' || !codeTouched || typeof message !== 'string' || !message.trim()) return [];
  if (isQuestion(message)) return [];
  const missing = [];
  if (!TEST_FLOOR.test(message)) missing.push({ label: 'Test floor:', rule: '.claude/rules/test-floor.md' });
  if (!SURFACES.test(message)) missing.push({ label: 'Surfaces:', rule: '.claude/rules/cli-and-reporting.md' });
  return missing;
}

export function decide({ blocking, missing, notes = [], stopHookActive, canBlock, report, internal = [] }) {
  if (!blocking.length && !missing.length && !internal.length) return notes.length ? { systemMessage: report } : null;
  if (stopHookActive || !canBlock || internal.length) return { systemMessage: report };
  return { decision: 'block', reason: report };
}

/* ─── transcript ─────────────────────────────────────────────────── */

export function lastAssistantText(transcriptPath) {
  const text = transcriptPath ? readText(transcriptPath) : null;
  if (!text) return null;
  const lines = text.split('\n');
  for (let i = lines.length - 1; i >= 0; i--) {
    const line = lines[i].trim();
    if (!line) continue;
    let entry;
    try {
      entry = JSON.parse(line);
    } catch {
      continue;
    }
    if (entry.type !== 'assistant' || entry.isSidechain) continue;
    const content = entry.message?.content;
    const body = typeof content === 'string' ? content : Array.isArray(content) ? content.filter((c) => c?.type === 'text').map((c) => c.text).join('\n') : '';
    if (body.trim()) return body;
  }
  return null;
}

/* ─── report ─────────────────────────────────────────────────────── */

function fmt(root, f) {
  const rel = relTo(root, f.file) ?? f.file;
  return `- ${rel}:${f.line ?? '-'} [${f.tool}/${f.rule}] ${f.message}`;
}

function buildReports({ root, event, agentId, reportRel, files, blocking, warnings, missing, checks, elapsedMs }) {
  const head = `Hand-back gate (${event}${agentId ? ` ${agentId}` : ''}): ${blocking.length} blocking finding(s) on lines changed this session${
    missing.length ? `; the reply is missing ${missing.length} required line(s)` : ''
  }${checks.internal.length ? `; ${checks.internal.length} internal error(s), so nothing was blocked` : ''}.`;
  const short = [head];
  if (blocking.length) {
    short.push('Blocking:');
    for (const f of blocking.slice(0, MAX_LISTED_BLOCKING)) short.push(fmt(root, f));
    if (blocking.length > MAX_LISTED_BLOCKING) short.push(`- …and ${blocking.length - MAX_LISTED_BLOCKING} more (see the full report)`);
  }
  if (missing.length) {
    short.push('Reply is missing:');
    for (const m of missing) short.push(`- a "${m.label}" line (${m.rule})`);
  }
  if (checks.internal.length) short.push(`Internal errors: ${checks.internal.join('; ')}`);
  if (warnings.length) {
    short.push(`Warnings (not blocking: pre-existing, outside the lines you changed, or Bash edits to files already dirty at session start): ${warnings.length}.`);
  }
  if (checks.skipped.length) short.push(`Skipped: ${checks.skipped.join('; ')}.`);
  if (checks.notes.length) {
    short.push('Notes:');
    for (const n of checks.notes) short.push(`- ${n}`);
  }
  short.push(`Full report: ${reportRel}. Fix the blocking items, or say in your reply why one is a false positive.`);

  let reason = short.join('\n');
  if (reason.length > MAX_REASON_CHARS) reason = `${reason.slice(0, MAX_REASON_CHARS)}\n…(truncated — see the full report)`;

  const full = [`# Hand-back gate — ${new Date().toISOString()}`, '', ...short, ''];
  full.push(`## Touched files (${files.length})`, '');
  for (const f of files) full.push(`- ${relTo(root, f) ?? f}`);
  full.push('', `## Warnings (${warnings.length})`, '');
  for (const f of warnings.slice(0, 500)) full.push(fmt(root, f));
  if (warnings.length > 500) full.push(`- …and ${warnings.length - 500} more`);
  full.push('', '## Timings (ms)', '');
  for (const [label, ms] of Object.entries(checks.timings)) full.push(`- ${label}: ${ms}`);
  full.push(`- total: ${elapsedMs}`, '');
  return { reason, full: full.join('\n') };
}

/* ─── gate ───────────────────────────────────────────────────────── */

function readJson(file, fallback) {
  const text = readText(file);
  if (text === null) return fallback;
  try {
    return JSON.parse(text);
  } catch {
    return fallback;
  }
}

export async function gate(payload, { root = REPO, runner = defaultRunner, git = defaultGit(root), now = Date.now } = {}) {
  const t0 = now();
  const event = payload.hook_event_name === 'SubagentStop' ? 'SubagentStop' : 'Stop';
  const isSub = event === 'SubagentStop';
  const agentId = isSub ? payload.agent_id ?? null : null;
  const canBlock = !isSub || Boolean(agentId);
  const stopHookActive = payload.stop_hook_active === true;
  const sessionId = String(payload.session_id ?? 'unknown').replace(/[^A-Za-z0-9_-]/g, '_');
  const dir = hooksDirOf(root);
  try {
    pruneHookState(dir, { now: t0, keepPrefix: `${sessionId}.` });
  } catch {
    // Housekeeping must never stop the gate from judging the edits.
  }
  // A read-only reviewer's SubagentStop must not overwrite the main thread's
  // report, which a pending block reason points at.
  const reportName = isSub ? `${sessionId}.${String(agentId ?? 'unknown').replace(/[^A-Za-z0-9_-]/g, '_')}.gate.md` : `${sessionId}.gate.md`;
  const gateMd = join(dir, reportName);
  const reportRel = `.results/hooks/${reportName}`;
  const stateFile = join(dir, `${sessionId}.gate-state.json`);
  const scope = isSub ? `agent:${agentId ?? 'unknown'}` : 'main';

  const startRaw = readText(join(dir, `${sessionId}.start`));
  const startMs = startRaw && /^\d+$/.test(startRaw.trim()) ? Number(startRaw.trim()) : null;
  const startStatus = readJson(join(dir, `${sessionId}.start-status.json`), null);

  const { files, sweepOnly } = collectTouched({ root, sessionId, agentId, startMs });
  mkdirSync(dir, { recursive: true });
  if (!files.length) {
    writeFileSync(gateMd, `# Hand-back gate — ${new Date(t0).toISOString()}\n\n${event}${agentId ? ` ${agentId}` : ''}: no touched files; nothing ran.\n`);
    return { output: null, report: '' };
  }

  const state = readJson(stateFile, {});
  const sig = files.map((f) => `${f}@${mtimeOf(f)}`).join('|');
  if (state[scope]?.sig === sig) return { output: null, report: '' };
  const lastAt = state[scope]?.at ?? 0;

  // A file only the mtime sweep found is this session's work only if it was
  // written while one of this session's shell calls was running; otherwise it
  // is the user's editor or another process, and it neither blocks nor makes
  // the reply owe a Test floor line.
  const windows = shellWindows(readShellRecords(root, sessionId), t0);
  const foreign = new Set(
    [...(sweepOnly ?? [])].filter((f) => {
      const m = mtimeOf(f);
      return !windows.some(([from, to]) => m >= from && m <= to);
    }),
  );
  const codeTouched = files.some((f) => !foreign.has(f) && CODE_EXT.test(f) && mtimeOf(f) > lastAt);

  let checks = { findings: [], skipped: [], notes: [], internal: [], timings: {} };
  let changed = new Map();
  try {
    changed = resolveChangedLines({ root, files, startMs, startStatus, records: readRecords(root, sessionId), git });
    for (const f of foreign) changed.set(f, 'foreign');
    const dirtyAtStart = new Set((startStatus?.dirty ?? []).map(keyOf));
    const beforeText = beforeTextFrom({ root, git, eligible: (rel) => Boolean(startStatus?.ok) && !dirtyAtStart.has(keyOf(rel)) });
    checks = await runChecks({ root, files, runner, deadline: t0 + BUDGET_MS, sessionId, changed, scope, startMs, beforeText });
  } catch (e) {
    checks.internal.push(String(e?.message ?? e));
  }

  const { blocking, warnings } = classify(checks.findings, changed, new Set(files));
  const message = isSub ? null : payload.last_assistant_message ?? lastAssistantText(payload.transcript_path);
  const missing = missingReplyLines({ event, message, codeTouched });
  const { reason, full } = buildReports({ root, event, agentId, reportRel, files, blocking, warnings, missing, checks, elapsedMs: now() - t0 });

  writeFileSync(gateMd, full);
  state[scope] = { at: t0, sig };
  writeFileSync(stateFile, JSON.stringify(state));

  const output = decide({ blocking, missing, notes: checks.notes, stopHookActive, canBlock, report: reason, internal: checks.internal });
  return { output, report: reason };
}

/* ─── manual (CLI) mode ──────────────────────────────────────────── */

const CLI_TIMEOUT_SCALE = 6;
const CLI_BUDGET_MS = 15 * 60_000;
const CLI_FALLBACK_WINDOW_MS = 24 * 60 * 60_000;
const CLI_MAX_LISTED = 200;
const CLI_BASELINE_MAX_BYTES = 1_000_000;
const CLI_BASELINE_TTL_MS = 30 * 24 * 60 * 60_000;
const CLI_USAGE = 'usage: gate-touched.mjs --cli [--paths <file>[,<file>...] | --since <git-ref|ISO-time> | --git-status]';

export function parseCliArgs(argv) {
  const out = { mode: 'default', paths: [], since: null, error: null };
  const setMode = (mode) => {
    if (out.mode !== 'default' && out.mode !== mode) out.error ??= 'choose one of --paths, --since, --git-status';
    out.mode = mode;
  };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === '--cli') continue;
    if (a === '--git-status') {
      setMode('git-status');
    } else if (a === '--since') {
      setMode('since');
      const v = argv[i + 1];
      if (!v || v.startsWith('--')) out.error ??= '--since needs a git ref or an ISO time';
      else out.since = argv[++i];
    } else if (a === '--paths') {
      setMode('paths');
      while (i + 1 < argv.length && !argv[i + 1].startsWith('--')) {
        out.paths.push(...argv[++i].split(',').map((p) => p.trim()).filter(Boolean));
      }
      if (!out.paths.length) out.error = '--paths needs at least one file';
    } else {
      out.error = `unknown argument ${a}`;
    }
  }
  return out;
}

function cliStatePath(root) {
  return join(hooksDirOf(root), 'manual-gate.json');
}

function parseSince(value, now) {
  if (/^\d{4}-\d{2}-\d{2}/.test(value)) {
    const ms = Date.parse(value);
    if (!Number.isNaN(ms)) return { startMs: ms };
  }
  return { ref: value, now };
}

// Explicit paths and a git ref are exact lists; the time-based modes fall back
// to an mtime sweep because this checkout is never committed from.
export function resolveCliScope({ root, args, git = defaultGit(root), now = Date.now(), cwd = process.cwd() }) {
  const notes = [];
  let candidates = [];
  let baseRef = 'HEAD';
  let label;
  let startMs = null;

  const fromMtime = (ms) => {
    const found = [];
    scanModified(root, ms, (f) => found.push(f));
    return found;
  };

  if (args.mode === 'paths') {
    label = `${args.paths.length} explicit path(s)`;
    for (const p of args.paths) {
      const abs = isAbsolute(p) ? p : existsSync(resolve(cwd, p)) ? resolve(cwd, p) : resolve(root, p);
      if (!existsSync(abs) || !statSync(abs).isFile()) notes.push(`${p}: not a file, skipped`);
      else candidates.push(abs);
    }
  } else if (args.mode === 'git-status') {
    label = 'git status (dirty + untracked)';
    const res = git(['status', '--porcelain', '-z', '-uall']);
    if (!res.ok) notes.push('git status failed; nothing was scoped');
    else candidates = parsePorcelainZ(res.stdout).map((p) => resolve(root, p));
  } else {
    let since = args.since;
    if (args.mode === 'default') {
      const state = readJson(cliStatePath(root), null);
      if (state && Number.isFinite(state.at)) {
        startMs = state.at;
        label = `since the last manual gate run (${new Date(startMs).toISOString()})`;
      } else {
        startMs = now - CLI_FALLBACK_WINDOW_MS;
        label = 'first manual run: files modified in the last 24 h';
      }
      candidates = fromMtime(startMs);
    } else {
      const parsed = parseSince(since, now);
      if (parsed.startMs !== undefined) {
        startMs = parsed.startMs;
        label = `files modified since ${new Date(startMs).toISOString()}`;
        candidates = fromMtime(startMs);
      } else {
        baseRef = since;
        label = `changed since git ref ${since}`;
        const verify = git(['rev-parse', '--verify', '--quiet', `${since}^{commit}`]);
        if (!verify.ok) {
          notes.push(`${since}: not a git ref or an ISO time; nothing was scoped`);
        } else {
          const diff = git(['diff', '--name-only', '-z', '--diff-filter=d', since]);
          const untracked = git(['ls-files', '-z', '--others', '--exclude-standard']);
          for (const part of [diff, untracked]) {
            if (!part.ok) notes.push('a git listing failed; the scope may be incomplete');
            else candidates.push(...part.stdout.split('\0').filter(Boolean).map((p) => resolve(root, p)));
          }
        }
      }
    }
  }

  const byKey = new Map();
  for (const c of candidates) {
    const r = relTo(root, c);
    if (!r || EXCLUDED_REL.test(r) || /\.umd\.cjs$/.test(r)) continue;
    if (!(SCAN_EXT.test(r) || LOCALE_JSON.test(r))) continue;
    if (!existsSync(c)) continue;
    byKey.set(keyOf(r), join(root, ...r.split('/')));
  }
  return { files: [...byKey.values()].sort(), label, notes, baseRef, startMs };
}

function baselinePath(root, rel) {
  return join(hooksDirOf(root), 'manual-base', ...rel.split('/'));
}

// The baseline is the file as it stood when the last manual run finished, so
// the next time-based run judges only what changed since, not the tree's history.
export function saveCliBaselines({ root, files, now = Date.now() }) {
  for (const abs of files) {
    const rel = relTo(root, abs);
    if (!rel) continue;
    try {
      if (statSync(abs).size > CLI_BASELINE_MAX_BYTES) continue;
      const dest = baselinePath(root, rel);
      mkdirSync(dirname(dest), { recursive: true });
      copyFileSync(abs, dest);
    } catch {
      // a file that vanished or is unreadable simply keeps no baseline and falls back to git hunks
    }
  }
  pruneOlderThan(join(hooksDirOf(root), 'manual-base'), { now, ttlMs: CLI_BASELINE_TTL_MS, recurse: true });
}

export function unchangedSinceBaseline({ root, files }) {
  return files.filter((abs) => {
    const rel = relTo(root, abs);
    const base = rel ? readText(baselinePath(root, rel)) : null;
    return base !== null && base === readText(abs);
  });
}

/**
 * beforeTextFrom returns how a file read before this change: the manual-run snapshot
 * when there is one, otherwise the git blob at baseRef, but only for files `eligible`
 * says the blob still describes (the hook passes "clean at session start").
 */
export function beforeTextFrom({ root, git, baseRef = 'HEAD', useBaselines = false, eligible = () => true }) {
  return (abs) => {
    const rel = relTo(root, abs);
    if (!rel) return null;
    if (useBaselines) {
      const snap = readText(baselinePath(root, rel));
      if (snap !== null) return snap;
    }
    if (!eligible(rel)) return null;
    const res = git(['show', `${baseRef}:${rel}`]);
    return res.ok ? res.stdout : null;
  };
}

// A tracked file with hunks against the base is judged on those lines only; a
// clean, untracked or unlisted file has no hunks, so every line of it counts.
export function cliChangedLines({ root, files, git = defaultGit(root), baseRef = 'HEAD', useBaselines = false }) {
  const result = new Map(files.map((f) => [f, 'all']));
  const rest = [];
  for (const abs of files) {
    const rel = relTo(root, abs);
    const base = useBaselines && rel ? baselinePath(root, rel) : null;
    if (base && existsSync(base)) {
      const d = git(['diff', '--no-index', '-U0', '--no-color', '--no-ext-diff', '--', base, abs]);
      const set = [...parseUnifiedNewLines(d.stdout ?? '').values()][0];
      if (set) {
        result.set(abs, set);
        continue;
      }
    }
    rest.push(abs);
  }
  files = rest;
  const rels = files.map((f) => [f, relTo(root, f)]).filter(([, r]) => r);
  const tracked = new Set();
  for (const part of chunk(rels.map(([, r]) => r), 100)) {
    const ls = git(['ls-files', '-z', '--', ...part]);
    if (!ls.ok) return result;
    for (const p of ls.stdout.split('\0')) if (p) tracked.add(keyOf(p));
  }
  const diffs = new Map();
  for (const part of chunk(rels.map(([, r]) => r).filter((r) => tracked.has(keyOf(r))), 100)) {
    const d = git(['diff', '-U0', '--no-color', '--no-ext-diff', baseRef, '--', ...part]);
    if (!d.ok) return result;
    for (const [k, set] of parseUnifiedNewLines(d.stdout)) diffs.set(k, set);
  }
  for (const [abs, r] of rels) {
    const set = diffs.get(keyOf(r));
    if (set && set.size) result.set(abs, set);
  }
  return result;
}

function formatCliReport({ root, label, files, blocking, warnings, checks, notes, elapsedMs }) {
  const lines = [];
  lines.push(`Manual gate: ${blocking.length} blocking finding(s), ${warnings.length} warning(s) over ${files.length} file(s) (${label}).`);
  if (blocking.length) {
    lines.push('', 'Blocking:');
    for (const f of blocking.slice(0, CLI_MAX_LISTED)) lines.push(fmt(root, f));
    if (blocking.length > CLI_MAX_LISTED) lines.push(`- ...and ${blocking.length - CLI_MAX_LISTED} more`);
  }
  if (warnings.length) lines.push('', `Warnings (outside the changed lines, or not error severity): ${warnings.length}; the full list is in .results/hooks/manual-gate.md.`);
  if (checks.internal.length) lines.push('', `Internal errors (the gate could not vouch for the tree): ${checks.internal.join('; ')}`);
  if (checks.skipped.length) lines.push('', `Skipped: ${checks.skipped.join('; ')}.`);
  const allNotes = [...notes, ...checks.notes];
  if (allNotes.length) {
    lines.push('', 'Notes:');
    for (const n of allNotes) lines.push(`- ${n}`);
  }
  lines.push('', 'Timings (ms):');
  for (const [name, ms] of Object.entries(checks.timings)) lines.push(`- ${name}: ${ms}`);
  lines.push(`- total: ${elapsedMs}`);
  return lines.join('\n');
}

// Exit 0 is clean, 1 is a blocking finding, 2 is a usage error or a check that
// crashed. Only a clean run of a time-based scope moves the manual-run marker.
export async function runCli(argv, { root = REPO, git = defaultGit(root), runner = defaultRunner, now = Date.now, out = (s) => process.stdout.write(`${s}\n`), cwd = process.cwd() } = {}) {
  const args = parseCliArgs(argv);
  if (args.error) {
    out(`${args.error}\n${CLI_USAGE}`);
    return 2;
  }
  const t0 = now();
  const scope = resolveCliScope({ root, args, git, now: t0, cwd });
  if (!scope.files.length) {
    out(`Manual gate: no files in scope (${scope.label ?? 'no scope'}); nothing ran.${scope.notes.length ? `\n${scope.notes.map((n) => `- ${n}`).join('\n')}` : ''}`);
    return args.mode === 'paths' || scope.notes.length ? 2 : 0;
  }
  const timeBased = scope.startMs !== null && args.mode !== 'paths';
  const scoped = scope.files;
  if (timeBased) {
    const same = new Set(unchangedSinceBaseline({ root, files: scoped }));
    scope.files = scoped.filter((f) => !same.has(f));
    if (!scope.files.length) {
      out(`Manual gate: 0 blocking finding(s); all ${scoped.length} file(s) in scope are unchanged since the last manual run (${scope.label}).`);
      if (args.mode === 'default') writeFileSync(cliStatePath(root), JSON.stringify({ at: t0 }));
      return 0;
    }
  }
  const changed = cliChangedLines({ root, files: scope.files, git, baseRef: scope.baseRef, useBaselines: timeBased });
  const beforeText = beforeTextFrom({ root, git, baseRef: scope.baseRef, useBaselines: timeBased });
  const checks = await runChecks({ root, files: scope.files, runner, deadline: t0 + CLI_BUDGET_MS, sessionId: 'cli', changed, scope: 'cli', startMs: scope.startMs, timeoutScale: CLI_TIMEOUT_SCALE, beforeText });
  const { blocking, warnings } = classify(checks.findings, changed, new Set(scope.files));
  const elapsedMs = now() - t0;
  const report = formatCliReport({ root, label: scope.label, files: scope.files, blocking, warnings, checks, notes: scope.notes, elapsedMs });
  const ack = blocking.length ? '\n\nThese findings will not be re-reported by the next default run unless the lines change again (the run is snapshotted).' : '';
  out(report + ack);

  const full = [report + ack, '', `## Files (${scope.files.length})`, ''];
  for (const f of scope.files) full.push(`- ${relTo(root, f) ?? f}`);
  full.push('', `## Warnings (${warnings.length})`, '');
  for (const f of warnings.slice(0, 1000)) full.push(fmt(root, f));
  mkdirSync(hooksDirOf(root), { recursive: true });
  writeFileSync(join(hooksDirOf(root), 'manual-gate.md'), `${full.join('\n')}\n`);

  if (checks.internal.length && !blocking.length) return 2;
  saveCliBaselines({ root, files: scoped, now: t0 });
  if (args.mode === 'default') writeFileSync(cliStatePath(root), JSON.stringify({ at: t0 }));
  return blocking.length ? 1 : 0;
}

/* ─── main ───────────────────────────────────────────────────────── */

let emitted = false;

// The watchdog and the normal path can both reach here; a second JSON object
// on stdout would make the whole hook output unparseable.
function emit(obj) {
  if (emitted) return;
  emitted = true;
  const text = obj ? JSON.stringify(obj) : '';
  if (!text) {
    process.exit(0);
    return;
  }
  process.stdout.write(text, () => process.exit(0));
}

async function main() {
  if (process.argv.includes('--cli')) {
    process.exitCode = await runCli(process.argv.slice(2), { root: process.env.MCHARBOR_GATE_ROOT || REPO });
    return;
  }
  const watchdog = setTimeout(() => {
    emit({ systemMessage: 'Hand-back gate: checks overran the 55 s safety limit; nothing was blocked. See .results/hooks/ for partial output.' });
  }, WATCHDOG_MS);
  watchdog.unref();

  let payload;
  try {
    const raw = readFileSync(0, 'utf8');
    payload = raw.trim() ? JSON.parse(raw) : {};
  } catch (e) {
    emit({ systemMessage: `Hand-back gate: could not read the hook payload (${e?.message ?? e}); nothing was checked.` });
    return;
  }
  try {
    const { output } = await gate(payload, { root: process.env.MCHARBOR_GATE_ROOT || REPO });
    emit(output);
  } catch (e) {
    emit({ systemMessage: `Hand-back gate internal error (${e?.message ?? e}); nothing was blocked.` });
  }
}

const isMain = process.argv[1] && resolve(process.argv[1]) === resolve(fileURLToPath(import.meta.url));
if (isMain) main();
