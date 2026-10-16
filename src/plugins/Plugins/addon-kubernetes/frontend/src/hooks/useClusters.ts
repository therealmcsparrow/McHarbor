// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

/**
 * A cluster connection, as the API returns it.
 *
 * There is deliberately no `kubeconfig`, `bearerToken` or `caCert` here: those
 * are write-only on this API. The two `has*` booleans are what let the edit
 * form say "kubeconfig on file" rather than render an empty field that looks
 * like the credential was lost.
 */
export type Cluster = {
  id: string;
  name: string;
  namespace: string;
  serverUrl: string;
  isDefault: boolean;
  isActive: boolean;
  hasKubeconfig: boolean;
  hasToken: boolean;
  /** The McHarbor environment the cluster runs on; '' when it runs elsewhere. */
  environmentId?: string;
  createdAt: string;
  updatedAt: string;
};

/** The create/update body. Omitted credential fields are left as stored. */
export type ClusterInput = {
  name?: string;
  namespace?: string;
  serverUrl?: string;
  kubeconfig?: string;
  bearerToken?: string;
  caCert?: string;
  isDefault?: boolean;
  isActive?: boolean;
  /** '' unlinks the cluster from its environment. */
  environmentId?: string;
};

const CLUSTERS_KEY = ['kubernetes', 'clusters'] as const;

export function useClusters() {
  return useQuery({
    queryKey: CLUSTERS_KEY,
    queryFn: () => api.get<Cluster[]>('/kubernetes/clusters').then(assertSuccess),
    // Cheap, and the picker on every page reads it — a stale list after
    // someone adds a cluster in another tab is a confusing dead end.
    refetchInterval: 60_000,
  });
}

function useClusterInvalidation() {
  const queryClient = useQueryClient();
  return () => queryClient.invalidateQueries({ queryKey: CLUSTERS_KEY });
}

export function useCreateCluster() {
  const { t } = useTranslation('kubernetes');
  const invalidate = useClusterInvalidation();
  return useMutation({
    mutationFn: (body: ClusterInput) => api.post<Cluster>('/kubernetes/clusters', body).then(assertSuccess),
    meta: { success: t('clusters.toast.created') },
    onSuccess: invalidate,
  });
}

export function useUpdateCluster() {
  const { t } = useTranslation('kubernetes');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: ClusterInput & { id: string }) =>
      api.put<Cluster>(`/kubernetes/clusters/${id}`, body).then(assertSuccess),
    meta: { success: t('clusters.toast.updated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: CLUSTERS_KEY });
      // Connection details may have changed, so what the cluster reports may
      // have changed with them.
      void queryClient.invalidateQueries({ queryKey: ['kubernetes', 'summary'] });
    },
  });
}

export function useDeleteCluster() {
  const { t } = useTranslation('kubernetes');
  const invalidate = useClusterInvalidation();
  return useMutation({
    mutationFn: (id: string) => api.del(`/kubernetes/clusters/${id}`).then(assertSuccess),
    meta: { success: t('clusters.toast.deleted') },
    onSuccess: invalidate,
  });
}
