// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { queryOptions, useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type ScannerSettingsData = {
  trivyEnabled: boolean;
  grypeEnabled: boolean;
  clairEnabled: boolean;
  clairUrl: string;
  defaultScanner: string;
  scanTimeout: number;
  scanOnInstall: boolean;
  scanOnUpdate: boolean;
};

export type ScannerInfo = {
  name: string;
  available: boolean;
};

export type ScannersResponse = {
  scanners: ScannerInfo[];
  defaultScanner: string;
};

export function getScannerSettingsQueryOptions() {
  return queryOptions({
    queryKey: ['settings', 'scanners'],
    queryFn: () => api.get<ScannerSettingsData>('/settings/scanners').then((r) => r.data),
  });
}

export function useScannerSettings() {
  return useQuery(getScannerSettingsQueryOptions());
}

export function useSaveScannerSettings() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');
  return useMutation({
    mutationFn: (data: ScannerSettingsData) =>
      api.put('/settings/scanners', data).then(assertSuccess),
    meta: { success: t('toast.scannerSettingsSaved') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['settings', 'scanners'] });
      queryClient.invalidateQueries({ queryKey: ['scans', 'scanners'] });
    },
  });
}

export function useAvailableScanners() {
  return useQuery(getAvailableScannersQueryOptions());
}

export function getAvailableScannersQueryOptions() {
  return queryOptions({
    queryKey: ['scans', 'scanners'],
    queryFn: () => api.get<ScannersResponse>('/scans/scanners').then((r) => r.data),
  });
}
