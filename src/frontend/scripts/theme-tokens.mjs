// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * theme-tokens — generate the status + chart half of the token contract
 * for every file under `public/themes/`.
 *
 * Themes hand-author the surface half (background, card, primary, border…)
 * because that is where a theme's identity lives. The status tones and the
 * chart ramp are *derived* from that surface half instead, for two reasons:
 *
 *   1. Contrast is a property of the pair, not of the colour. "Success is
 *      #10b981" is meaningless until you know what it sits on. Solving for
 *      L against each theme's own `--card` is the only way all 18 themes
 *      clear AA rather than just the two the values were eyeballed against.
 *
 *   2. Semantics must survive a theme switch. Green-is-good is not
 *      Tailwind's opinion, it is the user's expectation, so the *hues* stay
 *      fixed while chroma follows the theme's own saturation character —
 *      Nord gets muted statuses, Synthwave gets vivid ones, and both stay
 *      readable.
 *
 * The chart ramp keeps the validated *relative* hue spacing of the default
 * ramp — which is what colour-vision separation actually depends on — and
 * rotates it so step 1 lands on the theme's own primary hue.
 *
 * Usage:
 *   node scripts/theme-tokens.mjs           # rewrite every theme file
 *   node scripts/theme-tokens.mjs --check   # verify, exit 1 if stale
 */

import console from 'node:console';
import process from 'node:process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const THEMES_DIR = path.join(HERE, '..', 'public', 'themes');
const CHECK = process.argv.includes('--check');

// ── OKLCH ⇄ sRGB ────────────────────────────────────────────────────

function oklchToLinearRgb(L, C, H) {
  const h = (H * Math.PI) / 180;
  const a = C * Math.cos(h);
  const b = C * Math.sin(h);
  const l_ = L + 0.3963377774 * a + 0.2158037573 * b;
  const m_ = L - 0.1055613458 * a - 0.0638541728 * b;
  const s_ = L - 0.0894841775 * a - 1.291485548 * b;
  const l = l_ ** 3;
  const m = m_ ** 3;
  const s = s_ ** 3;
  return [
    4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s,
  ];
}

const inGamut = ([r, g, b]) =>
  r >= -0.0001 && r <= 1.0001 && g >= -0.0001 && g <= 1.0001 && b >= -0.0001 && b <= 1.0001;

/** Reduce chroma until the colour fits sRGB. Hue and lightness are kept. */
function clampChroma(L, C, H) {
  if (inGamut(oklchToLinearRgb(L, C, H))) return C;
  let lo = 0;
  let hi = C;
  for (let i = 0; i < 24; i++) {
    const mid = (lo + hi) / 2;
    if (inGamut(oklchToLinearRgb(L, mid, H))) lo = mid;
    else hi = mid;
  }
  return lo;
}

function relativeLuminance(L, C, H) {
  const lin = oklchToLinearRgb(L, C, H).map((c) => Math.min(1, Math.max(0, c)));
  return 0.2126 * lin[0] + 0.7152 * lin[1] + 0.0722 * lin[2];
}

function contrast(a, b) {
  const hi = Math.max(a, b);
  const lo = Math.min(a, b);
  return (hi + 0.05) / (lo + 0.05);
}

const ratioAt = (L, chroma, hue, surfaceLum) =>
  contrast(relativeLuminance(L, clampChroma(L, chroma, hue), hue), surfaceLum);

/**
 * Pick a lightness for one token.
 *
 * Solving purely for contrast returns the *dimmest* colour that clears the
 * floor, which is technically compliant and looks muddy — a "success" green
 * at the exact AA boundary on a dark card reads as olive. So the anchor is
 * the lightness band the default Tidal ramp was validated at, and the
 * solver only moves away from it when a theme's own surface makes that band
 * fail. Moving further from the surface can only raise contrast, so the
 * result is never worse than the floor.
 */
