// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { toast } from 'sonner';

export type BatchProgressStatus = 'idle' | 'running' | 'success' | 'error';
export type BatchProgressItemStatus = 'pending' | 'running' | 'success' | 'error';
export type BatchProgressLogLevel = 'info' | 'success' | 'warning' | 'error';

export type BatchProgressItem = {
  key: string;
  label: string;
  status: BatchProgressItemStatus;
  detail?: string;
};

export type BatchProgressLogEntry = {
  id: string;
  itemKey?: string;
  itemLabel?: string;
  level: BatchProgressLogLevel;
  message: string;
  timestamp: string;
};

export type BatchProgressDialogState = {
  open: boolean;
  title: string;
  description: string;
  actionLabel: string;
  status: BatchProgressStatus;
  items: BatchProgressItem[];
  total: number;
  completed: number;
  currentItemLabel?: string;
  failureCount: number;
  logs: BatchProgressLogEntry[];
};

export type BatchProgressResult<T> = {
  total: number;
  successCount: number;
  failureCount: number;
  successfulItems: T[];
  failedItems: Array<{ item: T; error: Error }>;
};

type RunBatchProgressOptions<T> = {
  title: string;
  description: string;
  actionLabel: string;
  items: T[];
  getKey: (item: T) => string;
  getLabel: (item: T) => string;
  execute: (item: T, context: BatchProgressContext) => Promise<{ detail?: string } | void>;
  onComplete?: (result: BatchProgressResult<T>) => Promise<void> | void;
  getSuccessToast?: (result: BatchProgressResult<T>) => string | undefined;
  getErrorToast?: (result: BatchProgressResult<T>) => string | undefined;
};

export type BatchProgressContext = {
  setDetail: (detail: string) => void;
  log: (message: string, options?: { detail?: string; level?: BatchProgressLogLevel }) => void;
};

const initialState: BatchProgressDialogState = {
  open: false,
  title: '',
  description: '',
  actionLabel: '',
  status: 'idle',
  items: [],
  total: 0,
  completed: 0,
  currentItemLabel: undefined,
  failureCount: 0,
  logs: [],
};

function toError(error: unknown): Error {
  if (error instanceof Error) {
    return error;
  }
  return new Error('Request failed');
}

export function useBatchProgressOperation() {
  const [dialogState, setDialogState] = useState<BatchProgressDialogState>(initialState);

  function appendLog(entry: Omit<BatchProgressLogEntry, 'id' | 'timestamp'>) {
    setDialogState((current) => ({
      ...current,
      logs: [
        ...current.logs,
        {
          id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
          timestamp: new Date().toISOString(),
          ...entry,
        },
      ].slice(-250),
    }));
  }

  async function runBatchOperation<T>({
    title,
    description,
    actionLabel,
    items,
    getKey,
    getLabel,
    execute,
    onComplete,
    getSuccessToast,
    getErrorToast,
  }: RunBatchProgressOptions<T>): Promise<BatchProgressResult<T>> {
    const progressItems = items.map((item) => ({
      key: getKey(item),
      label: getLabel(item),
      status: 'pending' as const,
    }));

    setDialogState({
      open: true,
      title,
      description,
      actionLabel,
      status: 'running',
      items: progressItems,
      total: progressItems.length,
      completed: 0,
      currentItemLabel: progressItems[0]?.label,
      failureCount: 0,
      logs: [],
    });

    const successfulItems: T[] = [];
    const failedItems: Array<{ item: T; error: Error }> = [];

    for (const [index, item] of items.entries()) {
      const itemKey = progressItems[index]?.key;
      const itemLabel = progressItems[index]?.label;

      const context: BatchProgressContext = {
        setDetail: (detail) => {
          setDialogState((current) => ({
            ...current,
            items: current.items.map((entry, entryIndex) =>
              entryIndex === index ? { ...entry, detail } : entry,
            ),
          }));
        },
        log: (message, options) => {
          if (options?.detail) {
            context.setDetail(options.detail);
          }
          appendLog({
            itemKey,
            itemLabel,
            level: options?.level ?? 'info',
            message,
          });
        },
      };

      setDialogState((current) => ({
        ...current,
        currentItemLabel: itemLabel,
        items: current.items.map((entry, entryIndex) =>
          entryIndex === index
            ? { ...entry, status: 'running', detail: undefined }
            : entry,
        ),
      }));

      try {
        const result = await execute(item, context);
        successfulItems.push(item);

        setDialogState((current) => ({
          ...current,
          completed: index + 1,
          items: current.items.map((entry, entryIndex) =>
            entryIndex === index
              ? { ...entry, status: 'success', detail: result?.detail }
              : entry,
          ),
        }));

        if (result?.detail) {
          appendLog({
            itemKey,
            itemLabel,
            level: 'success',
            message: result.detail,
          });
        }
      } catch (error) {
        const resolvedError = toError(error);
        failedItems.push({ item, error: resolvedError });
        appendLog({
          itemKey,
          itemLabel,
          level: 'error',
          message: resolvedError.message,
        });

        setDialogState((current) => ({
          ...current,
          completed: index + 1,
          failureCount: failedItems.length,
          items: current.items.map((entry, entryIndex) =>
            entryIndex === index
              ? { ...entry, status: 'error', detail: resolvedError.message }
              : entry,
          ),
        }));
      }
    }

    const result: BatchProgressResult<T> = {
      total: items.length,
      successCount: successfulItems.length,
      failureCount: failedItems.length,
      successfulItems,
      failedItems,
    };

    setDialogState((current) => ({
      ...current,
      status: failedItems.length > 0 ? 'error' : 'success',
      currentItemLabel: undefined,
      failureCount: failedItems.length,
    }));

    await onComplete?.(result);

    if (failedItems.length === 0) {
      const successToast = getSuccessToast?.(result);
      if (successToast) {
        toast.success(successToast);
      }
    } else {
      const errorToast = getErrorToast?.(result);
      if (errorToast) {
        toast.error(errorToast);
      }
    }

    return result;
  }

  function closeDialog() {
    setDialogState((current) =>
      current.status === 'running'
        ? current
        : { ...current, open: false },
    );
  }

  return {
    dialogState,
    runBatchOperation,
    closeDialog,
    isRunning: dialogState.status === 'running',
  };
}
