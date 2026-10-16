// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess } from '@resources/utils/api-mutation';
import { stackLinkKeys } from './stackLinkKeys';

export type ContainerStackLink = {
  id: string;
  environmentId: string;
  containerId: string;
  stackName: string;
  serviceName?: string;
  createdAt: string;
  updatedAt: string;
};

type LinkContainerInput = {
  containerId: string;
  stackName: string;
  serviceName?: string;
};

export function useContainerStackLink(containerId?: string) {
  const envId = useEnvironmentStore((state) => state.currentId);

  return useQuery({
    queryKey: stackLinkKeys.link(envId, containerId),
    queryFn: () =>
      api
        .get<ContainerStackLink | null>('/stacks/links', {
          containerId: containerId ?? '',
          ...(envId ? { env: envId } : {}),
        })
        .then((response) => response.data ?? null),
    enabled: !!containerId,
  });
}

export function useLinkContainerToStack() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((state) => state.currentId);
  const { t } = useTranslation('stacks');

  return useMutation({
    mutationFn: (input: LinkContainerInput) =>
      api.post<ContainerStackLink>(`/stacks/links${envId ? `?env=${envId}` : ''}`, input).then(assertSuccess),
    meta: { success: t('link.success') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: stackLinkKeys.linkAll });
      void queryClient.invalidateQueries({ queryKey: ['containers'] });
      void queryClient.invalidateQueries({ queryKey: ['container'] });
      void queryClient.invalidateQueries({ queryKey: ['stacks'] });
      void queryClient.invalidateQueries({ queryKey: ['stack'] });
      void queryClient.invalidateQueries({ queryKey: ['stack-containers'] });
    },
  });
}

export function useUnlinkContainerFromStack() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((state) => state.currentId);
  const { t } = useTranslation('stacks');

  return useMutation({
    mutationFn: (containerId: string) =>
      api.del(`/stacks/links/${containerId}`, envId ? { env: envId } : {}).then(assertSuccess),
    meta: { success: t('link.unlinked') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: stackLinkKeys.linkAll });
      void queryClient.invalidateQueries({ queryKey: ['containers'] });
      void queryClient.invalidateQueries({ queryKey: ['container'] });
      void queryClient.invalidateQueries({ queryKey: ['stacks'] });
      void queryClient.invalidateQueries({ queryKey: ['stack'] });
      void queryClient.invalidateQueries({ queryKey: ['stack-containers'] });
    },
  });
}