function pickLightness(hue, chroma, surfaceLum, target, darkMode, anchor) {
  if (ratioAt(anchor, chroma, hue, surfaceLum) >= target) return anchor;

  // Anchor fails against this theme's surface — walk away from the
  // surface until it clears, then stop. Dark surface -> lighter,
  // light surface -> darker.
  const limit = darkMode ? 0.94 : 0.24;
  let lo = anchor;
  let hi = limit;
  for (let i = 0; i < 36; i++) {
    const mid = (lo + hi) / 2;
    if (ratioAt(mid, chroma, hue, surfaceLum) >= target) hi = mid;
    else lo = mid;
  }
  return hi;
}

// ── Colour-vision separation ────────────────────────────────────────
// Adjacent ramp steps are the pairs most likely to sit side by side in a
// legend, so they are the pairs that have to survive CVD. A single fixed
// rotation of the base ramp happens to collide for some themes' primary
// hues, so the rotation is *chosen* per theme rather than assumed.

const CVD_MATRIX = {
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
};

function cvdDistance(a, b, kind) {
  const M = CVD_MATRIX[kind];
  const proj = (c) => {
    const rgb = oklchToLinearRgb(c.L, c.C, c.H).map((v) => Math.min(1, Math.max(0, v)));
    return M.map((row) => row[0] * rgb[0] + row[1] * rgb[1] + row[2] * rgb[2]);
  };
  const A = proj(a);
  const B = proj(b);
  return Math.hypot(A[0] - B[0], A[1] - B[1], A[2] - B[2]) * 100;
}

/** Worst adjacent-pair separation across all three CVD types. */
function rampScore(steps) {
  let worst = Infinity;
  for (let i = 0; i < steps.length - 1; i++) {
    for (const kind of ['protan', 'deutan', 'tritan']) {
      worst = Math.min(worst, cvdDistance(steps[i], steps[i + 1], kind));
    }
  }
  return worst;
}

const fmt = (L, C, H) =>
  `oklch(${L.toFixed(3)} ${clampChroma(L, C, H).toFixed(3)} ${(((H % 360) + 360) % 360).toFixed(1)})`;

// ── The contract ────────────────────────────────────────────────────

// Semantic hues are FIXED across every theme. Green-is-good is the user's
// expectation, not the theme's choice. Only chroma follows the theme.
//
// `dL` and `chroma` reproduce the *relative shape* of the validated Tidal
// tones rather than flattening every tone onto one lightness: warning sits
// brighter than success, danger below it, and critical lower and more
// saturated still, so "critical" reads as louder than "error" on its own
// rather than relying entirely on the ring `status-palette.ts` adds.
const STATUS_TONES = {
  success: { hue: 158, dL: 0, chroma: 1 },
  warning: { hue: 74, dL: +0.048, chroma: 1 },
  danger: { hue: 24, dL: -0.05, chroma: 1.15 },
  critical: { hue: 12, dL: -0.088, chroma: 1.45 },
  info: { hue: 240, dL: -0.014, chroma: 0.92 },
  neutral: { hue: null, dL: -0.05, chroma: 0 }, // theme's own muted hue
};

// Relative hue offsets of the validated default ramp, rotated onto the
// theme's primary hue. Preserves adjacent-pair separation.
const CHART_BASE_HUES = [205.4, 67.9, 290.1, 155.2, 249.8, 20.0, 334.8, 116.3];
const CHART_BASE_CHROMA = [0.113, 0.145, 0.155, 0.144, 0.145, 0.174, 0.157, 0.14];

const AA = 4.6; // a little headroom over the 4.5 floor
const CHART_MIN = 3.2; // graphical-object contrast, not text

// Lightness the default Tidal ramp was validated at, per mode. Themes
// anchor here and only move when their own surface forces it.
const ANCHOR = {
  status: { dark: 0.732, light: 0.545 },
  chart: { dark: 0.632, light: 0.554 },
};

function parseOklch(value) {
  const m = value.match(/oklch\(\s*([0-9.]+%?)\s+([0-9.]+)\s+([0-9.]+)(?:\s*\/\s*([0-9.]+))?\s*\)/);
  if (!m) return null;
  const L = m[1].endsWith('%') ? parseFloat(m[1]) / 100 : parseFloat(m[1]);
  return { L, C: parseFloat(m[2]), H: parseFloat(m[3]) };
}

function readVar(block, name) {
  const m = block.match(new RegExp(`${name}\\s*:\\s*([^;]+);`));
  return m ? m[1].trim() : null;
}

