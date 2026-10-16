// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { refusalOf } from './refusal';

export type DesiredStateSource = {
  id: string;
  name: string;
  repoId: string;
  branch: string;
  path: string;
  envId: string;
  enabled: boolean;
  lastCommit: string;
  lastSyncedAt?: string;
  lastError?: string;
  rowCount: number;
  createdAt: string;
};

export type SourceInput = {
  name: string;
  repoId: string;
  branch: string;
  path: string;
  envId: string;
  enabled: boolean;
};

export type SyncResult = {
  commit: string;
  changed: boolean;
  created: number;
  updated: number;
  deleted: number;
  rowCount: number;
};

export type PullRequestResult = {
  branch: string;
  pullRequestUrl?: string;
  provider?: string;
  providerError?: string;
  changed: string[];
  skipped: string[];
};

const KEY = ['reconciler-sources'] as const;

export function useSources() {
  return useQuery({
    queryKey: KEY,
    queryFn: () => api.get<DesiredStateSource[]>('/reconciler/sources').then(assertSuccess),
    refetchInterval: 30_000,
  });
}

function useInvalidate() {
  const client = useQueryClient();
  return () => {
    void client.invalidateQueries({ queryKey: KEY });
    void client.invalidateQueries({ queryKey: ['reconciler'] });
  };
}

export function useCreateSource() {
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (input: SourceInput) => api.post<DesiredStateSource>('/reconciler/sources', input).then(refusalOf),
    onSuccess: invalidate,
  });
}

export function useDeleteSource() {
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (id: string) => api.del(`/reconciler/sources/${encodeURIComponent(id)}`).then(refusalOf),
    onSuccess: invalidate,
  });
}

export function useSyncSource() {
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (id: string) =>
      api.post<SyncResult>(`/reconciler/sources/${encodeURIComponent(id)}/sync`).then(refusalOf),
    onSettled: invalidate,
  });
}

export function usePullRequest() {
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (desiredStateId: string) =>
      api.post<PullRequestResult>(`/reconciler/${encodeURIComponent(desiredStateId)}/pull-request`).then(refusalOf),
    onSuccess: invalidate,
  });
}
