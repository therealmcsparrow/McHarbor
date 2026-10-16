// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { GroupMapping } from './useIdentityProviders';

export type ProviderEndpointUrls = {
  oidcRedirectUri: string;
  loginUrl?: string;
  samlAcsUrl?: string;
  samlMetadataUrl?: string;
};

export type SamlEndpoint = { binding: string; location: string };

export type SamlCertificate = {
  use?: string;
  subject?: string;
  notAfter?: string;
  fingerprintSha256: string;
  pem: string;
};

export type MetadataImportResult = {
  source: 'url' | 'xml';
  metadata: {
    entityId: string;
    ssoServices: SamlEndpoint[];
    sloServices: SamlEndpoint[];
    nameIdFormats: string[];
    certificates: SamlCertificate[];
    wantAuthnRequestsSigned: boolean;
    validUntil?: string;
  };
  prefill: Record<string, string>;
};

export type MappingConfig = {
  groupMappingEnabled: boolean;
  groupMappings: GroupMapping[];
  autoImportGroups: boolean;
  autoProvision: boolean;
  defaultRoleId?: string;
};

export type RoleGrantView = { roleId: string; roleName: string; environmentId?: string; stackName?: string };
export type GroupView = { groupId?: string; groupName: string; new?: boolean };

export type MappingPreview = {
  sample: {
    groups: (GroupView & { from: string[]; roles: RoleGrantView[] })[];
    unmapped: string[];
    defaultRole?: RoleGrantView;
    roles: RoleGrantView[];
  };
  impact?: {
    evaluated: number;
    unchanged: number;
    users: {
      userId: string;
      username: string;
      gained: RoleGrantView[];
      lost: RoleGrantView[];
      joined: GroupView[];
      left: GroupView[];
    }[];
    unknown: { userId: string; username: string }[];
  };
};

export type MappingPreviewInput = {
  providerId?: string;
  providerType?: string;
  config: MappingConfig;
  sampleGroups: string[];
};

export function useProviderEndpointUrls(providerId?: string) {
  return useQuery({
    queryKey: ['identityProviders', 'endpointUrls', providerId ?? ''],
    queryFn: () =>
      api
        .get<ProviderEndpointUrls>('/identity-providers/endpoint-urls', providerId ? { providerId } : undefined)
        .then(assertSuccess),
  });
}

const METADATA_REASONS = ['invalid_url', 'fetch_failed', 'not_saml_metadata', 'no_idp_descriptor', 'input_missing'];

export function useMetadataImport() {
  const { t } = useTranslation('access');
  return useMutation({
    mutationFn: async (input: { url?: string; xml?: string }) => {
      const res = await api.post<MetadataImportResult | { reason?: string }>(
        '/identity-providers/metadata-import',
        input,
      );
      const reason = !res.success && res.data && 'reason' in res.data ? res.data.reason : undefined;
      if (reason && METADATA_REASONS.includes(reason)) {
        throw new Error(t(`metadata.reason.${reason}`));
      }
      return assertSuccess(res) as MetadataImportResult;
    },
  });
}

export function useMappingPreview() {
  return useMutation({
    mutationFn: (input: MappingPreviewInput) =>
      api.post<MappingPreview>('/identity-providers/mapping-preview', input).then(assertSuccess),
  });
}

export function parseGroupList(text: string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const part of text.split(/[\n,;]/)) {
    const itemName = part.trim();
    if (itemName !== '' && !seen.has(itemName)) {
      seen.add(itemName);
      out.push(itemName);
    }
  }
  return out;
}

export function schemaKeys(schema: unknown[] | undefined): Set<string> {
  const keys = new Set<string>();
  for (const field of schema ?? []) {
    if (!field || typeof field !== 'object') continue;
    const record = field as Record<string, unknown>;
    const key = record.key ?? record.Key;
    if (typeof key === 'string') keys.add(key);
  }
  return keys;
}

export function applicablePrefill(
  prefill: Record<string, string>,
  schema: unknown[] | undefined,
): Record<string, string> {
  const keys = schemaKeys(schema);
  return Object.fromEntries(Object.entries(prefill).filter(([key]) => keys.has(key)));
}
