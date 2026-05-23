// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type IdentityProvider = {
  id: string;
  name: string;
  providerType: 'entra_id' | 'google' | 'generic_oidc' | 'saml_2_0';
  enabled: boolean;
  clientId: string;
  tenantId?: string;
  domain?: string;
  issuerUrl?: string;
  metadataUrl?: string;
  entityId?: string;
  scopes: string;
  autoProvision: boolean;
  defaultRoleId?: string;
  groupMappingEnabled: boolean;
  groupMappings: GroupMapping[];
  autoImportGroups: boolean;
  createdAt: string;
  updatedAt: string;
};

export type ProviderGroupInfo = {
  id: string;
  name: string;
  description?: string;
};

export type GroupMapping = {
  providerGroup: string;
  mcharborGroupId: string;
};

export type CreateProviderInput = {
  name: string;
  providerType: 'entra_id' | 'google' | 'generic_oidc' | 'saml_2_0';
  clientId: string;
  clientSecret: string;
  tenantId?: string;
  domain?: string;
  issuerUrl?: string;
  metadataUrl?: string;
  entityId?: string;
  scopes?: string;
  autoProvision?: boolean;
  defaultRoleId?: string;
  groupMappingEnabled?: boolean;
  groupMappings?: GroupMapping[];
  autoImportGroups?: boolean;
};

export type UpdateProviderInput = {
  name?: string;
  enabled?: boolean;
  clientId?: string;
  clientSecret?: string;
  tenantId?: string;
  domain?: string;
  issuerUrl?: string;
  metadataUrl?: string;
  entityId?: string;
  scopes?: string;
  autoProvision?: boolean;
  defaultRoleId?: string;
  groupMappingEnabled?: boolean;
  groupMappings?: GroupMapping[];
  autoImportGroups?: boolean;
};

export function useTestProvider() {
  const { t } = useTranslation('security');

  return useMutation({
    mutationFn: (id: string) =>
      api.post(`/identity-providers/${id}/test`).then(assertSuccess),
    meta: { success: () => t('toast.testSuccess') },
  });
}

export function useFetchProviderGroups() {
  return useMutation({
    mutationFn: (id: string) =>
      api.get<ProviderGroupInfo[]>(`/identity-providers/${id}/groups`).then((r) => r.data ?? []),
  });
}

export function useIdentityProviders() {
  return useQuery({
    queryKey: ['identity-providers'],
    queryFn: () =>
      api.get<IdentityProvider[]>('/identity-providers').then((r) => r.data ?? []),
  });
}

export function useCreateProvider() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');

  return useMutation({
    mutationFn: (body: CreateProviderInput) =>
      api.post<IdentityProvider>('/identity-providers', body).then(assertSuccess),
    meta: { success: () => t('toast.providerCreated') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['identity-providers'] });
    },
  });
}

export function useUpdateProvider() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');

  return useMutation({
    mutationFn: ({ id, ...body }: { id: string } & UpdateProviderInput) =>
      api.put<IdentityProvider>(`/identity-providers/${id}`, body).then(assertSuccess),
    meta: { success: () => t('toast.providerUpdated') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['identity-providers'] });
    },
  });
}

export function useDeleteProvider() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');

  return useMutation({
    mutationFn: (id: string) =>
      api.del(`/identity-providers/${id}`).then(assertSuccess),
    meta: { success: () => t('toast.providerDeleted') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['identity-providers'] });
    },
  });
}
