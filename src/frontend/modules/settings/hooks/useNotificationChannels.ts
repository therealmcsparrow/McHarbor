// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type ChannelType = string;

export type CommunicationChannel = {
  id: string;
  name: string;
  channelType: ChannelType;
  method?: string;
  isDefault: boolean;
  enabled: boolean;
  serverUrl?: string;
  topic?: string;
  chatId?: string;
  phoneNumberId?: string;
  recipientPhone?: string;
  senderNumber?: string;
  recipients?: string;
  username?: string;
  priority?: string;
  config?: Record<string, string>;
  createdAt: string;
  updatedAt: string;
};

export type CreateChannelInput = {
  name: string;
  channelType: ChannelType;
  method?: string;
  isDefault?: boolean;
  webhookUrl?: string;
  serverUrl?: string;
  token?: string;
  topic?: string;
  chatId?: string;
  phoneNumberId?: string;
  recipientPhone?: string;
  senderNumber?: string;
  recipients?: string;
  username?: string;
  password?: string;
  priority?: string;
  config?: Record<string, string>;
};

export type UpdateChannelInput = {
  name?: string;
  method?: string;
  isDefault?: boolean;
  enabled?: boolean;
  webhookUrl?: string;
  serverUrl?: string;
  token?: string;
  topic?: string;
  chatId?: string;
  phoneNumberId?: string;
  recipientPhone?: string;
  senderNumber?: string;
  recipients?: string;
  username?: string;
  password?: string;
  priority?: string;
  config?: Record<string, string>;
};

export function useNotificationChannels() {
  return useQuery({
    queryKey: ['communication-channels'],
    queryFn: () => api.get<CommunicationChannel[]>('/communication-channels').then((r) => r.data ?? []),
  });
}

export function useCreateChannel() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');

  return useMutation({
    mutationFn: (body: CreateChannelInput) =>
      api.post<CommunicationChannel>('/communication-channels', body).then(assertSuccess),
    meta: { success: () => t('toast.channelCreated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['communication-channels'] });
    },
  });
}

export function useUpdateChannel() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');

  return useMutation({
    mutationFn: ({ id, ...body }: { id: string } & UpdateChannelInput) =>
      api.put<CommunicationChannel>(`/communication-channels/${id}`, body).then(assertSuccess),
    meta: { success: () => t('toast.channelUpdated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['communication-channels'] });
    },
  });
}

export function useDeleteChannel() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');

  return useMutation({
    mutationFn: (id: string) => api.del(`/communication-channels/${id}`).then(assertSuccess),
    meta: { success: () => t('toast.channelDeleted') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['communication-channels'] });
    },
  });
}

export function useSetDefaultChannel() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');

  return useMutation({
    mutationFn: (id: string) => api.post(`/communication-channels/${id}/default`).then(assertSuccess),
    meta: { success: () => t('toast.channelDefaultSet') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['communication-channels'] });
    },
  });
}

// ChannelTestStep / ChannelTestResult mirror the storage, identity,
// and AI provider equivalents so TestChannelDialog renders the same
// per-step modal. Backend shape: src/backend/modules/communications/
// test.go — phases load, config, plugin, send.
export type ChannelTestStep = {
  name: string;
  status: 'pass' | 'warn' | 'fail' | 'skip';
  detail?: string;
  latency?: string;
};

export type ChannelTestResult = {
  channelId: string;
  channelName: string;
  channelType: string;
  overallStatus: 'pass' | 'warn' | 'fail' | 'skip';
  startedAt: string;
  completedAt: string;
  duration: string;
  steps: ChannelTestStep[];
};

// useTestChannel returns the full per-step report. No success toast:
// a send that reaches the provider but warns on delivery is not a
// one-line outcome, so TestChannelDialog is the feedback surface.
export function useTestChannel() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => api.post<ChannelTestResult>(`/communication-channels/${id}/test`).then(assertSuccess),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['communication-channels'] });
    },
  });
}
