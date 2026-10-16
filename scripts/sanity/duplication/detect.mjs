// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';

const SKIP_DIRS = new Set([
  'node_modules', '.git', 'dist', 'build', 'vendor', 'coverage', '.vite',
  '.references', '.results', 'worktrees', '.superpowers',
]);
const GENERATED = /\.gen\.go$|\.pb\.go$|_generated\.|\.umd\.cjs$|\.min\.(js|css)$/;
const SHINGLE = 5;

function parseArgs(argv) {
  const opt = {
    root: process.cwd(),
    paths: [],
    exts: ['.go', '.ts', '.tsx'],
    minLines: 60,
    minLocForFail: 200,
    reportThreshold: 0.55,
    failThreshold: 0.70,
    tolerance: 0.03,
    json: false,
    updateBaseline: false,
    accepted: null,
  };
  for (let i = 2; i < argv.length; i++) {
    const a = argv[i];
    const next = () => argv[++i];
    if (a === '--json') opt.json = true;
    else if (a === '--update-baseline') opt.updateBaseline = true;
    else if (a === '--root') opt.root = next();
    else if (a === '--paths') opt.paths = next().split(',').filter(Boolean);
    else if (a === '--exts') opt.exts = next().split(',').filter(Boolean);
    else if (a === '--min-lines') opt.minLines = Number(next());
    else if (a === '--min-loc-fail') opt.minLocForFail = Number(next());
    else if (a === '--report-threshold') opt.reportThreshold = Number(next());
    else if (a === '--fail-threshold') opt.failThreshold = Number(next());
    else if (a === '--tolerance') opt.tolerance = Number(next());
    else if (a === '--accepted') opt.accepted = next();
    else if (a === '--help' || a === '-h') { usage(); process.exit(0); }
    else throw new Error(`unknown argument: ${a}`);
  }
  if (!opt.accepted) opt.accepted = path.join(path.dirname(new URL(import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1')), 'accepted.json');
  return opt;
}

function usage() {
  console.log(`duplication detector

  node scripts/sanity/duplication/detect.mjs [options]

  --root <dir>              repo root (default: cwd)
  --paths a,b               limit the scan to these repo-relative subtrees
  --exts .go,.ts,.tsx       file extensions to read
  --min-lines 60            skip files shorter than this
  --min-loc-fail 200        a cluster only fails if its largest file is at least this long
  --report-threshold 0.55   Jaccard at which a cluster is reported
  --fail-threshold 0.70     Jaccard at which an un-allowlisted cluster fails
  --tolerance 0.03          similarity movement against the baseline that counts as drift
  --accepted <file>         allowlist (default: beside this script)
  --update-baseline         write current drift-watch similarities into the allowlist
  --json                    machine-readable output`);
}

// pathTokens returns the vendor and module names taken from a file's own path.
// Normalising them away is what makes groq_client.go and together_client.go
// comparable; without it every vendor-prefixed clone looks like original code.
function pathTokens(p) {
  const toks = new Set();
  for (const seg of p.split(/[\\/]/)) {
    for (const t of seg.replace(/\.(go|ts|tsx|mjs|js)$/, '').split(/[-_.]/)) {
      if (t.length >= 3) toks.add(t.toLowerCase());
    }
  }
  return [...toks].sort((a, b) => b.length - a.length);
}

function normalize(src, toks) {
  let s = src
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .split('\n')
    .map((l) => l.replace(/\/\/.*$/, ''))
    .map((l) => l.trim().replace(/\s+/g, ' '))
    .filter((l) => l.length > 0)
    .join('\n')
    .toLowerCase();
  for (const t of toks) s = s.split(t).join('@');
  return s.split('\n');
}

// walk collects source files under each subtree. A subtree may name a single file
// as well as a directory; one that resolves to neither is reported in `missing`
// rather than swallowed, because a scope that scans nothing must never read as a pass.
function walk(root, subtrees, exts) {
  const out = [];
  const missing = [];
  const roots = subtrees.length ? subtrees.map((s) => path.resolve(root, s)) : [root];
  const accept = (name) => exts.some((x) => name.endsWith(x)) && !GENERATED.test(name);
  const visit = (dir) => {
    let ents;
    try { ents = fs.readdirSync(dir, { withFileTypes: true }); } catch { return; }
    for (const e of ents) {
      const p = path.join(dir, e.name);
      if (e.isDirectory()) { if (!SKIP_DIRS.has(e.name)) visit(p); }
      else if (accept(e.name)) out.push(p);
    }
  };
  for (const r of roots) {
    let st;
    try { st = fs.statSync(r); } catch { missing.push(path.relative(root, r).split(path.sep).join('/')); continue; }
    if (st.isDirectory()) visit(r);
    else if (accept(path.basename(r))) out.push(r);
  }
  return { files: out, missing };
}

function fingerprint(files, root, minLines) {
  const recs = [];
  for (const f of files) {
    let src;
    try { src = fs.readFileSync(f, 'utf8'); } catch { continue; }
    const loc = src.split('\n').length;
    if (loc < minLines) continue;
    const rel = path.relative(root, f).split(path.sep).join('/');
    const lines = normalize(src, pathTokens(rel));
    if (lines.length < minLines * 0.5) continue;
    const shingles = new Set();
    for (let i = 0; i + SHINGLE <= lines.length; i++) {
      shingles.add(crypto.createHash('md5').update(lines.slice(i, i + SHINGLE).join('\n')).digest('hex').slice(0, 12));
    }
    if (shingles.size < 10) continue;
    recs.push({ path: rel, loc, shingles });
  }
  return recs;
}

function jaccard(a, b) {
  let shared = 0;
  const [small, large] = a.size <= b.size ? [a, b] : [b, a];
  for (const s of small) if (large.has(s)) shared++;
  return shared / (a.size + b.size - shared);
}

function pairsAbove(recs, threshold) {
  const inv = new Map();
  recs.forEach((r, i) => {
    for (const s of r.shingles) {
      if (!inv.has(s)) inv.set(s, []);
      inv.get(s).push(i);
    }
  });
  const shared = new Map();
  for (const idxs of inv.values()) {
    if (idxs.length < 2 || idxs.length > 80) continue;
    for (let a = 0; a < idxs.length; a++) {
      for (let b = a + 1; b < idxs.length; b++) {
        const k = `${idxs[a]}:${idxs[b]}`;
        shared.set(k, (shared.get(k) || 0) + 1);
      }
    }
  }
  const pairs = [];
  for (const [k, n] of shared) {
    const [a, b] = k.split(':').map(Number);
    const j = n / (recs[a].shingles.size + recs[b].shingles.size - n);
    if (j >= threshold) pairs.push({ a, b, j });
  }
  return pairs;
}

function cluster(recs, pairs) {
  const parent = new Map();
  const find = (x) => {
    if (!parent.has(x)) parent.set(x, x);
    while (parent.get(x) !== x) { parent.set(x, parent.get(parent.get(x))); x = parent.get(x); }
    return x;
  };
  const union = (x, y) => { const rx = find(x), ry = find(y); if (rx !== ry) parent.set(rx, ry); };
  for (const p of pairs) union(p.a, p.b);

  const groups = new Map();
  for (const p of pairs) {
    for (const i of [p.a, p.b]) {
      const r = find(i);
      if (!groups.has(r)) groups.set(r, new Set());
      groups.get(r).add(i);
    }
  }
  const out = [];
  for (const members of groups.values()) {
    const idx = [...members];
    if (idx.length < 2) continue;
    const files = idx.map((i) => ({ path: recs[i].path, loc: recs[i].loc })).sort((x, y) => y.loc - x.loc);
    const loc = files.reduce((a, f) => a + f.loc, 0);
    const largest = files[0].loc;
    const inCluster = pairs.filter((p) => members.has(p.a) && members.has(p.b));
    const sims = inCluster.map((p) => p.j);
    out.push({
      files,
      loc,
      redundant: loc - largest,
      maxSimilarity: Math.max(...sims),
      minSimilarity: Math.min(...sims),
      _idx: idx,
    });
  }
  return out.sort((a, b) => b.redundant - a.redundant);
}

// globToRx turns an allowlist pattern into a regex. `*` stops at a path
// separator, `**` crosses them.
function globToRx(glob) {
  let rx = '';
  for (let i = 0; i < glob.length; i++) {
    const c = glob[i];
    if (c === '*') {
      if (glob[i + 1] === '*') { rx += '.*'; i++; if (glob[i + 1] === '/') i++; }
      else rx += '[^/]*';
    } else if ('\\^$+?.()|{}[]'.includes(c)) rx += '\\' + c;
    else rx += c;
  }
  return new RegExp('^' + rx + '$');
}

function loadAccepted(file) {
  if (!fs.existsSync(file)) return { entries: [], raw: null, file };
  const raw = JSON.parse(fs.readFileSync(file, 'utf8'));
  const entries = (raw.accepted || []).map((e) => ({ ...e, _rx: (e.patterns || []).map(globToRx) }));
  return { entries, raw, file };
}

function classify(clusters, accepted) {
  for (const c of clusters) {
    let best = null;
    for (const e of accepted.entries) {
      const matched = c.files.filter((f) => e._rx.some((rx) => rx.test(f.path)));
      if (!matched.length) continue;
      if (!best || matched.length > best.matched) best = { entry: e, matched: matched.length };
    }
    if (!best) { c.status = 'unaccepted'; continue; }
    c.family = best.entry.family;
    c.reason = best.entry.reason;
    c.decision = best.entry.decision;
    c.permanent = !!best.entry.permanent;
    c.status = best.matched === c.files.length ? 'accepted' : 'accepted-extended';
    c.unmatched = c.files.filter((f) => !best.entry._rx.some((rx) => rx.test(f.path))).map((f) => f.path);
  }
  return clusters;
}

function driftWatch(clusters, recs, accepted, tolerance) {
  const watch = [];
  for (const c of clusters) {
    if (c.status !== 'accepted' && c.status !== 'accepted-extended') continue;
    const entry = accepted.entries.find((e) => e.family === c.family);
    // The median member, not the largest. The biggest file in a family is often
    // the one vendor that is not quite the same API (Gemini among the
    // OpenAI-compatible clients), and measuring everyone against the outlier
    // makes a family of near-identical files read as unrelated.
    const byLoc = [...c.files].sort((x, y) => x.loc - y.loc);
    const median = byLoc[Math.floor(byLoc.length / 2)].path;
    const refPath = (entry && entry.reference) || median;
    const ref = recs.find((r) => r.path === refPath) || recs.find((r) => r.path === median);
    if (!ref) continue;
    // One family can own several clusters — logging-* has four. Baselines are
    // keyed by the cluster's reference so they do not overwrite each other.
    const baseline = (entry && entry.baselines && entry.baselines[ref.path]) || {};
    const members = [];
    for (const f of c.files) {
      if (f.path === ref.path) continue;
      const rec = recs.find((r) => r.path === f.path);
      const sim = Number(jaccard(ref.shingles, rec.shingles).toFixed(4));
      const was = baseline[f.path];
      const delta = was === undefined ? null : Number((sim - was).toFixed(4));
      members.push({
        path: f.path, similarity: sim, baseline: was ?? null, delta,
        drifted: delta !== null && Math.abs(delta) > tolerance,
      });
    }
    members.sort((a, b) => a.similarity - b.similarity);
    watch.push({
      family: c.family, decision: c.decision, permanent: c.permanent, reason: c.reason,
      reference: ref.path, members,
      hasBaseline: Object.keys(baseline).length > 0,
      drifted: members.filter((m) => m.drifted),
    });
  }
  return watch;
}

// withFamilyTwins adds every file of any accepted family a scoped file belongs to.
// Drift is a comparison against the family's other members, so a scoped run that
// only held the edited file would have nothing to measure it against.
function withFamilyTwins(files, opt, accepted) {
  const rel = (f) => path.relative(opt.root, f).split(path.sep).join('/');
  const touched = accepted.entries.filter((e) => files.some((f) => e._rx.some((rx) => rx.test(rel(f)))));
  if (!touched.length) return files;
  const have = new Set(files);
  const all = walk(opt.root, [], opt.exts).files;
  for (const f of all) {
    if (have.has(f)) continue;
    const r = rel(f);
    if (touched.some((e) => e._rx.some((rx) => rx.test(r)))) { files.push(f); have.add(f); }
  }
  return files;
}

const TEST_PATH =/(_test\.go|\.(test|spec)\.[cm]?[jt]sx?|\/__tests__\/|\/test\/)/;

// baselineGaps flags an accepted family whose non-test patterns span more than one
// pair while the recorded baselines leave some non-test pair unwatched. Information
// only: a one-sided change to production code escapes drift detection otherwise,
// because the pair that was baselined is usually the tests.
function baselineGaps(watch, accepted) {
  const gaps = [];
  for (const entry of accepted.entries) {
    const nonTestPatterns = (entry.patterns || []).filter((p) => !TEST_PATH.test(p));
    if (nonTestPatterns.length < 2) continue;
    const unbaselined = [];
    let covered = 0;
    for (const w of watch.filter((x) => x.family === entry.family)) {
      if (TEST_PATH.test(w.reference)) continue;
      for (const m of w.members) {
        if (TEST_PATH.test(m.path)) continue;
        if (m.baseline === null) unbaselined.push({ reference: w.reference, member: m.path });
        else covered++;
      }
    }
    if (unbaselined.length) {
      gaps.push({ family: entry.family, nonTestPatterns: nonTestPatterns.length, coveredPairs: covered, unbaselined });
    }
  }
  return gaps;
}

function main() {
  const opt = parseArgs(process.argv);
  const accepted = loadAccepted(opt.accepted);
  const scope = walk(opt.root, opt.paths, opt.exts);
  let files = scope.files;
  if (scope.missing.length || files.length === 0) {
    const why = scope.missing.length ? `path(s) not found: ${scope.missing.join(', ')}` : 'the given paths contain no file to scan';
    console.error(`duplication: scan scope is empty - ${why}`);
    process.exit(2);
  }
  if (opt.paths.length) files = withFamilyTwins(files, opt, accepted);
  const recs = fingerprint(files, opt.root, opt.minLines);
  if (recs.length === 0) console.error(`duplication: ${files.length} file(s) in scope, none long enough to fingerprint (min ${opt.minLines} lines); nothing to compare`);
  const pairs = pairsAbove(recs, opt.reportThreshold);
  const clusters = classify(cluster(recs, pairs), accepted);
  const watch = driftWatch(clusters, recs, accepted, opt.tolerance);

  const failing = clusters.filter(
    (c) => c.status === 'unaccepted' && c.maxSimilarity >= opt.failThreshold && c.files[0].loc >= opt.minLocForFail,
  );
  const reported = clusters.filter((c) => c.status === 'unaccepted' && !failing.includes(c));
  const extended = clusters.filter((c) => c.status === 'accepted-extended');
  const drifted = watch.filter((w) => w.drifted.length);

  if (opt.updateBaseline) {
    if (!accepted.raw) throw new Error(`no allowlist at ${opt.accepted}`);
    for (const w of watch) {
      const entry = accepted.raw.accepted.find((e) => e.family === w.family);
      if (!entry) continue;
      delete entry.baseline;
      if (!entry.baselines) entry.baselines = {};
      entry.baselines[w.reference] = {};
      for (const m of w.members) entry.baselines[w.reference][m.path] = m.similarity;
    }
    const now = new Date();
    accepted.raw.baselineUpdated = [
      now.getFullYear(),
      String(now.getMonth() + 1).padStart(2, '0'),
      String(now.getDate()).padStart(2, '0'),
    ].join('-');
    fs.writeFileSync(opt.accepted, JSON.stringify(accepted.raw, null, 2) + '\n');
    console.error(`baseline written for ${watch.length} family/families to ${opt.accepted}`);
  }

  const result = {
    scanned: recs.length,
    filesSeen: files.length,
    thresholds: {
      report: opt.reportThreshold, fail: opt.failThreshold,
      minLocForFail: opt.minLocForFail, tolerance: opt.tolerance,
    },
    totals: {
      clusters: clusters.length,
      redundantLoc: clusters.reduce((a, c) => a + c.redundant, 0),
      acceptedClusters: clusters.length - clusters.filter((c) => c.status === 'unaccepted').length,
      failingClusters: failing.length,
    },
    failing: failing.map(strip),
    reported: reported.map(strip),
    extended: extended.map(strip),
    accepted: clusters.filter((c) => c.status === 'accepted').map(strip),
    drift: watch,
    drifted,
    baselineGaps: baselineGaps(watch, accepted),
  };

  if (opt.json) { console.log(JSON.stringify(result, null, 2)); }
  else { text(result); }
  process.exitCode = failing.length || extended.length || drifted.length ? 1 : 0;
}

function strip(c) {
  const { _idx, ...rest } = c;
  return rest;
}

function text(r) {
  console.log(`files=${r.filesSeen} fingerprinted=${r.scanned} clusters=${r.totals.clusters} redundant-LOC~${r.totals.redundantLoc}`);
  console.log(`accepted=${r.totals.acceptedClusters} failing=${r.failing.length} reported=${r.reported.length} extended=${r.extended.length} drifted=${r.drifted.length}`);
  const dump = (title, list) => {
    if (!list.length) return;
    console.log(`\n== ${title} ==`);
    for (const c of list) {
      const fam = c.family ? ` [${c.family}]` : '';
      console.log(`\n${c.files.length} files, ${c.loc} LOC, redundant~${c.redundant}, jaccard ${c.minSimilarity.toFixed(2)}-${c.maxSimilarity.toFixed(2)}${fam}`);
      for (const f of c.files) console.log(`  ${String(f.loc).padStart(6)}  ${f.path}`);
      if (c.unmatched && c.unmatched.length) console.log(`  not in the allowlist entry: ${c.unmatched.join(', ')}`);
    }
  };
  dump('FAIL - undocumented clone family', r.failing);
  dump('REVIEW - below the fail threshold', r.reported);
  dump('FAIL - accepted family gained a member', r.extended);
  if (r.drifted.length) {
    console.log('\n== FAIL - drift inside an accepted family ==');
    for (const w of r.drifted) {
      console.log(`\n${w.family} (reference ${w.reference})`);
      for (const m of w.drifted) console.log(`  ${m.path}: ${m.baseline} -> ${m.similarity} (${m.delta > 0 ? '+' : ''}${m.delta})`);
    }
  }
  if (r.baselineGaps.length) {
    console.log('\n== INFO - accepted family with an unbaselined non-test pair ==');
    for (const g of r.baselineGaps) {
      console.log(`\n${g.family}: ${g.nonTestPatterns} non-test patterns, ${g.coveredPairs} non-test pair(s) baselined, ${g.unbaselined.length} not`);
      for (const u of g.unbaselined) console.log(`  ${u.reference} -> ${u.member}`);
    }
  }
  if (r.drift.length) {
    console.log('\n== drift watch ==');
    for (const w of r.drift) {
      const base = w.hasBaseline ? '' : '  (no baseline recorded; run with --update-baseline)';
      console.log(`\n${w.family} - ${w.decision}${w.permanent ? ', permanent' : ''}${base}`);
      console.log(`  reference: ${w.reference}`);
      for (const m of w.members) {
        const d = m.delta === null ? '' : ` (baseline ${m.baseline}, ${m.delta > 0 ? '+' : ''}${m.delta})`;
        console.log(`  ${m.similarity.toFixed(4)}  ${m.path}${d}`);
      }
    }
  }
}

main();
