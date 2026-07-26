// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useRef, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { IconX, IconCheck, IconAlertCircle, IconLoader2 } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { Button } from './ui/Button';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from './ui/Dialog';

export type BulkItemStatus = 'pending' | 'running' | 'success' | 'failed' | 'skipped';

export type BulkItem<T = unknown> = {
  id: string;
  label: string;
  data?: T;
  status: BulkItemStatus;
  error?: string;
};

export type BulkActionBarProps = {
  selectedCount: number;
  totalCount: number;
  actions: { id: string; label: string; destructive?: boolean; run: () => void | Promise<void> }[];
  onClear: () => void;
};

export function BulkActionBar({
  selectedCount,
  totalCount,
  actions,
  onClear,
}: BulkActionBarProps) {
  const { t } = useTranslation('common');
  if (selectedCount === 0) return null;
  return (
    <div
      className="sticky top-0 z-10 flex items-center justify-between gap-3 border-b border-border bg-card px-4 py-2 text-sm"
      role="region"
      aria-label={t('bulk.barLabel', { defaultValue: 'Bulk actions' })}
    >
      <div className="flex items-center gap-3">
        <span className="font-medium">
          {t('bulk.selected', {
            defaultValue: `${selectedCount} of ${totalCount} selected`,
            selectedCount,
            totalCount,
          })}
        </span>
        <Button variant="ghost" size="sm" onClick={onClear}>
          {t('bulk.clear', { defaultValue: 'Clear selection' })}
        </Button>
      </div>
      <div className="flex items-center gap-2">
        {actions.map((action) => (
          <Button
            key={action.id}
            size="sm"
            variant={action.destructive ? 'destructive' : 'default'}
            onClick={() => void action.run()}
          >
            {action.label}
          </Button>
        ))}
      </div>
    </div>
  );
}

type BulkProgressDialogProps<T> = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  items: BulkItem<T>[];
  onCancel?: () => void;
};

export function BulkProgressDialog<T>({
  open,
  onOpenChange,
  title,
  items,
  onCancel,
}: BulkProgressDialogProps<T>) {
  const { t } = useTranslation('common');
  const success = items.filter((i) => i.status === 'success').length;
  const failed = items.filter((i) => i.status === 'failed').length;
  const running = items.filter((i) => i.status === 'running').length;
  const total = items.length;
  const done = success + failed;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>
            {t('bulk.progress', {
              defaultValue: `${done} of ${total} complete (${success} succeeded, ${failed} failed, ${running} running)`,
              done,
              total,
              success,
              failed,
              running,
            })}
          </DialogDescription>
        </DialogHeader>
        <div className="max-h-[50vh] space-y-1 overflow-y-auto pr-2">
          {items.map((item) => (
            <BulkRow key={item.id} item={item} />
          ))}
        </div>
        <DialogFooter>
          {onCancel && (
            <Button variant="ghost" onClick={onCancel}>
              {t('actions.cancel', { defaultValue: 'Cancel' })}
            </Button>
          )}
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.close', { defaultValue: 'Close' })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function BulkRow<T>({ item }: { item: BulkItem<T> }) {
  return (
    <div
      className={cn(
        'flex items-center justify-between gap-2 rounded-md border border-border bg-card px-3 py-2 text-sm',
        item.status === 'failed' && 'border-destructive/40',
      )}
    >
      <span className="truncate">{item.label}</span>
      <BulkStatusIcon status={item.status} error={item.error} />
    </div>
  );
}

function BulkStatusIcon({ status, error }: { status: BulkItemStatus; error?: string }) {
  if (status === 'pending')
    return <span className="size-4 rounded-full border-2 border-muted" />;
  if (status === 'running')
    return <IconLoader2 className="size-4 animate-spin text-primary" />;
  if (status === 'success') return <IconCheck className="size-4 text-green-600" />;
  if (status === 'failed') {
    return (
      <span title={error}>
        <IconAlertCircle className="size-4 text-destructive" />
      </span>
    );
  }
  return <IconX className="size-4 text-muted-foreground" />;
}

type BatchProgressState<T> = {
  items: BulkItem<T>[];
  setItems: (updater: (prev: BulkItem<T>[]) => BulkItem<T>[]) => void;
  reset: (items: Omit<BulkItem<T>, 'status'>[]) => void;
  update: (id: string, patch: Partial<BulkItem<T>>) => void;
};

export function useBatchProgress<T = unknown>(): BatchProgressState<T> {
  const [items, setItems] = useState<BulkItem<T>[]>([]);
  const itemsRef = useRef(items);
  itemsRef.current = items;

  const reset = useCallback((initial: Omit<BulkItem<T>, 'status'>[]) => {
    setItems(initial.map((i) => ({ ...i, status: 'pending' as const })));
  }, []);

  const update = useCallback((id: string, patch: Partial<BulkItem<T>>) => {
    setItems((prev) => prev.map((i) => (i.id === id ? { ...i, ...patch } : i)));
  }, []);

  return { items, setItems, reset, update };
}

type RunBatchOptions<T> = {
  state: BatchProgressState<T>;
  items: BulkItem<T>[];
  runner: (item: BulkItem<T>, signal: AbortSignal) => Promise<void>;
  concurrency?: number;
  signal?: AbortSignal;
};

export async function runBatch<T>({
  state,
  items,
  runner,
  concurrency = 4,
  signal,
}: RunBatchOptions<T>): Promise<{ success: number; failed: number }> {
  let success = 0;
  let failed = 0;
  const queue = [...items];
  const inFlight = new Set<Promise<void>>();

  const abortHandler = () => {
    if (signal?.aborted) {
      for (const itm of items) {
        if (itm.status === 'pending' || itm.status === 'running') {
          state.update(itm.id, { status: 'skipped' });
        }
      }
    }
  };
  signal?.addEventListener('abort', abortHandler);

  while (queue.length > 0 || inFlight.size > 0) {
    if (signal?.aborted) break;
    while (inFlight.size < concurrency && queue.length > 0) {
      const item = queue.shift()!;
      state.update(item.id, { status: 'running' });
      const p = (async () => {
        const controller = new AbortController();
        try {
          await runner(item, controller.signal);
          state.update(item.id, { status: 'success' });
          success++;
        } catch (err) {
          const message = err instanceof Error ? err.message : String(err);
          state.update(item.id, { status: 'failed', error: message });
          failed++;
        }
      })();
      inFlight.add(p);
      p.finally(() => inFlight.delete(p));
    }
    if (inFlight.size > 0) {
      await Promise.race(inFlight);
    }
  }

  signal?.removeEventListener('abort', abortHandler);
  return { success, failed };
}


