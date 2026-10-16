// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type PaginatedData } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type LinkState = 'pending' | 'open' | 'remote_closed' | 'resolved' | 'orphaned';
export type RemoteState = '' | 'open' | 'acknowledged' | 'resolved' | 'closed' | 'cancelled';

export type TicketLink = {
  id: string;
  channelId: string;
  channelType: string;
  channelName?: string;
  dedupKey: string;
  alertId: string;
  alertName?: string;
  environmentId: string;
  subjectId: string;
  externalId: string;
  externalNumber: string;
  externalUrl: string;
  localState: LinkState;
  remoteState: RemoteState;
  remoteAssignee: string;
  lastError: string;
  openedAt: string;
  resolvedAt: string;
  lastSyncedAt: string;
  updatedAt: string;
  parked: boolean;
};

export type ItsmChannel = {
  id: string;
  name: string;
  channelType: string;
  webhookSupported: boolean;
  pollSupported: boolean;
  tokenSet: boolean;
  secretSet: boolean;
  baseUrl?: string;
};

export type RotateResult = { url: string; secret: string };

export function useTicketLinks(state: LinkState | '') {
  return useQuery({
    queryKey: ['itsm-links', state],
    queryFn: () =>
      api
        .get<PaginatedData<TicketLink>>('/itsm/links', { per_page: '200', ...(state ? { state } : {}) })
        .then((r) => r.data?.items ?? []),
    refetchInterval: 20_000,
  });
}

export function useItsmChannels(enabled = true) {
  return useQuery({
    queryKey: ['itsm-channels'],
    enabled,
    queryFn: () => api.get<ItsmChannel[]>('/itsm/channels').then(assertSuccess),
  });
}

function useLinkAction(path: 'resync' | 'retry') {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.post(`/itsm/links/${encodeURIComponent(id)}/${path}`).then(assertSuccess),
    onSettled: () => void client.invalidateQueries({ queryKey: ['itsm-links'] }),
  });
}

export const useResyncLink = () => useLinkAction('resync');
export const useRetryLink = () => useLinkAction('retry');

export function useRotateWebhookToken() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (channelId: string) =>
      api.post<RotateResult>(`/itsm/channels/${encodeURIComponent(channelId)}/webhook-token`).then(assertSuccess),
    onSuccess: () => void client.invalidateQueries({ queryKey: ['itsm-channels'] }),
  });
}

export function useSetWebhookSecret() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ channelId, secret }: { channelId: string; secret: string }) =>
      api.post(`/itsm/channels/${encodeURIComponent(channelId)}/webhook-secret`, { secret }).then(assertSuccess),
    onSuccess: () => void client.invalidateQueries({ queryKey: ['itsm-channels'] }),
  });
}
