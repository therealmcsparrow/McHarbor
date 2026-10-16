// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

type SettingRow = {
  id: string;
  key: string;
  value: string;
  category: string;
  updatedAt: string;
};

export const MFA_REQUIREMENT_KEY = 'security.mfa_required';

export const MFA_REQUIREMENTS = ['off', 'admins', 'all'] as const;

export type MFARequirement = (typeof MFA_REQUIREMENTS)[number];

export function parseMFARequirement(raw: string | undefined | null): MFARequirement {
  const value = (raw ?? '').trim().toLowerCase();
  return (MFA_REQUIREMENTS as readonly string[]).includes(value) ? (value as MFARequirement) : 'off';
}

export function useMFARequirement() {
  return useQuery({
    queryKey: ['settings', MFA_REQUIREMENT_KEY],
    queryFn: async () => {
      const res = await api.get<SettingRow>(`/settings/${MFA_REQUIREMENT_KEY}`);
      if (!res.success || !res.data) return 'off' as MFARequirement;
      return parseMFARequirement(res.data.value);
    },
  });
}

export function useSaveMFARequirement() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (value: MFARequirement) =>
      api
        .put(`/settings/${MFA_REQUIREMENT_KEY}`, {
          key: MFA_REQUIREMENT_KEY,
          value,
          category: 'security',
        })
        .then(assertSuccess),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['settings', MFA_REQUIREMENT_KEY] });
    },
  });
}
