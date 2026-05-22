// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef } from 'react';
import {
  IconAlertCircle,
  IconCircleCheckFilled,
  IconLoader2,
  IconClock,
} from '@tabler/icons-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { cn } from '@resources/utils/cn';
import type {
  BatchProgressDialogState,
  BatchProgressItem,
} from '@resources/hooks/useBatchProgressOperation';

type OperationProgressDialogProps = {
  state: BatchProgressDialogState;
  onClose: () => void;
};

function getHeaderIcon(status: BatchProgressDialogState['status']) {
  if (status === 'success') {
    return (
      <div className="flex size-11 items-center justify-center rounded-2xl bg-emerald-500/10 text-emerald-400">
        <IconCircleCheckFilled className="size-5" />
      </div>
    );
  }

  if (status === 'error') {
    return (
      <div className="flex size-11 items-center justify-center rounded-2xl bg-amber-500/10 text-amber-400">
        <IconAlertCircle className="size-5" />
      </div>
    );
  }

  return (
    <div className="flex size-11 items-center justify-center rounded-2xl bg-primary/10 text-primary">
      <IconLoader2 className="size-5 animate-spin" />
    </div>
  );
}

function getItemIcon(item: BatchProgressItem) {
  if (item.status === 'success') {
    return <IconCircleCheckFilled className="size-4 text-emerald-400" />;
  }
  if (item.status === 'error') {
    return <IconAlertCircle className="size-4 text-amber-400" />;
  }
  if (item.status === 'running') {
    return <IconLoader2 className="size-4 animate-spin text-primary" />;
  }
  return <IconClock className="size-4 text-muted-foreground" />;
}

export function OperationProgressDialog({ state, onClose }: OperationProgressDialogProps) {
  const { t } = useTranslation('common');
  const logEndRef = useRef<HTMLDivElement>(null);
  const segmentCount = Math.min(Math.max(state.total, 1), 12);
  const filledSegments = state.total === 0
    ? 0
    : Math.round((state.completed / state.total) * segmentCount);

  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [state.logs.length]);

  const headline = state.status === 'running'
    ? t('operations.running', { action: state.actionLabel, name: state.currentItemLabel ?? '' })
    : state.status === 'success'
      ? t('operations.completed', { action: state.actionLabel })
      : t('operations.completedWithErrors', {
          action: state.actionLabel,
          count: state.failureCount,
        });

  return (
    <Dialog open={state.open} onOpenChange={(open) => !open && onClose()}>
      <DialogContent
        className={cn(
          'max-w-2xl p-0',
          state.status === 'running' && '[&>button.absolute]:hidden',
        )}
      >
        <DialogHeader className="space-y-1">
          <DialogTitle>{state.title}</DialogTitle>
          <DialogDescription>{state.description}</DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-5 p-5">
          <div className="rounded-2xl border border-border bg-muted/35 p-4">
            <div className="flex items-center gap-3">
              {getHeaderIcon(state.status)}
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium text-foreground">{headline}</p>
                <p className="text-xs text-muted-foreground">
                  {t('operations.progress', { completed: state.completed, total: state.total })}
                </p>
              </div>
              <span className="text-xs tabular-nums text-muted-foreground">
                {state.completed}/{state.total}
              </span>
            </div>

            <div className={cn('mt-4 grid gap-1.5', segmentCount === 1 ? 'grid-cols-1' : 'grid-cols-12')}>
              {Array.from({ length: segmentCount }).map((_, index) => (
                <div
                  key={`progress-segment-${index + 1}`}
                  className={cn(
                    'h-2 rounded-full bg-border/80 transition-colors',
                    index < filledSegments &&
                      (state.status === 'error' ? 'bg-amber-400' : 'bg-primary'),
                  )}
                />
              ))}
            </div>
          </div>

          <div className="max-h-[320px] overflow-y-auto rounded-2xl border border-border bg-card/60">
            <div className="divide-y divide-border">
              {state.items.map((item) => (
                <div key={item.key} className="flex items-start gap-3 px-4 py-3">
                  <div className="mt-0.5">{getItemIcon(item)}</div>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <p className="truncate text-sm font-medium text-foreground">{item.label}</p>
                      <span className="text-[11px] uppercase tracking-[0.14em] text-muted-foreground">
                        {t(`operations.itemStatus.${item.status}`)}
                      </span>
                    </div>
                    <p className="mt-0.5 truncate text-xs text-muted-foreground">
                      {item.detail ?? t(`operations.itemFallback.${item.status}`)}
                    </p>
                  </div>
                </div>
              ))}
            </div>
          </div>

          <div className="overflow-hidden rounded-2xl border border-border bg-muted/25">
            <div className="border-b border-border px-4 py-3">
              <p className="text-sm font-medium text-foreground">{t('operations.logTitle')}</p>
              <p className="text-xs text-muted-foreground">{t('operations.logEmpty')}</p>
            </div>
            <div className="max-h-[240px] overflow-y-auto px-4 py-3 font-mono text-xs leading-6">
              {state.logs.length === 0 ? (
                <p className="text-muted-foreground">{t('operations.logEmpty')}</p>
              ) : (
                state.logs.map((entry) => (
                  <div key={entry.id} className="flex gap-3">
                    <span className="shrink-0 text-muted-foreground/60">
                      {new Date(entry.timestamp).toLocaleTimeString([], {
                        hour: '2-digit',
                        minute: '2-digit',
                        second: '2-digit',
                        hour12: false,
                      })}
                    </span>
                    <span className="shrink-0 text-muted-foreground/60">
                      {entry.itemLabel ?? 'system'}
                    </span>
                    <span
                      className={cn(
                        'min-w-0 break-words',
                        entry.level === 'success' && 'text-emerald-400',
                        entry.level === 'warning' && 'text-amber-400',
                        entry.level === 'error' && 'text-destructive',
                        entry.level === 'info' && 'text-foreground',
                      )}
                    >
                      {entry.message}
                    </span>
                  </div>
                ))
              )}
              <div ref={logEndRef} />
            </div>
          </div>

        </DialogBody>
        {state.status !== 'running' && (
          <DialogFooter>
            <Button onClick={onClose}>{t('actions.close')}</Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}
