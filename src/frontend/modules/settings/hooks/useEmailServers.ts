// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type EmailServer = {
  id: string;
  name: string;
  serverType: 'smtp' | 'exchange' | 'gmail';
  isDefault: boolean;
  enabled: boolean;
  host?: string;
  port?: number;
  encryption?: string;
  authMethod?: string;
  username?: string;
  clientId?: string;
  tenantId?: string;
  fromAddress: string;
  fromName?: string;
  createdAt: string;
  updatedAt: string;
};

export type CreateEmailServerInput = {
  name: string;
  serverType: 'smtp' | 'exchange' | 'gmail';
  isDefault?: boolean;
  host?: string;
  port?: number;
  encryption?: string;
  authMethod?: string;
  username?: string;
  password?: string;
  clientId?: string;
  clientSecret?: string;
  tenantId?: string;
  fromAddress: string;
  fromName?: string;
};

export type UpdateEmailServerInput = {
  name?: string;
  isDefault?: boolean;
  enabled?: boolean;
  host?: string;
  port?: number;
  encryption?: string;
  authMethod?: string;
  username?: string;
  password?: string;
  clientId?: string;
  clientSecret?: string;
  tenantId?: string;
  fromAddress?: string;
  fromName?: string;
};

export function useEmailServers() {
  return useQuery({
    queryKey: ['email-servers'],
    queryFn: () =>
      api.get<EmailServer[]>('/email-servers').then((r) => r.data ?? []),
  });
}

export function useCreateEmailServer() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');

  return useMutation({
    mutationFn: (body: CreateEmailServerInput) =>
      api.post<EmailServer>('/email-servers', body).then(assertSuccess),
    meta: { success: () => t('toast.emailServerCreated') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['email-servers'] });
    },
  });
}

export function useUpdateEmailServer() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');

  return useMutation({
    mutationFn: ({ id, ...body }: { id: string } & UpdateEmailServerInput) =>
      api.put<EmailServer>(`/email-servers/${id}`, body).then(assertSuccess),
    meta: { success: () => t('toast.emailServerUpdated') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['email-servers'] });
    },
  });
}

export function useDeleteEmailServer() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');

  return useMutation({
    mutationFn: (id: string) =>
      api.del(`/email-servers/${id}`).then(assertSuccess),
    meta: { success: () => t('toast.emailServerDeleted') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['email-servers'] });
    },
  });
}

export function useSetDefaultEmailServer() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');

  return useMutation({
    mutationFn: (id: string) =>
      api.post(`/email-servers/${id}/default`).then(assertSuccess),
    meta: { success: () => t('toast.emailServerDefaultSet') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['email-servers'] });
    },
  });
}

export function useTestEmailServer() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');

  return useMutation({
    mutationFn: ({ id, to }: { id: string; to: string }) =>
      api.post(`/email-servers/${id}/test`, { to }).then(assertSuccess),
    meta: { success: () => t('toast.emailServerTestSent') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['email-servers'] });
    },
  });
}
