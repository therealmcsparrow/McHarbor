// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { PluginInstallResult } from '../types/plugins';

export interface PluginStreamEvent {
  phase: string;
  step: number;
  total: number;
  message?: string;
  status: 'progress' | 'done' | 'error';
  error_code?: string;
  error_text?: string;
  result?: PluginInstallResult;
}

export interface PluginStreamLogEntry {
  id: string;
  message: string;
  phase?: string;
}

export interface PluginStreamState {
  running: boolean;
  progress: PluginStreamEvent | null;
  logs: PluginStreamLogEntry[];

  result: PluginInstallResult | null;

  errorCode: string | null;
}
