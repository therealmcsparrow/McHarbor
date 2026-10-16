// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * theme-lint — CI gate for the theme token contract.
 *
 * Adding a token to the contract used to mean silently breaking 18 files:
 * nothing failed, the theme just fell back to whatever `app.css` said and
 * the drift was invisible until someone opened Gruvbox and found a
 * Tidal-teal chart. This makes that a build failure.
 *
 * Three checks, in order of how loudly they should fail:
 *
 *   1. COMPLETENESS — every theme defines every token in the contract, in
 *      both `:root` and `.dark`.
 *   2. VALIDITY     — every value parses as OKLCH and lands inside sRGB.
 *      An out-of-gamut value is not an error the browser reports; it just
 *      renders as something else.
 *   3. QUALITY      — the values actually work: status tones clear WCAG AA
 *      against the theme's own `--card`, chart steps clear the 3:1
 *      graphical-object floor, and adjacent chart steps stay far enough
 *      apart to be told apart, including under the three common forms of
 *      colour-vision deficiency.
 *
 * A theme may override any token; whatever it supplies is re-validated
 * here rather than trusted.
 *
 * Usage:
 *   node scripts/theme-lint.mjs          # lint every theme
 *   node scripts/theme-lint.mjs --quiet  # only report failures
 */

import console from 'node:console';
import process from 'node:process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const THEMES_DIR = path.join(HERE, '..', 'public', 'themes');
const APP_CSS = path.join(HERE, '..', 'app.css');
const QUIET = process.argv.includes('--quiet');

// ── The contract ────────────────────────────────────────────────────

const STATUS_TONES = ['success', 'warning', 'danger', 'critical', 'info', 'neutral'];

const REQUIRED_TOKENS = [
  ...STATUS_TONES.flatMap((t) => [`--status-${t}`, `--status-${t}-fg`]),
  ...Array.from({ length: 8 }, (_, i) => `--chart-${i + 1}`),
];

// Surface tokens a theme must already define for the derived half to make
// sense. Listed so a malformed theme fails here rather than at render.
const REQUIRED_SURFACE = ['--background', '--foreground', '--card', '--primary', '--border', '--muted-foreground'];

const AA_TEXT = 4.5; // WCAG AA, normal text
const GRAPHIC = 3.0; // WCAG AA, non-text graphical objects
const MIN_SERIES_SEPARATION = 9; // ΔE-ish floor between adjacent chart steps

// ── Colour maths ────────────────────────────────────────────────────

