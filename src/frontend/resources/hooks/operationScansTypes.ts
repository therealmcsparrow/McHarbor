// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { BatchProgressLogLevel } from '@resources/hooks/useBatchProgressOperation';

export type OperationScan = {
  id: string;
  imageRef: string;
  scanner: string;
  status: 'pending' | 'running' | 'completed' | 'failed';
  totalVulns: number;
  criticalCount: number;
  highCount: number;
  mediumCount: number;
  lowCount: number;
};

export type ScanLogger = (message: string, options?: { level?: BatchProgressLogLevel }) => void;

export type ScanSummary = {
  image: string;
  scanner: string;
  status: 'completed' | 'failed';
  totalVulns: number;
  criticalCount: number;
  highCount: number;
  mediumCount: number;
  lowCount: number;
};

export type ScanPlan =
  | { status: 'ready'; scanners: string[] }
  | { status: 'disabled' }
  | { status: 'none' }
  | { status: 'unavailable' };

export type OperationScanBatchResult = {
  attempted: number;
  completed: number;
  failed: number;
  skipped: boolean;
};