/** Build the contract block for one mode of one theme. */
function buildBlock(block, darkMode, themeName) {
  const card = parseOklch(readVar(block, '--card') || '') || parseOklch(readVar(block, '--background') || '');
  const primary = parseOklch(readVar(block, '--primary') || '');
  const muted = parseOklch(readVar(block, '--muted-foreground') || '');
  const bg = parseOklch(readVar(block, '--background') || '');

  if (!card || !primary || !bg) {
    throw new Error(`${themeName}: cannot derive tokens — missing --card / --primary / --background`);
  }

  const surfaceLum = relativeLuminance(card.L, card.C, card.H);
  const fgOn = darkMode ? bg : { L: 1, C: 0, H: 0 };

  // Chroma character of the theme, normalised against the Tidal default
  // (~0.118). Nord lands ~0.6, Synthwave ~1.5.
  const scale = Math.min(1.6, Math.max(0.55, primary.C / 0.118));

  const lines = [];
  lines.push("  /* Status tones — derived from this theme's own surface.");
  lines.push('     Hues are fixed by meaning; chroma follows the theme; lightness');
  lines.push(`     is solved against --card for >= ${AA}:1. */`);

  const statusAnchor = darkMode ? ANCHOR.status.dark : ANCHOR.status.light;
  for (const [tone, spec] of Object.entries(STATUS_TONES)) {
    const h = spec.hue ?? (muted ? muted.H : primary.H);
    const baseC = spec.chroma === 0 ? 0.022 : 0.145 * scale * spec.chroma;
    // Same sign in both modes: "louder" is lower lightness and higher
    // chroma either way. `pickLightness` then enforces the AA floor, which
    // on a light surface means it may pull a tone darker still.
    const anchor = statusAnchor + spec.dL * (darkMode ? 1 : 0.6);
    const L = pickLightness(h, baseC, surfaceLum, AA, darkMode, anchor);
    lines.push(`  --status-${tone}: ${fmt(L, baseC, h)};`);
    lines.push(`  --status-${tone}-fg: ${fmt(fgOn.L, fgOn.C, fgOn.H)};`);
  }

  lines.push('');
  lines.push('  /* Chart ramp — the validated relative hue spacing, rotated so');
  lines.push("     step 1 sits on this theme's primary hue. */");

  const chartAnchor = darkMode ? ANCHOR.chart.dark : ANCHOR.chart.light;

  const buildRamp = (rotation) =>
    CHART_BASE_HUES.map((baseH, i) => {
      const h = baseH + rotation;
      const c = CHART_BASE_CHROMA[i] * scale;
      const L = pickLightness(h, c, surfaceLum, CHART_MIN, darkMode, chartAnchor);
      return { L, C: clampChroma(L, c, h), H: h };
    });

  // Anchoring step 1 on the theme's primary hue is the goal, but not at the
  // cost of two neighbouring series being the same colour to a third of
  // colourblind users. Search a window around the ideal rotation and take
  // the one with the best worst-pair separation; ties break toward the
  // smallest deviation, so a theme only drifts off its primary when it must.
  const ideal = primary.H - CHART_BASE_HUES[0];
  let best = { rotation: ideal, score: -1, drift: 0 };
  for (let d = -44; d <= 44; d += 2) {
    const ramp = buildRamp(ideal + d);
    const score = rampScore(ramp);
    const drift = Math.abs(d);
    if (score > best.score + 0.25 || (score > best.score - 0.25 && drift < best.drift)) {
      best = { rotation: ideal + d, score, drift };
    }
  }

  const ramp = buildRamp(best.rotation);
  if (best.drift > 0) {
    lines.push(`  /* rotation nudged ${best.drift}° off --primary for CVD separation */`);
  }
  ramp.forEach((c, i) => {
    lines.push(`  --chart-${i + 1}: ${fmt(c.L, c.C, c.H)};`);
  });

  // ── Surface language ──────────────────────────────────────────────
  //
  // Glass, hairlines, shadows and the aurora. These are what make the
  // interface look built rather than assembled, and they have to be
  // per-theme for the same reason the status tones do: a fixed teal
  // aurora over Gruvbox's canvas is Tidal's identity wearing Gruvbox's
  // colours.
  //
  // Glass and hairlines are alpha over the canvas, so their *direction*
  // flips with the mode — white over a dark canvas, ink over a light one.
  const alpha = (l, c, h, a) => `oklch(${l} ${c} ${h} / ${a})`;
  const inkH = muted ? muted.H : primary.H;

  lines.push('');
  lines.push('  /* Surface language — glass, hairlines, shadows, aurora.');
  lines.push('     Alpha direction follows the mode: white over a dark canvas,');
  lines.push("     ink over a light one. Aurora hues orbit this theme's primary. */");

  if (darkMode) {
    lines.push(`  --canvas-2: ${fmt(Math.min(0.999, card.L + 0.037), card.C, card.H)};`);
    lines.push(`  --glass: ${alpha(1, 0, 0, 0.048)};`);
    lines.push(`  --glass-2: ${alpha(1, 0, 0, 0.075)};`);
    lines.push(`  --glass-solid: ${fmt(card.L + 0.009, card.C, card.H)};`);
    lines.push('  --glass-blur: 22px;');
    lines.push(`  --hairline: ${alpha(1, 0, 0, 0.085)};`);
    lines.push(`  --hairline-2: ${alpha(1, 0, 0, 0.16)};`);
    lines.push(`  --edge-hi: ${alpha(1, 0, 0, 0.1)};`);
    lines.push(`  --shine: ${alpha(1, 0, 0, 0.07)};`);
    lines.push(`  --grid-line: ${alpha(1, 0, 0, 0.06)};`);
    lines.push(`  --scrim: ${alpha(0.1, 0.02, inkH.toFixed(0), 0.62)};`);
  } else {
    lines.push(`  --canvas-2: ${fmt(Math.min(1, card.L + 0.017), card.C * 0.4, card.H)};`);
    lines.push(`  --glass: ${alpha(1, 0, 0, 0.7)};`);
    lines.push(`  --glass-2: ${alpha(1, 0, 0, 0.92)};`);
    lines.push(`  --glass-solid: ${alpha(1, 0, 0, 1)};`);
    lines.push('  --glass-blur: 22px;');
    lines.push(`  --hairline: ${alpha(0.52, 0.045, inkH.toFixed(0), 0.17)};`);
    lines.push(`  --hairline-2: ${alpha(0.52, 0.045, inkH.toFixed(0), 0.3)};`);
    lines.push(`  --edge-hi: ${alpha(1, 0, 0, 0.9)};`);
    lines.push(`  --shine: ${alpha(1, 0, 0, 0.55)};`);
    lines.push(`  --grid-line: ${alpha(0.42, 0.05, inkH.toFixed(0), 0.09)};`);
    lines.push(`  --scrim: ${alpha(0.42, 0.05, inkH.toFixed(0), 0.28)};`);
  }

  // --structure is the deep counterpart to the accent: the second stop of
  // the brand mark's gradient, 45 degrees off the primary. Only app.css and
  // the default theme used to define it, so `.mc-brand` fell back to
  // McHarbor blue on every other theme - half a gradient themed, half not.
  const structureH = (((primary.H + 45) % 360) + 360) % 360;
  const structureL = darkMode ? 0.56 : 0.44;
  lines.push(
    `  --structure: oklch(${structureL.toFixed(3)} ${clampChroma(structureL, 0.118 * scale, structureH).toFixed(3)} ${structureH.toFixed(0)});`,
  );

  const pc = clampChroma(primary.L, primary.C, primary.H).toFixed(3);
  lines.push(`  --accent-wash: oklch(${primary.L.toFixed(3)} ${pc} ${primary.H.toFixed(1)} / 0.13);`);
  lines.push(
    `  --accent-glow: oklch(${primary.L.toFixed(3)} ${pc} ${primary.H.toFixed(1)} / ${darkMode ? 0.32 : 0.26});`,
  );

  const sh = darkMode
    ? [
        `0 1px 2px ${alpha(0.1, 0.02, inkH.toFixed(0), 0.4)}`,
        `0 2px 5px ${alpha(0.1, 0.02, inkH.toFixed(0), 0.34)}, 0 14px 34px -16px ${alpha(0.08, 0.02, inkH.toFixed(0), 0.72)}`,
        `0 4px 10px ${alpha(0.1, 0.02, inkH.toFixed(0), 0.38)}, 0 32px 64px -24px ${alpha(0.06, 0.02, inkH.toFixed(0), 0.82)}`,
      ]
    : [
        `0 1px 2px ${alpha(0.45, 0.04, inkH.toFixed(0), 0.1)}`,
        `0 2px 5px ${alpha(0.45, 0.04, inkH.toFixed(0), 0.09)}, 0 14px 34px -16px ${alpha(0.42, 0.05, inkH.toFixed(0), 0.2)}`,
        `0 4px 10px ${alpha(0.45, 0.04, inkH.toFixed(0), 0.1)}, 0 32px 64px -24px ${alpha(0.42, 0.05, inkH.toFixed(0), 0.24)}`,
      ];
  sh.forEach((v, i) => lines.push(`  --shadow-${i + 1}: ${v};`));

  // Three washes: the primary hue, a cool partner 62° off, and a near
  // neighbour 28° back. Wide enough to read as more than one colour,
  // close enough to stay one family.
  const auroraSpecs = darkMode
    ? [
        [primary.H, 0.15, 0.7, 0.17],
        [primary.H + 62, 0.17, 0.52, 0.26],
        [primary.H - 28, 0.13, 0.62, 0.11],
      ]
    : [
        [primary.H, 0.1, 0.86, 0.3],
        [primary.H + 62, 0.09, 0.8, 0.26],
        [primary.H - 28, 0.08, 0.88, 0.22],
      ];
  ['a', 'b', 'c'].forEach((key, i) => {
    const [h, c, l, a] = auroraSpecs[i];
    const hue = ((h % 360) + 360) % 360;
    lines.push(
      `  --aurora-${key}: oklch(${l.toFixed(2)} ${clampChroma(l, c * scale, hue).toFixed(3)} ${hue.toFixed(0)} / ${a});`,
    );
  });

  return lines.join('\n');
}

