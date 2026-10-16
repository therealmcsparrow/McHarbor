// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type HostLogSource = 'activity' | 'system' | 'kernel' | 'auth' | 'docker' | 'libvirt' | 'agent' | 'qemu';

export interface HostLogTarget {
  id: string;
  label?: string;
}

export interface HostLogResult {
  source: string;
  tail: number;
  lines: string[];
  notices: string[];
  availableFiles: string[];
  /** The guests a qemu read can name; empty for every other source and on older agents. */
  targets?: HostLogTarget[];
  fallbackFile?: string;
  fetchedAt: string;
}

export interface LifecycleEvent {
  id: string;
  environmentId?: string;
  subjectType: string;
  subjectId: string;
  subjectName?: string;
  eventType: string;
  action: string;
  state?: string;
  severity: string;
  metadata?: string;
  source: string;
  timestamp: string;
}

export interface LifecycleListResult {
  items: LifecycleEvent[];
  total: number;
  page: number;
  perPage: number;
}
