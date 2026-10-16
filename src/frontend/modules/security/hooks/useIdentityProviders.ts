// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type IdentityProvider = {
  id: string;
  name: string;
  /** Plugin-owned provider type key (e.g. 'entra_id', 'saml_2_0').
   * Deliberately a plain string, not a union: every type comes from
   * an installed plugin, so the host cannot enumerate them. */
  providerType: string;
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
  defaultRoleName?: string;
  groupMappingEnabled: boolean;
  groupMappings: GroupMapping[];
  autoImportGroups: boolean;
  /** Plugin-owned field map from identity_providers.config, with
   * every encrypted value redacted out by the API. Seeds the edit
   * dialog's ConfigForm; a missing secret means 'unchanged'. */
  config?: Record<string, string>;
  createdAt: string;
  updatedAt: string;
};

// IdentityProviderTestStep / IdentityProviderTestResult mirror
// StorageLocationTestStep / StorageLocationTestResult and the AI
// provider equivalents, so IdentityProviderTestDialog renders the
// same modal the storage tab already uses. The backend shape lives in
// src/backend/modules/identity/test.go.
export type IdentityProviderTestStep = {
  name: string;
  status: 'pass' | 'warn' | 'fail' | 'skip';
  detail?: string;
  latency?: string;
};

export type IdentityProviderTestResult = {
  providerId: string;
  providerName: string;
  providerType: string;
  overallStatus: 'pass' | 'warn' | 'fail' | 'skip';
  startedAt: string;
  completedAt: string;
  duration: string;
  steps: IdentityProviderTestStep[];
  groups?: string[];
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
  providerType: string;
  config?: Record<string, string>;
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

export type UpdateProviderInput = {
  name?: string;
  enabled?: boolean;
  config?: Record<string, string>;
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
// (kept for backwards compat — UpdateProviderInput still uses
// undefined-as-omitted semantics on the wire, with the host's
// Service.Update picking between typed fields and Config.)

// useTestProvider returns the full per-step report. No success toast:
// a test that passes four steps and warns on the fifth is not a
// one-line outcome, so IdentityProviderTestDialog is the feedback
// surface. A non-2xx (404 / 500) still surfaces through the shared
// error handling in assertSuccess.
export function useTestProvider() {
  return useMutation({
    mutationFn: (id: string) =>
      api.post<IdentityProviderTestResult>(`/identity-providers/${id}/test`).then(assertSuccess),
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
    queryKey: ['identityProviders'],
    queryFn: () => api.get<IdentityProvider[]>('/identity-providers').then((r) => r.data ?? []),
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
      void queryClient.invalidateQueries({ queryKey: ['identityProviders'] });
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
      void queryClient.invalidateQueries({ queryKey: ['identityProviders'] });
    },
  });
}

export function useDeleteProvider() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');

  return useMutation({
    mutationFn: (id: string) => api.del(`/identity-providers/${id}`).then(assertSuccess),
    meta: { success: () => t('toast.providerDeleted') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['identityProviders'] });
    },
  });
}
