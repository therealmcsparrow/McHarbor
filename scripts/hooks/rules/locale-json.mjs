// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { readFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { inChanged } from '../lib/payload.mjs';

// The UTF-8-read-as-Windows-1252 byte sequence a double-encoded apostrophe or
// quote leaves behind; it is never legitimate content in a locale value.
const MOJIBAKE = /â€/;

const TRANSLATED_LOCALES = new Set(['nl', 'de', 'es', 'fr', 'pt']);

// A host bundle is keyed by its bare namespace in the i18n-untranslated baseline;
// a plugin bundle by its group-qualified one (src/plugins/Plugins/<slug>/i18n/<ns>).
const HOST_BUNDLE = /^(.*?)[\\/]src[\\/]frontend[\\/]core[\\/]i18n[\\/]locales[\\/]([a-z]{2})[\\/]([^\\/]+)\.json$/;
const PLUGIN_BUNDLE = /^(.*?)[\\/](src[\\/]plugins[\\/]Plugins[\\/][^\\/]+[\\/]i18n)[\\/]([a-z]{2})[\\/]([^\\/]+)\.json$/;

/** check flags a changed locale value that was saved with the wrong encoding, or left in English. */
export function check({ path = '', text, changedLines }) {
  const findings = [];
  const lines = text.split('\n');

  lines.forEach((raw, idx) => {
    const line = idx + 1;
    if (!inChanged(changedLines, line)) return;

    if (MOJIBAKE.test(raw)) {
      findings.push({
        line,
        rule: 'i18n/mojibake',
        message: 'mojibake (â€¦) in a locale value — re-save as UTF-8, not Windows-1252',
        severity: 'error',
      });
    }
  });

  findings.push(...untranslatedFindings({ path, text, changedLines }));
  return findings;
}

/* ─── untranslated values ────────────────────────────────────────── */

function bundleOf(path) {
  const abs = resolve(path);
  const host = abs.match(HOST_BUNDLE);
  if (host) {
    return { root: host[1], locale: host[2], ns: host[3], enPath: join(host[1], 'src', 'frontend', 'core', 'i18n', 'locales', 'en', `${host[3]}.json`) };
  }
  const plugin = abs.match(PLUGIN_BUNDLE);
  if (plugin) {
    const group = plugin[2].split(/[\\/]/).join('/');
    return { root: plugin[1], locale: plugin[3], ns: `${group}/${plugin[4]}`, enPath: join(plugin[1], plugin[2], 'en', `${plugin[4]}.json`) };
  }
  return null;
}

function readText(file) {
  try {
    return readFileSync(file, 'utf8');
  } catch {
    return null;
  }
}

/**
 * leaves walks JSON text and returns every string leaf with its dotted key and the
 * line its value starts on. A parse error returns what was read so far.
 */
export function leaves(text) {
  const out = [];
  let i = 0;
  let line = 1;
  const ws = () => {
    while (i < text.length && /\s/.test(text[i])) {
      if (text[i] === '\n') line += 1;
      i += 1;
    }
  };
  const str = () => {
    const start = i;
    i += 1;
    while (i < text.length && text[i] !== '"') i += text[i] === '\\' ? 2 : 1;
    i += 1;
    return JSON.parse(text.slice(start, i));
  };
  const value = (path) => {
    ws();
    const c = text[i];
    if (c === '{') {
      i += 1;
      ws();
      if (text[i] === '}') {
        i += 1;
        return;
      }
      for (;;) {
        ws();
        const key = str();
        ws();
        i += 1;
        value(path ? `${path}.${key}` : key);
        ws();
        if (text[i] === ',') {
          i += 1;
          continue;
        }
        i += 1;
        return;
      }
    }
    if (c === '[') {
      i += 1;
      let n = 0;
      for (;;) {
        ws();
        if (text[i] === ']') {
          i += 1;
          return;
        }
        value(`${path}.${n++}`);
        ws();
        if (text[i] === ',') i += 1;
      }
    }
    if (c === '"') {
      const at = line;
      out.push({ key: path, value: str(), line: at });
      return;
    }
    while (i < text.length && !/[,}\]\s]/.test(text[i])) i += 1;
  };
  try {
    value('');
  } catch {
    // A half-written bundle still yields the leaves before the break.
  }
  return out;
}

