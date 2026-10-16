// Regenerate public/themes/mcharbor-default.css from app.css.
//
// `useApplyThemeTokens` injects the selected theme as a <link>, so for a
// user on the default theme this file cascades AFTER app.css and wins
// every token it declares. If the two drift, app.css's values never
// render — which is exactly what happened: this file still held the
// pre-Tidal palette, so the entire Phase 6 colour change was invisible.
//
// Copying rather than hand-maintaining removes the drift by construction.

import console from 'node:console';
import fs from 'node:fs';

const app = fs.readFileSync('app.css', 'utf8');

/** Extract a top-level rule body by selector, brace-matched. */
function block(selector) {
  const start = app.indexOf(`\n${selector} {`);
  if (start === -1) throw new Error(`app.css: no "${selector}" block`);
  const open = app.indexOf('{', start);
  let depth = 0;
  for (let i = open; i < app.length; i++) {
    if (app[i] === '{') depth++;
    else if (app[i] === '}') {
      depth--;
      if (depth === 0) {
        return (
          app
            .slice(open + 1, i)
            // theme-tokens.mjs re-derives the contract per theme.
            .replace(/\n[ \t]*\/\* ── generated[\s\S]*?end generated ── \*\/\n?/g, '\n')
            .replace(/\s+$/, '')
        );
      }
    }
  }
  throw new Error(`app.css: unbalanced braces in "${selector}"`);
}

const out = `/* Copyright (c) 2026 McSparrow. All rights reserved. */
/* McHarbor is licensed under the McHarbor License. See LICENSE for details. */
/* McHarbor default theme — Tidal.
   Deep (dark) and Shallow (light), derived in OKLCH from the logo:
   teal rgb(38,176,198) and deep blue rgb(39,106,162).

   GENERATED from app.css by scripts/sync-default-theme.mjs — do not hand-edit.

   Why it is generated: useApplyThemeTokens injects the active theme as a
   <link>, so for anyone on the default theme this file cascades after
   app.css and wins every token it declares. Hand-maintaining a second copy
   of the same palette guarantees drift, and drift here is silent — the
   app.css values simply stop rendering. */

:root {
${block(':root')}
}

.dark {
${block('.dark')}
}
`;

fs.writeFileSync('public/themes/mcharbor-default.css', out, 'utf8');
console.log('mcharbor-default.css regenerated from app.css');
