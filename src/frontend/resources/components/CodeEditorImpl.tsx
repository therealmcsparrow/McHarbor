// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useRef } from 'react';
import { Compartment, EditorState, type Extension } from '@codemirror/state';
import {
  EditorView,
  drawSelection,
  highlightActiveLine,
  highlightActiveLineGutter,
  keymap,
  lineNumbers,
} from '@codemirror/view';
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
import type { CodeEditorProps } from './CodeEditor';

const editorTheme = EditorView.theme({
  '&': {
    minHeight: '100%',
    backgroundColor: 'hsl(var(--card))',
    color: 'hsl(var(--foreground))',
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
    backgroundColor: 'hsl(var(--muted) / 0.45)',
    color: 'hsl(var(--muted-foreground))',
    borderRight: '1px solid hsl(var(--border))',
  },
  '.cm-activeLine': {
    backgroundColor: 'hsl(var(--accent) / 0.18)',
  },
  '.cm-activeLineGutter': {
    backgroundColor: 'hsl(var(--accent) / 0.28)',
  },
  '.cm-selectionBackground, &.cm-focused .cm-selectionBackground, ::selection': {
    backgroundColor: 'hsl(var(--primary) / 0.25)',
  },
  '&.cm-focused': {
    outline: 'none',
  },
  '.cm-cursor, .cm-dropCursor': {
    borderLeftColor: 'hsl(var(--primary))',
  },
});

const baseExtensions: Extension[] = [
  lineNumbers(),
  history(),
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

async function loadLanguageExtension(
  language: NonNullable<CodeEditorProps['language']>,
): Promise<Extension> {
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

function getMinHeightClass(minHeight: string): string {
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

export function CodeEditorImpl({
  value,
  onChange,
  readOnly = false,
  language = 'yaml',
  minHeight = '400px',
  className,
}: CodeEditorProps) {
  const mountRef = useRef<HTMLDivElement | null>(null);
  const viewRef = useRef<EditorView | null>(null);
  const valueRef = useRef(value);
  const onChangeRef = useRef(onChange);
  const compartments = useMemo(
    () => ({
      language: new Compartment(),
      editable: new Compartment(),
      activeLine: new Compartment(),
    }),
    [],
  );

  onChangeRef.current = onChange;
  valueRef.current = value;

  const wrapperClassName = `overflow-hidden rounded-lg border border-border ${className ?? ''}`;
  const editorHeightClass = minHeight === '100%' ? 'h-full min-h-full' : `h-full ${getMinHeightClass(minHeight)}`;

  useEffect(() => {
    if (!mountRef.current || viewRef.current) {
      return;
    }

    const view = new EditorView({
      state: EditorState.create({
        doc: value,
        extensions: [
          ...baseExtensions,
          compartments.editable.of(EditorView.editable.of(!readOnly)),
          compartments.language.of([]),
          compartments.activeLine.of(
            readOnly ? [] : [highlightActiveLine(), highlightActiveLineGutter()],
          ),
          EditorView.updateListener.of((update) => {
            if (!update.docChanged) {
              return;
            }

            const nextValue = update.state.doc.toString();
            valueRef.current = nextValue;
            onChangeRef.current?.(nextValue);
          }),
        ],
      }),
      parent: mountRef.current,
    });

    viewRef.current = view;

    return () => {
      view.destroy();
      viewRef.current = null;
    };
  }, [compartments, readOnly, value]);

  useEffect(() => {
    const view = viewRef.current;
    if (!view) {
      return;
    }

    const currentValue = view.state.doc.toString();
    if (currentValue === value) {
      return;
    }

    view.dispatch({
      changes: { from: 0, to: currentValue.length, insert: value },
    });
  }, [value]);

  useEffect(() => {
    const view = viewRef.current;
    if (!view) {
      return;
    }

    view.dispatch({
      effects: [
        compartments.editable.reconfigure(EditorView.editable.of(!readOnly)),
        compartments.activeLine.reconfigure(
          readOnly ? [] : [highlightActiveLine(), highlightActiveLineGutter()],
        ),
      ],
    });
  }, [compartments, readOnly]);

  useEffect(() => {
    const view = viewRef.current;
    if (!view) {
      return;
    }

    let cancelled = false;

    async function applyLanguage() {
      const extension = await loadLanguageExtension(language);
      if (cancelled || !viewRef.current) {
        return;
      }

      viewRef.current.dispatch({
        effects: compartments.language.reconfigure(extension),
      });
    }

    void applyLanguage();

    return () => {
      cancelled = true;
    };
  }, [compartments, language]);

  return (
    <div className={wrapperClassName}>
      <div ref={mountRef} className={editorHeightClass} />
    </div>
  );
}
