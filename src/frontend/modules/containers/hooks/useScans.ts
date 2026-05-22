// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api, type PaginatedData } from '@core/api/client';
import { envParams } from '@core/api/env-url';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess } from '@resources/utils/api-mutation';

export type Scan = {
  id: string;
  imageRef: string;
  scanner: string;
  status: 'pending' | 'running' | 'completed' | 'failed';
  severity: string;
  totalVulns: number;
  criticalCount: number;
  highCount: number;
  mediumCount: number;
  lowCount: number;
  environmentId: string;
  startedAt: string;
  completedAt: string;
  createdAt: string;
  updatedAt: string;
};

export type ScanVulnerability = {
  id: string;
  scanId: string;
  vulnId: string;
  pkgName: string;
  pkgVersion: string;
  fixedVersion: string;
  severity: string;
  title: string;
  description: string;
  url: string;
};

export function useImageScans(imageRef: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['scans', 'by-image', imageRef, envId],
    queryFn: () =>
      api
        .get<PaginatedData<Scan>>('/scans/by-image', envParams({ image: imageRef }))
        .then((r) => r.data),
    enabled: !!imageRef,
    refetchInterval: 15_000,
  });
}

export function useScanVulnerabilities(scanId: string | null) {
  return useQuery({
    queryKey: ['scans', scanId, 'vulnerabilities'],
    queryFn: () =>
      api
        .get<PaginatedData<ScanVulnerability>>(`/scans/${scanId}/vulnerabilities`, {
          per_page: '100',
        })
        .then((r) => r.data),
    enabled: !!scanId,
  });
}

export function useStartScan() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('containers');
  return useMutation({
    mutationFn: (data: { imageRef: string; scanner: string; environmentId: string }) =>
      api.post<Scan>('/scans', data).then(assertSuccess),
    meta: { success: t('scan.toast.scanStarted') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['scans'] });
    },
  });
}

export function useDeleteScan() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('containers');
  return useMutation({
    mutationFn: (id: string) => api.del(`/scans/${id}`).then(assertSuccess),
    meta: { success: t('scan.toast.scanDeleted') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['scans'] });
    },
  });
}
