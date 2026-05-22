// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type APIKey = {
  id: string;
  userId: string;
  username: string;
  name: string;
  keyPrefix: string;
  scopes: string[];
  expiresAt: string | null;
  lastUsedAt: string | null;
  isActive: boolean;
  createdAt: string;
  updatedAt: string;
};

export type CreateAPIKeyResult = APIKey & {
  key: string;
};

export function useAPIKeys() {
  return useQuery({
    queryKey: ['api-keys'],
    queryFn: () => api.get<APIKey[]>('/api-keys').then((r) => r.data ?? []),
  });
}

export function useCreateAPIKey() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');

  return useMutation({
    mutationFn: (body: { name: string; scopes: string[]; expiresAt?: string }) =>
      api.post<CreateAPIKeyResult>('/api-keys', body).then(assertSuccess),
    meta: { success: () => t('toast.keyCreated') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['api-keys'] });
    },
  });
}

export function useRevokeAPIKey() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');

  return useMutation({
    mutationFn: (id: string) => api.del(`/api-keys/${id}`).then(assertSuccess),
    meta: { success: () => t('toast.keyRevoked') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['api-keys'] });
    },
  });
}
