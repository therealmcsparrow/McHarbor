// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { EditorState, type Extension } from '@codemirror/state';
import { EditorView, drawSelection, keymap, lineNumbers } from '@codemirror/view';
import { defaultKeymap, history as cmHistory, historyKeymap, indentWithTab } from '@codemirror/commands';
import type { CodeEditorProps } from './CodeEditor';

const editorTheme = EditorView.theme({
  '&': {
    minHeight: '100%',
    backgroundColor: 'var(--card)',
    color: 'var(--foreground)',
  },
  '.cm-scroller': {
    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
    minHeight: '100%',
  },
  '.cm-content': {
    minHeight: '100%',
    padding: '0.75rem 0',
  },
  '.cm-gutters': {
    backgroundColor: 'color-mix(in oklch, var(--muted) 45%, transparent)',
    color: 'var(--muted-foreground)',
    borderRight: '1px solid var(--border)',
  },
  '.cm-activeLine': {
    backgroundColor: 'color-mix(in oklch, var(--accent) 18%, transparent)',
  },
  '.cm-activeLineGutter': {
    backgroundColor: 'color-mix(in oklch, var(--accent) 28%, transparent)',
  },
  '.cm-selectionBackground, &.cm-focused .cm-selectionBackground, ::selection': {
    backgroundColor: 'color-mix(in oklch, var(--primary) 25%, transparent)',
  },
  '&.cm-focused': {
    outline: 'none',
  },
  '.cm-cursor, .cm-dropCursor': {
    borderLeftColor: 'var(--primary)',
  },
});

export const baseExtensions: Extension[] = [
  lineNumbers(),
  cmHistory(),
  drawSelection(),
  EditorState.tabSize.of(2),
  EditorView.lineWrapping,
  keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab]),
  EditorView.theme({
    '.cm-line': {
      padding: '0 0.75rem',
    },
  }),
  editorTheme,
];

export async function loadLanguageExtension(language: NonNullable<CodeEditorProps['language']>): Promise<Extension> {
  if (language === 'yaml') {
    const { yaml } = await import('@codemirror/lang-yaml');
    return yaml();
  }

  if (language === 'javascript') {
    const { javascript } = await import('@codemirror/lang-javascript');
    return javascript();
  }

  if (language === 'typescript') {
    const { javascript } = await import('@codemirror/lang-javascript');
    return javascript({ typescript: true });
  }

  return [];
}

export function getMinHeightClass(minHeight: string): string {
  switch (minHeight) {
    case '100%':
      return 'min-h-full';
    case '120px':
      return 'min-h-[120px]';
    case '200px':
      return 'min-h-[200px]';
    case '300px':
      return 'min-h-[300px]';
    case '400px':
      return 'min-h-[400px]';
    default:
      return 'min-h-[400px]';
  }
}