const START = '  /* ── generated: theme token contract ── */';
const END = '  /* ── end generated ── */';

const escapeRe = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

// Matches every previously generated section, so a rerun replaces rather
// than appends. `/*` contains a quantifier, so this must be escaped.
const GENERATED_RE = new RegExp(`\\n*${escapeRe(START)}[\\s\\S]*?${escapeRe(END)}\\n?`, 'g');

function rewriteBlock(css, selector, darkMode, themeName) {
  const re = new RegExp(`(${selector}\\s*\\{)([\\s\\S]*?)(\\n\\})`);
  const m = css.match(re);
  if (!m) throw new Error(`${themeName}: no ${selector} block`);

  // Strip any previously generated section before deriving, so a rerun
  // reads the hand-authored surface tokens rather than its own output.
  const cleaned = m[2].replace(GENERATED_RE, '\n');
  const generated = buildBlock(cleaned, darkMode, themeName);
  const body = `${cleaned.replace(/\s*$/, '')}\n\n${START}\n${generated}\n${END}\n`;
  return css.replace(re, `${m[1]}${body}}`);
}

const files = fs
  .readdirSync(THEMES_DIR)
  .filter((f) => f.endsWith('.css'))
  .sort();

let stale = 0;
for (const file of files) {
  const full = path.join(THEMES_DIR, file);
  const original = fs.readFileSync(full, 'utf8');
  let out = original;
  out = rewriteBlock(out, ':root', false, file);
  out = rewriteBlock(out, '\\.dark', true, file);
  if (out === original) {
    console.log(`  ok      ${file}`);
    continue;
  }
  if (CHECK) {
    console.error(`  STALE   ${file}`);
    stale++;
  } else {
    fs.writeFileSync(full, out, 'utf8');
    console.log(`  written ${file}`);
  }
}

if (CHECK && stale) {
  console.error(`\n${stale} theme file(s) out of date. Run: node scripts/theme-tokens.mjs`);
  process.exit(1);
}
console.log(`\n${files.length} theme files processed.`);
