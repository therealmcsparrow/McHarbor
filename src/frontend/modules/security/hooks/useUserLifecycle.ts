// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type LifecycleUser = {
  id: string;
  username: string;
  displayName: string | null;
  authProvider: string;
  disabled: boolean;
  lockedUntil: string | null;
  locked: boolean;
  mfaEnabled: boolean;
  lastLogin: string | null;
  inactive: boolean;
  lastElevatedAt: string | null;
  recentlyElevated: boolean;
  breakGlass: boolean;
  createdAt: string;
};

export type LifecycleCounts = {
  total: number;
  disabled: number;
  locked: number;
  noMfa: number;
  inactive: number;
  recentlyElevated: number;
};

export type LifecycleReport = {
  inactiveDays: number;
  elevatedDays: number;
  counts: LifecycleCounts;
  users: LifecycleUser[];
};

export type LifecycleFilter = 'all' | 'disabled' | 'locked' | 'noMfa' | 'inactive' | 'recentlyElevated';

export const LIFECYCLE_FILTERS: LifecycleFilter[] = [
  'all',
  'disabled',
  'locked',
  'noMfa',
  'inactive',
  'recentlyElevated',
];

export function matchesLifecycleFilter(user: LifecycleUser, filter: LifecycleFilter): boolean {
  switch (filter) {
    case 'disabled':
      return user.disabled;
    case 'locked':
      return user.locked;
    case 'noMfa':
      return !user.mfaEnabled;
    case 'inactive':
      return user.inactive;
    case 'recentlyElevated':
      return user.recentlyElevated;
    default:
      return true;
  }
}

export type BulkAction = 'disable' | 'enable' | 'unlock' | 'delete';

export type BulkItem = {
  userId: string;
  username: string;
  outcome: 'would_apply' | 'applied' | 'skipped' | 'failed';
  reason?: string;
};

export type BulkResult = {
  action: BulkAction;
  dryRun: boolean;
  items: BulkItem[];
  summary: { total: number; applied: number; skipped: number; failed: number };
};

export type BulkInput = {
  action: BulkAction;
  userIds: string[];
  dryRun: boolean;
  reason: string;
};

export function useUserLifecycle(inactiveDays: number, elevatedDays: number) {
  return useQuery({
    queryKey: ['users', 'lifecycle', inactiveDays, elevatedDays],
    queryFn: () =>
      api
        .get<LifecycleReport>('/users/lifecycle', {
          inactiveDays: String(inactiveDays),
          elevatedDays: String(elevatedDays),
        })
        .then(assertSuccess),
  });
}

export function useBulkUserAction() {
  const client = useQueryClient();
  const { t } = useTranslation('security');
  return useMutation({
    meta: {
      success: (data: unknown) => {
        const result = data as BulkResult;
        return result.dryRun ? '' : t('governance.lifecycle.bulkDone', result.summary);
      },
    },
    mutationFn: (input: BulkInput) =>
      api
        .post<BulkResult>(input.action === 'delete' ? '/users/bulk/delete' : '/users/bulk/account', input)
        .then(assertSuccess),
    onSuccess: (result) => {
      if (!result.dryRun) void client.invalidateQueries({ queryKey: ['users'] });
    },
  });
}
