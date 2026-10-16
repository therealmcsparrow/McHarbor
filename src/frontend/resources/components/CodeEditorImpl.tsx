// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useRef } from 'react';
import { Compartment, EditorState } from '@codemirror/state';
import { EditorView, highlightActiveLine, highlightActiveLineGutter } from '@codemirror/view';
import { baseExtensions, getMinHeightClass, loadLanguageExtension } from './codeEditorSetup';
import type { CodeEditorProps } from './CodeEditor';

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

  useEffect(() => {
    onChangeRef.current = onChange;
    valueRef.current = value;
  }, [onChange, value]);

  const wrapperClassName = `overflow-hidden rounded-lg border border-border ${className ?? ''}`;
  const editorHeightClass = minHeight === '100%' ? 'h-full min-h-full' : `h-full ${getMinHeightClass(minHeight)}`;

  useEffect(() => {
    if (!mountRef.current || viewRef.current) {
      return;
    }

    // The view is built once and fed later values by the sync effect below. Rebuilding it on
    // every value change destroyed the focused editor on each keystroke, and a dialog's focus
    // trap then moved focus to the dialog itself.
    const view = new EditorView({
      state: EditorState.create({
        doc: valueRef.current,
        extensions: [
          ...baseExtensions,
          compartments.editable.of(EditorView.editable.of(!readOnly)),
          compartments.language.of([]),
          compartments.activeLine.of(readOnly ? [] : [highlightActiveLine(), highlightActiveLineGutter()]),
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
  }, [compartments, readOnly]);

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
        compartments.activeLine.reconfigure(readOnly ? [] : [highlightActiveLine(), highlightActiveLineGutter()]),
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
