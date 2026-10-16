// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The two header lines CLAUDE.md's "Copyright header" section requires on
// every new source file, verbatim, so a match is exact rather than fuzzy.
const HEADER_LINES = [
  'Copyright (c) 2026 McSparrow. All rights reserved.',
  'McHarbor is licensed under the McHarbor License. See LICENSE for details.',
];

/** check flags a file whose opening lines are missing the copyright header. */
export function check({ text }) {
  const lines = text.split('\n');
  // A shebang, when present, comes before the header in every language this
  // repo uses it in.
  const start = lines[0]?.startsWith('#!') ? 1 : 0;
  const window = lines.slice(start, start + 4).join('\n');
  const hasHeader = HEADER_LINES.every((line) => window.includes(line));
  if (hasHeader) return [];

  return [
    {
      line: 1,
      rule: 'copyright/missing',
      message: `add the two-line copyright header before any code (CLAUDE.md § Copyright header): "${HEADER_LINES[0]}"`,
      severity: 'error',
    },
  ];
}
