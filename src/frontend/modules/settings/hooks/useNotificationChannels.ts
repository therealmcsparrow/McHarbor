// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type ChannelType =
  | 'slack'
  | 'discord'
  | 'teams'
  | 'gotify'
  | 'ntfy'
  | 'telegram'
  | 'signal'
  | 'whatsapp';

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
};

export function useNotificationChannels() {
  return useQuery({
    queryKey: ['communication-channels'],
    queryFn: () =>
      api.get<CommunicationChannel[]>('/communication-channels').then((r) => r.data ?? []),
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
      queryClient.invalidateQueries({ queryKey: ['communication-channels'] });
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
      queryClient.invalidateQueries({ queryKey: ['communication-channels'] });
    },
  });
}

export function useDeleteChannel() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');

  return useMutation({
    mutationFn: (id: string) =>
      api.del(`/communication-channels/${id}`).then(assertSuccess),
    meta: { success: () => t('toast.channelDeleted') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['communication-channels'] });
    },
  });
}

export function useSetDefaultChannel() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');

  return useMutation({
    mutationFn: (id: string) =>
      api.post(`/communication-channels/${id}/default`).then(assertSuccess),
    meta: { success: () => t('toast.channelDefaultSet') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['communication-channels'] });
    },
  });
}

export function useTestChannel() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');

  return useMutation({
    mutationFn: (id: string) =>
      api.post(`/communication-channels/${id}/test`).then(assertSuccess),
    meta: { success: () => t('toast.channelTestSent') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['communication-channels'] });
    },
  });
}
