// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { queryOptions, useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

/**
 * The scanner settings that are the host's rather than any engine's.
 *
 * Which engines are enabled, the Clair URL and the default scanner all
 * left this type when the engines became plugins: they are per-instance
 * properties of a `scanner_configs` row, edited through
 * `useScannerConfigs` in the security module. What is left applies to
 * every engine equally.
 */
export type ScannerSettingsData = {
  scanTimeout: number;
  scanOnInstall: boolean;
  scanOnUpdate: boolean;
};

/**
 * One scanner a scan may be started with. Returned by /scans/scanners,
 * which is the picker feed — the container detail page and the App
 * Store install flow read it, not the Scanners tab.
 */
export type ScannerInfo = {
  /**
   * The `scanner_configs` row. Sending this back on a scan request
   * names one instance exactly, which is the only way to pick the
   * production Clair over the staging one.
   */
  id: string;
  /** The engine type ("trivy"), unchanged from before the plugin move. */
  name: string;
  /** The operator's name for this instance ("Clair — prod"). */
  label: string;
  available: boolean;
  isDefault: boolean;
};

export type ScannersResponse = {
  scanners: ScannerInfo[];
  /** The default row's engine type, kept for callers that preselect it. */
  defaultScanner: string;
  /** The default row's id — what to send to run exactly that scanner. */
  defaultScannerId: string;
};

export function getScannerSettingsQueryOptions() {
  return queryOptions({
    queryKey: ['settings', 'scanners'],
    queryFn: () => api.get<ScannerSettingsData>('/settings/scanners').then(assertSuccess),
  });
}

export function useScannerSettings() {
  return useQuery(getScannerSettingsQueryOptions());
}

export function useSaveScannerSettings() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');
  return useMutation({
    mutationFn: (data: ScannerSettingsData) => api.put('/settings/scanners', data).then(assertSuccess),
    meta: { success: t('toast.scannerSettingsSaved') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['settings', 'scanners'] });
      void queryClient.invalidateQueries({ queryKey: ['scans', 'scanners'] });
    },
  });
}

export function useAvailableScanners() {
  return useQuery(getAvailableScannersQueryOptions());
}

export function getAvailableScannersQueryOptions() {
  return queryOptions({
    queryKey: ['scans', 'scanners'],
    queryFn: () => api.get<ScannersResponse>('/scans/scanners').then(assertSuccess),
  });
}
