// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import fs from 'node:fs';
import path from 'node:path';

const root = process.argv[2] ? path.resolve(process.argv[2]) : process.cwd();
const SKIP_DIRS = new Set(['node_modules', '.git', 'dist', 'vendor', 'testdata']);

function walk(dir, accept, out = []) {
  let ents;
  try { ents = fs.readdirSync(dir, { withFileTypes: true }); } catch { return out; }
  for (const e of ents) {
    const p = path.join(dir, e.name);
    if (e.isDirectory()) { if (!SKIP_DIRS.has(e.name)) walk(p, accept, out); }
    else if (accept(e.name)) out.push(p);
  }
  return out;
}

// norm reduces a route or URL literal to the check's comparison form: no /api
// prefix, {param} and interpolated segments collapsed to *.
function norm(raw) {
  let p = raw.replace(/\{[^}]*\}/g, '*').replace(/\?.*$/, '');
  if (!p.startsWith('/')) return null;
  p = p.replace(/^\/api(?=\/|$)/, '') || '/';
  p = p.replace(/\/+/g, '/').replace(/\*+/g, '*');
  if (p.length > 1) p = p.replace(/\/$/, '');
  return p;
}

const specFile = path.join(root, 'src/backend/api/openapi.json');
const byOp = new Map();
if (fs.existsSync(specFile)) {
  const spec = JSON.parse(fs.readFileSync(specFile, 'utf8'));
  for (const [p, item] of Object.entries(spec.paths || {})) {
    for (const [verb, op] of Object.entries(item)) {
      if (op && typeof op === 'object' && op.operationId) {
        byOp.set(op.operationId, { verb: verb.toUpperCase(), path: norm(p) });
      }
    }
  }
}

const calls = [];
const seen = new Set();
const add = (c) => {
  const k = `${c.source}|${c.verb}|${c.path}`;
  if (c.path && !seen.has(k)) { seen.add(k); calls.push(c); }
};

// A Go consumer: the CLI and the Terraform provider. Generated client methods are
// named after the operationId with the first letter upper-cased, and a call to
// one is a call to that route; hand-written raw requests carry the path literal.
const consumers = [
  { source: 'cli', dir: 'src/cli' },
  { source: 'terraform', dir: 'src/terraform-provider-mcharbor' },
];
const termRx = /"([^"\\]*)"|[A-Za-z_][\w.]*(?:\((?:[^()]|\([^()]*\))*\))?/y;
for (const { source, dir } of consumers) {
  const base = path.join(root, dir);
  if (!fs.existsSync(base)) continue;
  const files = walk(base, (n) => n.endsWith('.go') && !n.endsWith('_test.go'));
  const methodNames = new Map();
  for (const f of files.filter((x) => x.endsWith('client.gen.go'))) {
    const src = fs.readFileSync(f, 'utf8');
    for (const m of src.matchAll(/^func \(c \*Client\) (\w+?)(?:WithBody)?\(/gm)) {
      const op = m[1][0].toLowerCase() + m[1].slice(1);
      if (byOp.has(op)) methodNames.set(m[1], op);
    }
  }
  for (const f of files) {
    if (/\.gen\.go$/.test(f)) continue;
    const src = fs.readFileSync(f, 'utf8');
    for (const m of src.matchAll(/\b([A-Z]\w*)\(/g)) {
      const op = methodNames.get(m[1]) || methodNames.get(m[1].replace(/WithBody$/, ''));
      if (op) { const r = byOp.get(op); add({ source, verb: r.verb, path: r.path, via: `${op} in ${path.relative(root, f).split(path.sep).join('/')}` }); }
    }
    for (const m of src.matchAll(/"(\/api\/[^"\\]*)"/g)) {
      let built = m[1];
      let i = m.index + m[0].length;
      for (;;) {
        const rest = src.slice(i);
        const plus = /^\s*\+\s*/.exec(rest);
        if (!plus) break;
        termRx.lastIndex = i + plus[0].length;
        const t = termRx.exec(src);
        if (!t) break;
        built += t[1] !== undefined ? t[1] : '*';
        i = termRx.lastIndex;
      }
      add({ source, verb: 'ANY', path: norm(built), via: `${path.relative(root, f).split(path.sep).join('/')}` });
    }
  }
}

console.log(JSON.stringify(calls));
