// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

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

export type RunBatchProgressOptions<T> = {
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
