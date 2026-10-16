// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { DisplayProfile, DisplayProfileDraft } from '../types-display';

const PROFILES_KEY = ['vms', 'display-profiles'] as const;

/** A profile change moves guest counts, and a reassign moves guests' choices. */
function useInvalidateProfiles() {
  const queryClient = useQueryClient();
  return () => {
    void queryClient.invalidateQueries({ queryKey: PROFILES_KEY });
    void queryClient.invalidateQueries({ queryKey: ['vms', 'list'] });
    void queryClient.invalidateQueries({ queryKey: ['vms', 'detail'] });
  };
}

export function useDisplayProfiles() {
  return useQuery({
    queryKey: PROFILES_KEY,
    queryFn: async () => assertSuccess(await api.get<DisplayProfile[]>('/vms/display-profiles')) ?? [],
  });
}

export function useSaveDisplayProfile() {
  const { t } = useTranslation('vm');
  const invalidate = useInvalidateProfiles();
  return useMutation({
    meta: {
      success: (_data: unknown, input: unknown) =>
        typeof input === 'object' && input !== null && 'id' in input && input.id
          ? t('msg.vm.display_profile_updated')
          : t('msg.vm.display_profile_created'),
    },
    mutationFn: async (input: { id?: string; body: DisplayProfileDraft }) =>
      assertSuccess(
        input.id
          ? await api.put<DisplayProfile>(`/vms/display-profiles/${input.id}`, input.body)
          : await api.post<DisplayProfile>('/vms/display-profiles', input.body),
      ),
    onSuccess: invalidate,
  });
}

export function useDuplicateDisplayProfile() {
  const { t } = useTranslation('vm');
  const invalidate = useInvalidateProfiles();
  return useMutation({
    meta: { success: t('msg.vm.display_profile_created') },
    mutationFn: async (id: string) =>
      assertSuccess(await api.post<DisplayProfile>(`/vms/display-profiles/${id}/duplicate`, {})),
    onSuccess: invalidate,
  });
}

export function useDeleteDisplayProfile() {
  const { t } = useTranslation('vm');
  const invalidate = useInvalidateProfiles();
  return useMutation({
    meta: { success: t('msg.vm.display_profile_deleted') },
    mutationFn: async (input: { id: string; reassign: boolean }) =>
      assertSuccess(await api.del(`/vms/display-profiles/${input.id}${input.reassign ? '?reassign=true' : ''}`)),
    onSuccess: invalidate,
  });
}
