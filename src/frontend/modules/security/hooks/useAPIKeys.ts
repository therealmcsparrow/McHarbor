// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type APIKeyRole = {
  id: string;
  name: string;
};

/**
 * `deleted` is a tombstone: the credential is destroyed but a read-only
 * record of what it could do is retained until retention expires.
 */
export type APIKeyStatus = 'active' | 'expired' | 'revoked' | 'deleted';

export type APIKey = {
  id: string;
  userId: string;
  username: string;
  name: string;
  keyPrefix: string;
  scopes: string[];
  roles: APIKeyRole[];
  environmentIds: string[];
  environmentNames: string[];
  expiresAt: string | null;
  lastUsedAt: string | null;
  isActive: boolean;
  createdAt: string;
  updatedAt: string;
  status: APIKeyStatus;
  deletedAt: string | null;
  ageDays?: number;
  rotationDue?: boolean;
  expiringSoon?: boolean;
};

export type APIKeyRoleDetail = {
  id: string;
  name: string;
  description: string;
  permissions: string[];
};

export type APIKeyEnvironmentDetail = {
  id: string;
  name: string;
};

/**
 * The four permission views the detail page must keep apart:
 *
 *   roleDetails            what each assigned role would grant
 *   scopes                 permissions granted directly on the key
 *   configuredPermissions  the union of those two — what the key asks for
 *   effectivePermissions   that union once the owner's rights are applied
 *
 * `effectiveSnapshot` is true for a deleted key, where the effective set is
 * what was captured at deletion rather than a live calculation.
 */
export type APIKeyDetail = APIKey & {
  roleDetails: APIKeyRoleDetail[];
  environments: APIKeyEnvironmentDetail[];
  configuredPermissions: string[];
  effectivePermissions: string[];
  effectiveSnapshot: boolean;
};

export type CreateAPIKeyResult = APIKey & {
  key: string;
};

export type APIKeyGrantInput = {
  roleIds: string[];
  scopes: string[];
  environmentIds: string[];
};

export type APIKeyGrantOptions = {
  roles: Array<{
    id: string;
    name: string;
    description: string;
    permissions: string[];
    grantable: boolean;
  }>;
  environments: Array<{ id: string; name: string }>;
  permissions: string[];
  wildcard: boolean;
};

export function useAPIKeys() {
  return useQuery({
    queryKey: ['api-keys'],
    queryFn: () => api.get<APIKey[]>('/api-keys').then((r) => assertSuccess(r) ?? []),
  });
}

/**
 * One key in full, live or deleted. The backend falls back to the tombstone
 * for a deleted key, so this keeps working until retention removes it.
 */
export function useAPIKeyDetail(id: string | undefined) {
  return useQuery({
    queryKey: ['api-keys', 'detail', id],
    queryFn: () => api.get<APIKeyDetail>(`/api-keys/${id}`).then(assertSuccess),
    enabled: !!id,
  });
}

export function useAPIKeyGrantOptions(enabled: boolean) {
  return useQuery({
    queryKey: ['api-keys', 'options'],
    queryFn: () => api.get<APIKeyGrantOptions>('/api-keys/options').then(assertSuccess),
    enabled,
    staleTime: 60_000,
  });
}

export function useCreateAPIKey() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');

  return useMutation({
    mutationFn: (body: { name: string; expiresAt?: string } & APIKeyGrantInput) =>
      api.post<CreateAPIKeyResult>('/api-keys', body).then(assertSuccess),
    meta: { success: () => t('toast.keyCreated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['api-keys'] });
    },
  });
}

export function useUpdateAPIKey() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');

  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: { name?: string } & APIKeyGrantInput }) =>
      api.patch<APIKey>(`/api-keys/${id}`, body).then(assertSuccess),
    meta: { success: () => t('toast.keyUpdated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['api-keys'] });
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
      void queryClient.invalidateQueries({ queryKey: ['api-keys'] });
    },
  });
}

export function useRotateAPIKey() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');

  return useMutation({
    mutationFn: (id: string) => api.post<CreateAPIKeyResult>(`/api-keys/${id}/rotate`).then(assertSuccess),
    meta: { success: () => t('governance.apiKeys.rotated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['api-keys'] });
    },
  });
}

/** Same route as revoke; the backend deletes when the key is already revoked. */
export function useDeleteAPIKey() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');

  return useMutation({
    mutationFn: (id: string) => api.del(`/api-keys/${id}`).then(assertSuccess),
    meta: { success: () => t('toast.keyDeleted') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['api-keys'] });
    },
  });
}
