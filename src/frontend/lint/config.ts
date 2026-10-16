// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import path from 'node:path';
import type { UserConfig } from 'vite-plus';

type LintConfig = NonNullable<UserConfig['lint']>;
type FmtConfig = NonNullable<UserConfig['fmt']>;

const frontendDir = path.resolve(import.meta.dirname, '..');

// Shared across host, widgets, nodes and plugin frontends. The plugin-slug literal check
// is deliberately NOT here: plugins may name themselves, so it only applies to
// core/**, modules/** and resources/** below (see host-plugin-boundary.md).
const commonRestrictedSyntax = [
  {
    selector: "JSXOpeningElement[name.name='button']",
    message: 'Use <Button> or <PlainButton> from @resources/components/ui',
  },
  {
    selector: "JSXAttribute[name.name='style'] > JSXExpressionContainer > ObjectExpression",
    message: 'No inline style objects; Tailwind, or style={cssVars({...})} + w-(--w)',
  },
  {
    selector: "CallExpression[callee.name='useTranslation'][arguments.length=0]",
    message: "Name the namespace: useTranslation('<ns>')",
  },
  {
    selector: "CallExpression[callee.name='t'] Property[key.name='defaultValue']",
    message:
      'No defaultValue (i18n-keys rule 4). Only for an open-ended computed key: // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: <reason>',
  },
  {
    selector: "TSAsExpression[expression.type='TSAsExpression'][expression.typeAnnotation.type='TSUnknownKeyword']",
    message:
      'Double cast: fix the type or narrow with a type guard. At a real boundary: // eslint-disable-next-line mcharbor/no-restricted-syntax -- unavoidable cast: <reason>',
  },
  {
    selector: 'CallExpression[callee.property.name=/^toLocale(Date|Time)?String$/]',
    message: 'Use formatDate / formatDateCustom from @resources/utils/format',
  },
  {
    selector:
      "Property[key.name='queryFn'] CallExpression[callee.property.name='then'] > ArrowFunctionExpression > MemberExpression[property.name='data']",
    message: 'queryFn must end in .then(assertSuccess) so failures reach isError',
  },
  {
    selector: "Property[key.name='onError'] > ArrowFunctionExpression[params.length=0]",
    message: 'The global MutationCache already toasts the server message; drop this onError or use meta.error',
  },
];

// core/modules/resources only: the plugin-slug literal is a host-authored string, so a plugin
// naming its own slug in its own frontend is not a violation of this rule.
const hostRestrictedSyntax = [
  ...commonRestrictedSyntax,
  {
    selector:
      'Literal[value=/^(?!storage-breakdown$|snapshot-status$)(addon|ai-provider|comm|identity|logging|scanner|snapshot|storage)-(?![a-z0-9-]*-select$)[a-z0-9-]+$/]',
    message: 'Host code must not name a plugin slug (host-plugin-boundary.md)',
  },
];

// Plugin frontends only. jsx-only i18next/no-literal-string cannot see prose that lives in a
// data structure (a steps array, an options table), which is how comm-* setup guides shipped
// untranslated. Placeholders are deliberately not listed: example hosts and ports stay literal.
const pluginRestrictedSyntax = [
  ...commonRestrictedSyntax,
  {
    selector:
      'Property[key.name=/^(label|title|help|description|hint|tooltip)$/] > Literal[value=/^(?![a-z][A-Za-z0-9_-]*[.][A-Za-z0-9_.]+$).*[A-Za-z]{2}/]',
    message:
      'User-facing text in a data table must go through t(). A protocol or product name: // eslint-disable-next-line mcharbor/no-restricted-syntax -- do-not-translate: <reason>',
  },
  {
    selector: 'ArrayExpression > Literal[value=/[^ ]+ [^ ]+ [^ ]+/]',
    message: 'Prose in an array must be an i18n key resolved through t()',
  },
];

const testFiles = ['**/*.test.{ts,tsx}', '**/*.spec.ts', 'tests/**', '**/*.stories.tsx'];

const generated = [
  '**/dist/**',
  '**/node_modules/**',
  '**/*.umd.cjs',
  '**/coverage/**',
  '**/playwright-report/**',
  '**/test-results/**',
  '**/tsconfig.tsbuildinfo',
];

