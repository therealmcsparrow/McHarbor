// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess } from '@resources/utils/api-mutation';

type AdoptPreviewRequest = {
  stackName?: string;
  containerId?: string;
};

type AdoptPreviewResponse = {
  name: string;
  compose: string;
};

type AdoptRequest = {
  name: string;
  compose: string;
  description?: string;
  containerId?: string;
};

export function useAdoptPreview() {
  const envId = useEnvironmentStore((state) => state.currentId);

  return useMutation({
    mutationFn: (req: AdoptPreviewRequest) =>
      api
        .post<AdoptPreviewResponse>(`/stacks/adopt/preview${envId ? `?env=${envId}` : ''}`, req)
        .then((response) => {
          assertSuccess(response);
          return response.data as AdoptPreviewResponse;
        }),
  });
}

export function useAdoptStack() {
  const { t } = useTranslation('stacks');
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((state) => state.currentId);

  return useMutation({
    mutationFn: (req: AdoptRequest) =>
      api
        .post(`/stacks/adopt${envId ? `?env=${envId}` : ''}`, req)
        .then(assertSuccess),
    meta: { success: t('takeOver.success') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['container-stack-link'] });
      queryClient.invalidateQueries({ queryKey: ['stacks'] });
      queryClient.invalidateQueries({ queryKey: ['containers'] });
      queryClient.invalidateQueries({ queryKey: ['stack-containers'] });
    },
  });
}
