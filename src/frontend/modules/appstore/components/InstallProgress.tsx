// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { IconLoader2, IconCheck, IconX } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import type { InstallEvent } from '../types';

interface InstallProgressProps {
  progress: InstallEvent | null;
  logs: LogEntry[];
  onClose: () => void;
}

export type LogEntry = {
  id: string;
  message: string;
  phase?: string;
};

function logLineColor(entry: LogEntry, isLast: boolean, status?: string): string {
  if (entry.phase === 'scan-error') return 'text-amber-400';
  if (entry.phase === 'scan-result') return 'text-cyan-400';
  if (entry.phase === 'scan') return 'text-violet-400';
  if (status === 'error' && isLast) return 'text-destructive';
  if (status === 'done' && isLast) return 'text-green-400';
  if (isLast && status !== 'done' && status !== 'error') return 'text-foreground';
  return 'text-muted-foreground';
}

export function InstallProgress({ progress, logs, onClose }: InstallProgressProps) {
  const { t } = useTranslation('common');
  const logEndRef = useRef<HTMLDivElement>(null);

  const progressPercent = progress
    ? Math.round((progress.step / progress.total) * 100)
    : 0;

  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [logs.length]);

  return (
    <div className="flex flex-col gap-4 py-4">
      {/* Status header */}
      <div className="flex items-center gap-3">
        {progress?.status === 'done' ? (
          <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-green-500/10">
            <IconCheck className="size-4 text-green-500" />
          </div>
        ) : progress?.status === 'error' ? (
          <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-destructive/10">
            <IconX className="size-4 text-destructive" />
          </div>
        ) : (
          <IconLoader2 className="size-5 shrink-0 animate-spin text-primary" />
        )}

        {/* Progress bar */}
        <div className="flex-1">
          <div className="h-2 w-full overflow-hidden rounded-full bg-muted">
            <div
              className={`h-full rounded-full transition-all duration-500 ${
                progress?.status === 'error'
                  ? 'bg-destructive'
                  : progress?.status === 'done'
                    ? 'bg-green-500'
                    : 'bg-primary'
              }`}
              style={{ width: `${progressPercent}%` }}
            />
          </div>
        </div>

        <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
          {progressPercent}%
        </span>
      </div>

      {/* Log output */}
      <div className="max-h-[200px] min-h-[120px] overflow-y-auto rounded-md border border-border bg-muted/50 p-3 font-mono text-xs leading-relaxed">
        {logs.map((entry, i) => {
          const isLast = i === logs.length - 1;
          const color = logLineColor(entry, isLast, progress?.status);
          return (
            <div key={entry.id} className="flex gap-2">
              <span className="shrink-0 select-none text-muted-foreground/40">
                {String(i + 1).padStart(2, '0')}
              </span>
              <span className={color}>
                {entry.phase === 'scan' && '🔍 '}
                {entry.phase === 'scan-result' && '📊 '}
                {entry.phase === 'scan-error' && '⚠ '}
                {entry.message}
              </span>
            </div>
          );
        })}
        <div ref={logEndRef} />
      </div>

      {/* Close button when done or errored */}
      {(progress?.status === 'done' || progress?.status === 'error') && (
        <Button
          size="sm"
          variant={progress.status === 'done' ? 'default' : 'outline'}
          onClick={onClose}
          className="self-end"
        >
          {progress.status === 'done' ? t('appStore.done') : t('actions.close')}
        </Button>
      )}
    </div>
  );
}

