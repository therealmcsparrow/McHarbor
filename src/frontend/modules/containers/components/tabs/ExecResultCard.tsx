// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';

export type ExecResult = {
  stdout: string;
  stderr: string;
  exitCode: number;
  durationMs: number;
};

export function ExecResultCard({ lastResult }: { lastResult: ExecResult }) {
  const { t } = useTranslation('containers');
  return (
    <div className="mc-glass rounded-lg p-4">
      <div className="mb-2 flex items-center justify-between text-xs">
        <span className="font-semibold tracking-wider text-muted-foreground uppercase">{t('exec.lastResult')}</span>
        <span className={lastResult.exitCode === 0 ? 'text-success' : 'text-destructive'}>
          {t('exec.exitSummary', { code: lastResult.exitCode, ms: lastResult.durationMs })}
        </span>
      </div>
      {lastResult.stdout && (
        <pre className="max-h-64 overflow-auto rounded bg-muted p-3 font-mono text-xs whitespace-pre-wrap">
          {lastResult.stdout}
        </pre>
      )}
      {lastResult.stderr && (
        <pre className="mt-2 max-h-64 overflow-auto rounded bg-destructive/10 p-3 font-mono text-xs whitespace-pre-wrap text-destructive">
          {lastResult.stderr}
        </pre>
      )}
    </div>
  );
}