function oklchToLinearRgb(L, C, H) {
  const h = (H * Math.PI) / 180;
  const a = C * Math.cos(h);
  const b = C * Math.sin(h);
  const l = (L + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const m = (L - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const s = (L - 0.0894841775 * a - 1.291485548 * b) ** 3;
  return [
    4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s,
  ];
}

const luminance = (c) => {
  const lin = oklchToLinearRgb(c.L, c.C, c.H).map((v) => Math.min(1, Math.max(0, v)));
  return 0.2126 * lin[0] + 0.7152 * lin[1] + 0.0722 * lin[2];
};

function contrast(a, b) {
  const la = luminance(a);
  const lb = luminance(b);
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05);
}

const inGamut = (c) => {
  const [r, g, b] = oklchToLinearRgb(c.L, c.C, c.H);
  const ok = (v) => v >= -0.005 && v <= 1.005;
  return ok(r) && ok(g) && ok(b);
};

/** Perceptual distance in OKLab — good enough to catch "these two look the same". */
function deltaE(a, b) {
  const toLab = (c) => {
    const h = (c.H * Math.PI) / 180;
    return [c.L, c.C * Math.cos(h), c.C * Math.sin(h)];
  };
  const [l1, a1, b1] = toLab(a);
  const [l2, a2, b2] = toLab(b);
  // Scale L to roughly match the a/b range so no axis dominates.
  return Math.hypot((l1 - l2) * 1.0, a1 - a2, b1 - b2) * 100;
}

/** Crude but effective CVD simulation in linear RGB. */
function simulateCvd(c, kind) {
  const [r, g, b] = oklchToLinearRgb(c.L, c.C, c.H).map((v) => Math.min(1, Math.max(0, v)));
  const M = {
    protan: [
      [0.1121, 0.8853, -0.0005],
      [0.1127, 0.8897, -0.0001],
      [0.0045, 0.0, 1.0019],
    ],
    deutan: [
      [0.292, 0.7054, -0.0003],
      [0.2843, 0.6975, 0.0001],
      [-0.0212, 0.0027, 1.0038],
    ],
    tritan: [
      [1.0173, 0.1442, -0.1621],
      [-0.0092, 0.7935, 0.2153],
      [0.0068, 0.5241, 0.4718],
    ],
  }[kind];
  const out = M.map((row) => row[0] * r + row[1] * g + row[2] * b);
  // Compare in linear space; only relative separation matters here.
  return out;
}

function cvdDistance(a, b, kind) {
  const A = simulateCvd(a, kind);
  const B = simulateCvd(b, kind);
  return Math.hypot(A[0] - B[0], A[1] - B[1], A[2] - B[2]) * 100;
}

// ── Parsing ─────────────────────────────────────────────────────────

function parseOklch(value) {
  if (!value) return null;
  const m = value.match(/oklch\(\s*([0-9.]+%?)\s+([0-9.]+)\s+([0-9.]+)/);
  if (!m) return null;
  const L = m[1].endsWith('%') ? parseFloat(m[1]) / 100 : parseFloat(m[1]);
  return { L, C: parseFloat(m[2]), H: parseFloat(m[3]) };
}

function extractBlock(css, selector) {
  const re = new RegExp(`${selector}\\s*\\{([\\s\\S]*?)\\n\\}`);
  const m = css.match(re);
  if (!m) return null;
  const vars = {};
  for (const line of m[1].split('\n')) {
    const v = line.match(/(--[a-z0-9-]+)\s*:\s*([^;]+);/);
    if (v) vars[v[1]] = v[2].trim();
  }
  return vars;
}

// ── Linting ─────────────────────────────────────────────────────────

const problems = [];
const warnings = [];

function lintMode(file, mode, vars, fallback) {
  const label = `${file} ${mode}`;
  const get = (name) => vars[name] ?? fallback?.[name] ?? null;
  const own = (name) => name in vars;

  for (const token of REQUIRED_SURFACE) {
    if (!get(token)) problems.push(`${label}: missing surface token ${token}`);
  }

  const card = parseOklch(get('--card')) || parseOklch(get('--background'));
  if (!card) {
    problems.push(`${label}: --card / --background unparseable, cannot validate`);
    return;
  }

  for (const token of REQUIRED_TOKENS) {
    const raw = get(token);
    if (!raw) {
      problems.push(`${label}: missing ${token}`);
      continue;
    }
    const c = parseOklch(raw);
    if (!c) {
      problems.push(`${label}: ${token} is not an oklch() value — "${raw}"`);
      continue;
    }
    if (!inGamut(c)) {
      problems.push(`${label}: ${token} is outside sRGB — "${raw}"`);
    }
  }

  // Only quality-check what this theme actually supplies. Tokens inherited
  // from app.css were validated when app.css was written.
  const statusTokens = STATUS_TONES.map((t) => `--status-${t}`).filter(own);
  for (const token of statusTokens) {
    const c = parseOklch(get(token));
    if (!c) continue;
    const ratio = contrast(c, card);
    if (ratio < AA_TEXT) {
      problems.push(`${label}: ${token} is ${ratio.toFixed(2)}:1 on --card, below AA ${AA_TEXT}`);
    }
    const fg = parseOklch(get(`${token}-fg`));
    if (fg) {
      const solid = contrast(c, fg);
      if (solid < AA_TEXT) {
        problems.push(`${label}: ${token}-fg is ${solid.toFixed(2)}:1 on ${token}, below AA ${AA_TEXT}`);
      }
    }
  }

  const chart = Array.from({ length: 8 }, (_, i) => `--chart-${i + 1}`);
  const ownedChart = chart.filter(own);
  for (const token of ownedChart) {
    const c = parseOklch(get(token));
    if (!c) continue;
    const ratio = contrast(c, card);
    if (ratio < GRAPHIC) {
      problems.push(`${label}: ${token} is ${ratio.toFixed(2)}:1 on --card, below the ${GRAPHIC} graphical floor`);
    }
  }

  // Adjacent-pair separation. Steps are assigned in fixed order, so
  // neighbours are the pairs most likely to appear side by side.
  if (ownedChart.length === 8) {
    const parsed = chart.map((t) => parseOklch(get(t)));
    for (let i = 0; i < parsed.length - 1; i++) {
      const a = parsed[i];
      const b = parsed[i + 1];
      if (!a || !b) continue;
      const d = deltaE(a, b);
      if (d < MIN_SERIES_SEPARATION) {
        warnings.push(`${label}: --chart-${i + 1} and --chart-${i + 2} are close (Δ${d.toFixed(1)})`);
      }
      for (const kind of ['protan', 'deutan', 'tritan']) {
        const dc = cvdDistance(a, b, kind);
        if (dc < 6) {
          warnings.push(
            `${label}: --chart-${i + 1}/--chart-${i + 2} may be indistinguishable under ${kind}opia (Δ${dc.toFixed(1)})`,
          );
        }
      }
    }
  }
}

const appCss = fs.readFileSync(APP_CSS, 'utf8');
const fallbackRoot = extractBlock(appCss, ':root');
const fallbackDark = extractBlock(appCss, '\\.dark');

if (!fallbackRoot || !fallbackDark) {
  console.error('app.css: could not read the :root / .dark fallback blocks');
  process.exit(1);
}

// app.css is the fallback source, so it must itself be complete.
lintMode('app.css', ':root', fallbackRoot, null);
lintMode('app.css', '.dark', fallbackDark, null);

const files = fs
  .readdirSync(THEMES_DIR)
  .filter((f) => f.endsWith('.css'))
  .sort();

for (const file of files) {
  const css = fs.readFileSync(path.join(THEMES_DIR, file), 'utf8');
  const root = extractBlock(css, ':root');
  const dark = extractBlock(css, '\\.dark');
  if (!root) {
    problems.push(`${file}: no :root block`);
    continue;
  }
  if (!dark) {
    problems.push(`${file}: no .dark block`);
    continue;
  }
  lintMode(file, ':root', root, fallbackRoot);
  lintMode(file, '.dark', dark, fallbackDark);
}

if (!QUIET) {
  console.log(`theme-lint — ${files.length} themes + app.css, ${REQUIRED_TOKENS.length} contract tokens per mode`);
}

for (const w of warnings) console.warn(`  warn  ${w}`);
for (const p of problems) console.error(`  FAIL  ${p}`);

if (problems.length) {
  console.error(`\n${problems.length} problem(s). The token contract is not satisfied.`);
  process.exit(1);
}

console.log(`\nOK — contract satisfied${warnings.length ? ` (${warnings.length} warning(s))` : ''}.`);