const dntCache = new Map();

// The brand and protocol words come from the i18n-untranslated check itself, so the
// hook and the sanity sweep cannot disagree about what counts as a translatable word.
function dntWords(root) {
  if (dntCache.has(root)) return dntCache.get(root);
  const set = new Set();
  const ps1 = readText(join(root, 'scripts', 'sanity', 'i18n-untranslated', 'check.ps1')) ?? '';
  const block = ps1.match(/\$dntWords\s*=\s*@\(([\s\S]*?)\n\)/);
  if (block) for (const m of block[1].matchAll(/'([^']+)'/g)) set.add(m[1].toLowerCase());
  dntCache.set(root, set);
  return set;
}

/** translatableWords mirrors Get-SanityTranslatableWords in scripts/sanity/i18n-untranslated/check.ps1. */
export function translatableWords(value, dnt = new Set()) {
  if (/^\s*[{[].*[:"].*[}\]]\s*$/.test(value)) return [];
  let v = value.replace(
    /(\{\{[^}]*\}\}|[0-9]+(?:[.,][0-9]+)?)\s?(?:ms|kib|mib|gib|kb|mb|gb|tb|px|bps)\b|(\{\{[^}]*\}\}|[0-9]+(?:[.,][0-9]+)?)(?:s|m|h|d|b|%)(?![\p{L}])/giu,
    ' ',
  );
  v = v.replace(/\{\{[^}]*\}\}/g, ' ').replace(/\$t\([^)]*\)/g, ' ').replace(/<[^>]*>/g, ' ').replace(/[0-9]+(?:[.,][0-9]+)?/g, ' ');
  const words = [];
  for (const tok of v.split(/[^\p{L}\p{M}'’-]+/u)) {
    const t = tok.replace(/^['’-]+|['’-]+$/g, '');
    if (t.length < 2) continue;
    if (dnt.has(t.toLowerCase())) continue;
    if (/^[A-Z0-9]{2,}$/.test(t)) continue;
    if (/^[a-z]+[A-Z]/.test(t)) continue;
    words.push(t);
  }
  return words;
}

function baseline(root) {
  const set = new Set();
  const text = readText(join(root, '.instructions', 'sanity', 'baselines', 'i18n-untranslated.txt')) ?? '';
  for (const raw of text.split('\n')) {
    const l = raw.replace(/^﻿/, '').trim();
    if (l && !l.startsWith('#')) set.add(l.toLowerCase());
  }
  return set;
}

/**
 * untranslatedFindings flags a changed nl/de/es/fr/pt value byte-identical to en
 * (i18n-keys.md rule 3): an error at three or more translatable words, a warning at
 * one or two, nothing for a frozen cognate in the i18n-untranslated baseline.
 */
function untranslatedFindings({ path, text, changedLines }) {
  const bundle = path ? bundleOf(path) : null;
  if (!bundle || !TRANSLATED_LOCALES.has(bundle.locale)) return [];
  const local = leaves(text).filter((l) => inChanged(changedLines, l.line));
  if (!local.length) return [];
  const enText = readText(bundle.enPath);
  if (enText === null) return [];
  const en = new Map(leaves(enText).map((l) => [l.key, l.value]));
  const frozen = baseline(bundle.root);
  const dnt = dntWords(bundle.root);
  const findings = [];
  for (const l of local) {
    if (typeof l.value !== 'string' || en.get(l.key) !== l.value) continue;
    const words = translatableWords(l.value, dnt);
    if (!words.length) continue;
    if (frozen.has(`${bundle.locale}|${bundle.ns}|${l.key}`.toLowerCase())) continue;
    const long = words.length >= 3;
    findings.push({
      line: l.line,
      rule: 'i18n/untranslated',
      message: `${bundle.locale} value for "${l.key}" is the English text verbatim — translate it, or for a genuine cognate add "${bundle.locale}|${bundle.ns}|${l.key}" to .instructions/sanity/baselines/i18n-untranslated.txt (i18n-keys.md rule 3)`,
      severity: long ? 'error' : 'warn',
    });
  }
  return findings;
}
