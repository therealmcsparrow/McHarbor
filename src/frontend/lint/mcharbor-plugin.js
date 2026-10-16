// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import path from 'node:path';
import { definePlugin, defineRule } from 'vite-plus/lint/plugins';

// Oxlint has no native no-restricted-syntax, so this is ESLint's rule rebuilt on the selector
// support JS plugins get: each { selector, message } option becomes one visitor key.
const noRestrictedSyntax = defineRule({
  meta: { type: 'suggestion', schema: false },
  create(context) {
    const visitors = {};
    for (const option of context.options) {
      const { selector, message } = typeof option === 'string' ? { selector: option, message: undefined } : option;
      const report = (node) => context.report({ node, message: message ?? `Using '${selector}' is not allowed.` });
      const previous = visitors[selector];
      visitors[selector] = previous
        ? (node) => {
            previous(node);
            report(node);
          }
        : report;
    }
    return visitors;
  },
});

const frontendDir = path.resolve(import.meta.dirname, '..');
const modulesDir = path.join(frontendDir, 'modules');

function moduleOf(file) {
  const rel = path.relative(modulesDir, file);
  if (!rel || rel.startsWith('..') || path.isAbsolute(rel)) return undefined;
  const parts = rel.split(path.sep);
  // A file directly under modules/ (a cross-module test) belongs to no module.
  return parts.length > 1 ? parts[0] : undefined;
}

function targetModule(source, fromFile) {
  const aliased = /^@\/?modules\/([^/]+)/.exec(source);
  if (aliased) return aliased[1];
  if (source.startsWith('.')) return moduleOf(path.resolve(path.dirname(fromFile), source));
  return undefined;
}

// Replaces eslint-plugin-boundaries: modules/X never imports modules/Y (module-boundaries.md
// rule 5). `allow` maps a module to the siblings it may reach, mirroring the old policies.
const moduleBoundaries = defineRule({
  meta: { type: 'problem', schema: false },
  create(context) {
    const from = moduleOf(path.resolve(context.filename));
    if (!from) return {};
    const allowed = new Set(context.options[0]?.allow?.[from] ?? []);
    const check = (node, sourceNode) => {
      if (!sourceNode || typeof sourceNode.value !== 'string') return;
      const to = targetModule(sourceNode.value, path.resolve(context.filename));
      if (!to || to === from || allowed.has(to)) return;
      context.report({
        node,
        message: `modules/${from} must not import modules/${to}; move shared code to @resources or @core (module-boundaries.md)`,
      });
    };
    return {
      ImportDeclaration: (node) => check(node, node.source),
      ExportNamedDeclaration: (node) => check(node, node.source),
      ExportAllDeclaration: (node) => check(node, node.source),
      ImportExpression: (node) => check(node, node.source),
    };
  },
});

// Replaces eslint-plugin-headers, whose default options fail Oxlint's schema check at load time.
// The header sits after an optional shebang, as CLAUDE.md § Copyright header requires.
const copyrightHeader = defineRule({
  meta: { type: 'layout', fixable: 'code', schema: false },
  create(context) {
    return {
      Program(node) {
        const lines = context.options[0]?.lines ?? [];
        const text = context.sourceCode.text;
        const start = text.startsWith('#!') ? text.indexOf('\n') + 1 : 0;
        const expected = lines.map((line) => `// ${line}`).join('\n');
        const actual = text.slice(start).replace(/\r\n/g, '\n').slice(0, expected.length);
        if (actual === expected) return;
        context.report({
          node,
          message: 'Missing or incorrect copyright header (CLAUDE.md § Copyright header)',
          fix: (fixer) => fixer.insertTextBeforeRange([start, start], `${expected}\n`),
        });
      },
    };
  },
});

export default definePlugin({
  meta: { name: 'mcharbor' },
  rules: {
    'no-restricted-syntax': noRestrictedSyntax,
    'module-boundaries': moduleBoundaries,
    'copyright-header': copyrightHeader,
  },
});
