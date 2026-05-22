// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Suspense, lazy } from 'react';

export type CodeEditorProps = {
  value: string;
  onChange?: (value: string) => void;
  readOnly?: boolean;
  language?: 'yaml' | 'javascript' | 'typescript';
  minHeight?: string;
  className?: string;
};

const CodeEditorImpl = lazy(() =>
  import('./CodeEditorImpl').then((module) => ({ default: module.CodeEditorImpl }))
);

function ReadOnlyCodePreview({
  value,
  className,
}: Pick<CodeEditorProps, 'value' | 'className'>) {
  return (
    <div className={`overflow-hidden rounded-lg border border-border bg-card ${className ?? ''}`}>
      <pre className="min-h-[12rem] overflow-auto p-4 font-mono text-xs leading-5 text-foreground whitespace-pre-wrap break-words">
        {value}
      </pre>
    </div>
  );
}

export function CodeEditor({
  value,
  onChange,
  readOnly = false,
  language = 'yaml',
  minHeight = '400px',
  className,
}: CodeEditorProps) {
  if (readOnly && !onChange) {
    return <ReadOnlyCodePreview value={value} className={className} />;
  }

  return (
    <Suspense
      fallback={
        <div className={`overflow-hidden rounded-lg border border-border bg-card ${className ?? ''}`}>
          <div className="min-h-[12rem] animate-pulse bg-muted/60" />
        </div>
      }
    >
      <CodeEditorImpl
        value={value}
        onChange={onChange}
        readOnly={readOnly}
        language={language}
        minHeight={minHeight}
        className={className}
      />
    </Suspense>
  );
}
