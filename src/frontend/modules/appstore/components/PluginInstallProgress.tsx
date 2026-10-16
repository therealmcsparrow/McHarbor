// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCheck, IconLoader2, IconX } from '@tabler/icons-react';
import type { PluginStreamEvent, PluginStreamLogEntry } from '../hooks/useStreamPlugin';

function logColor(currentStatus: string | undefined, isLast: boolean): string {
  if (currentStatus === 'error' && isLast) return 'text-destructive';
  if (currentStatus === 'done' && isLast) return 'text-success';
  if (isLast && currentStatus !== 'done' && currentStatus !== 'error') return 'text-foreground';
  return 'text-muted-foreground';
}

interface PluginInstallProgressProps {
  progress: PluginStreamEvent | null;
  logs: PluginStreamLogEntry[];
}

// The progress body of PluginInstallDialog: status icon, progress bar,
// current phase and the scrolling install log.
export function PluginInstallProgress({ progress, logs }: PluginInstallProgressProps) {
  const { t } = useTranslation(['common', 'plugins']);
  const logEndRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [logs.length]);

  const percent = progress ? Math.round((progress.step / Math.max(progress.total, 1)) * 100) : 0;
  const statusValue = progress?.status;

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-4 py-4">
      <div className="flex items-center gap-3">
        {statusValue === 'done' ? (
          <div className="flex size-8 shrink-0 items-center justify-center bg-success/10">
            <IconCheck className="size-4 text-success" />
          </div>
        ) : statusValue === 'error' ? (
          <div className="flex size-8 shrink-0 items-center justify-center bg-destructive/10">
            <IconX className="size-4 text-destructive" />
          </div>
        ) : (
          <IconLoader2 className="size-5 shrink-0 animate-spin text-primary" />
        )}

        <div className="flex-1">
          <progress
            className={`h-2 w-full overflow-hidden rounded-full bg-muted [&::-moz-progress-bar]:rounded-full [&::-webkit-progress-bar]:rounded-full [&::-webkit-progress-bar]:bg-muted [&::-webkit-progress-value]:rounded-full ${
              statusValue === 'error'
                ? '[&::-moz-progress-bar]:bg-destructive [&::-webkit-progress-value]:bg-destructive'
                : statusValue === 'done'
                  ? '[&::-moz-progress-bar]:bg-success [&::-webkit-progress-value]:bg-success'
                  : '[&::-moz-progress-bar]:bg-primary [&::-webkit-progress-value]:bg-primary'
            }`}
            value={percent}
            max={100}
          />
        </div>

        <span className="shrink-0 text-xs text-muted-foreground tabular-nums">{percent}%</span>
      </div>

      {progress?.phase && (
        <div className="text-xs tracking-wide text-muted-foreground uppercase">
          {t(`install.phases.${progress.phase}`, { ns: 'plugins' })}
        </div>
      )}

      <div className="max-h-[200px] min-h-[120px] overflow-y-auto rounded-md border border-border bg-muted/50 p-3 font-mono text-xs leading-relaxed">
        {logs.map((entry, i) => {
          const isLast = i === logs.length - 1;
          const color = logColor(statusValue, isLast);
          return (
            <div key={entry.id} className="flex gap-2">
              <span className="shrink-0 text-muted-foreground/40 select-none">{String(i + 1).padStart(2, '0')}</span>
              <span className={color}>{entry.message}</span>
            </div>
          );
        })}
        <div ref={logEndRef} />
      </div>
    </div>
  );
}
