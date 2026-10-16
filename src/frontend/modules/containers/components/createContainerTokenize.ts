// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { RestartPolicy } from '../hooks/useCreateContainer';

// dockerRunTokenize splits a shell-style command line into individual
// argument tokens. Supports single quotes, double quotes and
// backslash escapes. This is intentionally minimal — we don't try to
// emulate full shell semantics, just enough to round-trip the flags
// the user is likely to paste (docker run --name foo -p 80:80 nginx).
export function dockerRunTokenize(input: string): string[] {
  // Normalise shell line-continuations BEFORE tokenising. A trailing
  // `\<newline>` (followed by any leading indentation on the next
  // line) glues the two lines together with a single space. The user
  // gets to write `docker run \<newline>   --name foo` and have it
  // behave like `docker run    --name foo` instead of being parsed as
  // three separate tokens.
  const normalised = input.replace(/\\\r?\n[ \t]*/g, ' ');

  const tokens: string[] = [];
  let current = '';
  let quote: '"' | "'" | null = null;
  let i = 0;
  while (i < normalised.length) {
    const ch = normalised.charAt(i);
    if (quote) {
      if (ch === '\\' && quote === '"' && i + 1 < normalised.length) {
        current += normalised.charAt(i + 1);
        i += 2;
        continue;
      }
      if (ch === quote) {
        quote = null;
        i += 1;
        continue;
      }
      current += ch;
      i += 1;
      continue;
    }
    if (ch === '"' || ch === "'") {
      quote = ch;
      i += 1;
      continue;
    }
    if (/\s/.test(ch)) {
      if (current.length > 0) {
        tokens.push(current);
        current = '';
      }
      i += 1;
      continue;
    }
    if (ch === '\\' && i + 1 < normalised.length) {
      current += normalised.charAt(i + 1);
      i += 2;
      continue;
    }
    current += ch;
    i += 1;
  }
  if (current.length > 0) tokens.push(current);
  return tokens;
}

export function splitOnce(value: string, sep: string): [string, string] | null {
  const idx = value.indexOf(sep);
  if (idx < 0) return null;
  return [value.slice(0, idx), value.slice(idx + sep.length)];
}

export function parsePort(value: string): { host: string; container: string; protocol: 'tcp' | 'udp' } | null {
  let proto: 'tcp' | 'udp' = 'tcp';
  let stripped = value;
  if (stripped.endsWith('/udp')) {
    proto = 'udp';
    stripped = stripped.slice(0, -4);
  } else if (stripped.endsWith('/tcp')) {
    proto = 'tcp';
    stripped = stripped.slice(0, -4);
  }
  if (!stripped) return null;
  const parts = stripped.split(':');
  if (parts.length > 2) return null;
  if (parts.length === 1) {
    return { host: '', container: parts[0] ?? '', protocol: proto };
  }
  return { host: parts[0] ?? '', container: parts[1] ?? '', protocol: proto };
}

export function restartFromFlag(value: string): RestartPolicy | null {
  const v = value.toLowerCase();
  if (v === 'no' || v === 'always' || v === 'on-failure' || v === 'unless-stopped') {
    return v;
  }
  return null;
}
