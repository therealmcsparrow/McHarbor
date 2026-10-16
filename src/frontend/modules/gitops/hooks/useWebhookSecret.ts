// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { WebhookSecretGenerated, WebhookSecretStatus } from '../types';

export function useWebhookSecretStatus(repoId: string) {
  return useQuery({
    queryKey: ['gitops-webhook-secret', repoId],
    queryFn: () => api.get<WebhookSecretStatus>(`/gitops/repos/${repoId}/webhook-secret`).then(assertSuccess),
  });
}

/** Rotating replaces the secret the forge holds, so the caller must show the returned value once. */
export function useGenerateWebhookSecret(repoId: string, t: (key: string) => string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.post<WebhookSecretGenerated>(`/gitops/repos/${repoId}/webhook-secret`).then(assertSuccess),
    meta: { success: t('webhookSecret.generated') },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['gitops-webhook-secret', repoId] }),
  });
}

export function useRemoveWebhookSecret(repoId: string, t: (key: string) => string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.del(`/gitops/repos/${repoId}/webhook-secret`).then(assertSuccess),
    meta: { success: t('webhookSecret.removed') },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['gitops-webhook-secret', repoId] }),
  });
}