export const lint: LintConfig = {
  plugins: ['typescript', 'react', 'jsx-a11y', 'unicorn', 'oxc'],
  jsPlugins: ['./lint/mcharbor-plugin.js', 'eslint-plugin-i18next', 'eslint-plugin-better-tailwindcss'],
  categories: { correctness: 'error' },
  env: { browser: true, es2024: true },
  ignorePatterns: generated,
  options: { typeAware: true, typeCheck: true },
  rules: {
    'react/rules-of-hooks': 'error',
    'react/exhaustive-deps': 'warn',
    'no-unused-vars': ['error', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }],
    'no-control-regex': 'off',
  },
  overrides: [
    {
      files: ['**/*.{ts,tsx}'],
      rules: {
        'mcharbor/copyright-header': [
          'error',
          {
            lines: [
              'Copyright (c) 2026 McSparrow. All rights reserved.',
              'McHarbor is licensed under the McHarbor License. See LICENSE for details.',
            ],
          },
        ],
        'i18next/no-literal-string': [
          'error',
          {
            mode: 'jsx-only',
            'jsx-attributes': { include: ['title', 'placeholder', 'aria-label', 'alt', 'label'] },
            // Oxlint hands the rule JSX text with entities undecoded (`&rarr;`), so the plugin's own
            // decoded-entity and emoji defaults never match; these restate them for raw text.
            words: {
              exclude: [
                '[0-9!-/:-@[-`{-~]+',
                '[A-Z_-]+',
                '(&[A-Za-z]+;|&#[0-9]+;|[0-9!-/:-@[-`{-~\\s\\u00A0-\\u00BF\\u00D7\\u00F7\\u2000-\\u2BFF\\uD800-\\uDFFF\\uFE0F])+',
              ],
            },
          },
        ],
        'jsx-a11y/click-events-have-key-events': 'error',
        'jsx-a11y/no-static-element-interactions': 'error',
        'no-alert': 'error',
        'no-empty': ['error', { allowEmptyCatch: false }],
        'typescript/no-explicit-any': 'error',
        'no-shadow': ['error', { builtinGlobals: true, ignoreTypeValueShadow: true }],
        'max-lines': ['error', { max: 300, skipBlankLines: true, skipComments: true }],
        // tailwindcss is only installed under src/frontend, so cwd is pinned for the
        // widgets/nodes/plugin trees that are linted from outside it.
        'better-tailwindcss/no-restricted-classes': [
          'error',
          {
            entryPoint: path.join(frontendDir, 'app.css'),
            cwd: frontendDir,
            restrict: [
              {
                pattern: '^(bg|text|border|ring|fill|stroke)-(white|black|(gray|slate|zinc|neutral)-\\d+)$',
                message: 'Use semantic OKLCH tokens (bg-card, text-muted-foreground, …)',
              },
              { pattern: '\\[#[0-9a-fA-F]{3,8}\\]', message: 'No raw hex' },
            ],
          },
        ],
        'mcharbor/no-restricted-syntax': ['error', ...commonRestrictedSyntax],
      },
    },
    {
      files: ['resources/**/*.{ts,tsx}', 'core/**/*.{ts,tsx}'],
      rules: {
        'no-restricted-imports': [
          'error',
          { patterns: [{ group: ['@modules/*'], message: 'Shared code must not import a feature module' }] },
        ],
      },
    },
    // core/router/ holds what core/router.tsx was split into for max-lines, plus the shell
    // wiring that registers feature components into the shell's slots: the composition root.
    {
      files: ['core/router.tsx', 'core/router/**'],
      rules: { 'no-restricted-imports': 'off' },
    },
    {
      files: ['core/**/*.{ts,tsx}', 'modules/**/*.{ts,tsx}', 'resources/**/*.{ts,tsx}'],
      rules: { 'mcharbor/no-restricted-syntax': ['error', ...hostRestrictedSyntax] },
    },
    {
      files: ['modules/**/*.{ts,tsx}'],
      rules: { 'mcharbor/module-boundaries': ['error', { allow: { oncall: ['runbooks', 'incidents'] } }] },
    },
    {
      files: ['**/src/plugins/Plugins/*/frontend/src/**/*.{ts,tsx}'],
      rules: { 'mcharbor/no-restricted-syntax': ['error', ...pluginRestrictedSyntax] },
    },
    {
      files: testFiles,
      rules: {
        'mcharbor/copyright-header': 'off',
        'i18next/no-literal-string': 'off',
        'jsx-a11y/click-events-have-key-events': 'off',
        'jsx-a11y/no-static-element-interactions': 'off',
        'no-alert': 'off',
        'no-empty': 'off',
        'typescript/no-explicit-any': 'off',
        'no-shadow': 'off',
        'max-lines': 'off',
        'better-tailwindcss/no-restricted-classes': 'off',
        'mcharbor/no-restricted-syntax': 'off',
      },
    },
  ],
};

export const fmt: FmtConfig = {
  printWidth: 120,
  singleQuote: true,
  jsxSingleQuote: false,
  semi: true,
  trailingComma: 'all',
  arrowParens: 'always',
  endOfLine: 'lf',
  sortTailwindcss: {
    stylesheet: './app.css',
    functions: ['cn', 'clsx', 'cva', 'twMerge'],
  },
  // Locale bundles keep their PowerShell house style (4-space indent), and lockfiles and
  // committed plugin bundles are generated.
  ignorePatterns: [
    ...generated,
    'core/vendor/**',
    '**/locales/**/*.json',
    '**/i18n/**/*.json',
    '**/package-lock.json',
    '**/public/**',
    '**/__snapshots__/**',
  ],
};
