// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { BatchProgressLogLevel } from '@resources/hooks/useBatchProgressOperation';
import {
  getAvailableScannersQueryOptions,
  getScannerSettingsQueryOptions,
} from '@resources/hooks/useScannerSettings';

const scanPollIntervalMs = 2_000;
const scanTimeoutMs = 10 * 60 * 1_000;

type OperationScan = {
  id: string;
  imageRef: string;
  scanner: string;
  status: 'pending' | 'running' | 'completed' | 'failed';
  totalVulns: number;
  criticalCount: number;
  highCount: number;
  mediumCount: number;
  lowCount: number;
};

type ScanLogger = (message: string, options?: { level?: BatchProgressLogLevel }) => void;

type ScanSummary = {
  image: string;
  scanner: string;
  status: 'completed' | 'failed';
  totalVulns: number;
  criticalCount: number;
  highCount: number;
  mediumCount: number;
  lowCount: number;
};

type ScanPlan =
  | { status: 'ready'; scanners: string[] }
  | { status: 'disabled' }
  | { status: 'none' }
  | { status: 'unavailable' };

export type OperationScanBatchResult = {
  attempted: number;
  completed: number;
  failed: number;
  skipped: boolean;
};

function sleep(ms: number) {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}

function summarizeScan(
  scan: OperationScan,
  status: 'completed' | 'failed',
): ScanSummary {
  return {
    image: scan.imageRef,
    scanner: scan.scanner,
    status,
    totalVulns: scan.totalVulns,
    criticalCount: scan.criticalCount,
    highCount: scan.highCount,
    mediumCount: scan.mediumCount,
    lowCount: scan.lowCount,
  };
}

export function useOperationScans() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('common');

  function createBatchScanner() {
    let scanPlanPromise: Promise<ScanPlan> | null = null;
    const imageCache = new Map<string, Promise<ScanSummary[]>>();

    async function loadScanPlan(): Promise<ScanPlan> {
      const [settings, scannersResponse] = await Promise.all([
        queryClient.ensureQueryData(getScannerSettingsQueryOptions()),
        queryClient.ensureQueryData(getAvailableScannersQueryOptions()),
      ]);

      if (!settings || !scannersResponse) {
        return { status: 'unavailable' };
      }

      if (!settings.scanOnUpdate) {
        return { status: 'disabled' };
      }

      const scanners = (scannersResponse.scanners ?? [])
        .filter((scanner) => scanner.available)
        .map((scanner) => scanner.name);

      if (scanners.length === 0) {
        return { status: 'none' };
      }

      return { status: 'ready', scanners };
    }

    async function getScanPlan() {
      if (!scanPlanPromise) {
        scanPlanPromise = loadScanPlan();
      }
      return scanPlanPromise;
    }

    async function waitForScan(scanId: string): Promise<OperationScan> {
      const startedAt = Date.now();

      while (Date.now() - startedAt <= scanTimeoutMs) {
        const scan = await api.get<OperationScan>(`/scans/${scanId}`).then(assertSuccess);
        if (scan.status === 'completed' || scan.status === 'failed') {
          return scan;
        }
        await sleep(scanPollIntervalMs);
      }

      throw new Error('scan timed out');
    }

    async function runImageScans(image: string, scanners: string[], log: ScanLogger) {
      const summaries: ScanSummary[] = [];

      for (const scanner of scanners) {
        log(t('operations.log.scanStarting', { scanner, image }));

        try {
          const scan = await api
            .post<OperationScan>('/scans', {
              imageRef: image,
              scanner,
              environmentId: envId ?? '',
            })
            .then(assertSuccess);

          log(t('operations.log.scanWaiting', { scanner, image }));

          const completedScan = await waitForScan(scan.id);
          const summary = summarizeScan(
            completedScan,
            completedScan.status === 'completed' ? 'completed' : 'failed',
          );
          summaries.push(summary);

          if (summary.status === 'completed') {
            log(
              t('operations.log.scanCompleted', {
                scanner,
                image,
                total: summary.totalVulns,
                critical: summary.criticalCount,
                high: summary.highCount,
                medium: summary.mediumCount,
                low: summary.lowCount,
              }),
              { level: 'success' },
            );
          } else {
            log(t('operations.log.scanFailed', { scanner, image }), { level: 'error' });
          }
        } catch {
          summaries.push({
            image,
            scanner,
            status: 'failed',
            totalVulns: 0,
            criticalCount: 0,
            highCount: 0,
            mediumCount: 0,
            lowCount: 0,
          });
          log(t('operations.log.scanFailed', { scanner, image }), { level: 'error' });
        }
      }

      await queryClient.invalidateQueries({ queryKey: ['scans'] });
      return summaries;
    }

    async function runScansForImages(
      images: string[],
      log: ScanLogger,
    ): Promise<OperationScanBatchResult> {
      const uniqueImages = [...new Set(images.filter(Boolean))];
      if (uniqueImages.length === 0) {
        return { attempted: 0, completed: 0, failed: 0, skipped: true };
      }

      const scanPlan = await getScanPlan();
      if (scanPlan.status !== 'ready') {
        if (scanPlan.status === 'disabled') {
          log(t('operations.log.scanPolicyDisabled'), { level: 'warning' });
        } else if (scanPlan.status === 'none') {
          log(t('operations.log.scanNoEnabled'), { level: 'warning' });
        } else {
          log(t('operations.log.scanUnavailable'), { level: 'warning' });
        }

        return { attempted: 0, completed: 0, failed: 0, skipped: true };
      }

      const summaries: ScanSummary[] = [];
      for (const image of uniqueImages) {
        let imageScanPromise = imageCache.get(image);
        if (!imageScanPromise) {
          imageScanPromise = runImageScans(image, scanPlan.scanners, log);
          imageCache.set(image, imageScanPromise);
        } else {
          log(t('operations.log.scanCached', { image }));
        }

        summaries.push(...(await imageScanPromise));
      }

      return {
        attempted: summaries.length,
        completed: summaries.filter((summary) => summary.status === 'completed').length,
        failed: summaries.filter((summary) => summary.status === 'failed').length,
        skipped: false,
      };
    }

    return {
      runScansForImages,
    };
  }

  return {
    createBatchScanner,
  };
}
