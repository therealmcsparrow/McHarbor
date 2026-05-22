// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import type { DockerSystemInfo } from '@core/types/docker';
import { useHostMetrics } from '@resources/hooks/useHostMetrics';
import { formatBytes } from '@resources/utils/format';
import { useDockerInfo } from '@modules/docker/hooks/useDockerInfo';
import { useCurrentEnvironmentActivitySettings } from './useCurrentEnvironmentActivitySettings';

const BYTE_UNITS: Record<string, number> = {
  b: 1,
  kb: 1024,
  kib: 1024,
  mb: 1024 ** 2,
  mib: 1024 ** 2,
  gb: 1024 ** 3,
  gib: 1024 ** 3,
  tb: 1024 ** 4,
  tib: 1024 ** 4,
  pb: 1024 ** 5,
  pib: 1024 ** 5,
};

function parseByteCount(value: string): number | null {
  const trimmed = value.trim();
  const match = trimmed.match(/^([\d.,]+)\s*([a-zA-Z]+)$/);
  if (!match) {
    return null;
  }

  const amountText = match[1];
  const unitText = match[2];
  if (!amountText || !unitText) {
    return null;
  }

  const amount = Number.parseFloat(amountText.replace(/,/g, ''));
  const unit = unitText.toLowerCase();
  const multiplier = unit ? BYTE_UNITS[unit] : undefined;
  if (!Number.isFinite(amount) || !multiplier) {
    return null;
  }

  return amount * multiplier;
}

function readDriverStatusBytes(info: DockerSystemInfo | undefined, keys: string[]): number | null {
  if (!info) {
    return null;
  }

  for (const [label, value] of info.driverStatus) {
    if (!label || !value) {
      continue;
    }

    const normalized = label.trim().toLowerCase();
    if (!keys.includes(normalized)) {
      continue;
    }

    const bytes = parseByteCount(value);
    if (bytes && bytes > 0) {
      return bytes;
    }
  }

  return null;
}

function resolveDiskCapacity(info: DockerSystemInfo | undefined, usedBytes: number): number | null {
  const total = readDriverStatusBytes(info, ['data space total', 'space total', 'total space']);
  if (total && total > 0) {
    return total;
  }

  const available = readDriverStatusBytes(info, ['data space available', 'space available', 'available space']);
  if (available && available >= 0) {
    return usedBytes + available;
  }

  return null;
}

export function useDockerDiskUsageNotifications() {
  const { t } = useTranslation('environments');
  const {
    currentId,
    currentEnvironment,
    dockerDiskUsageNotificationsEnabled,
    dockerDiskUsageThresholdPercent,
  } = useCurrentEnvironmentActivitySettings();
  const isDockerEnvironment = currentEnvironment?.orchestratorType === 'docker';
  const { data: hostMetrics } = useHostMetrics();
  const { data: dockerInfo } = useDockerInfo(Boolean(currentId && isDockerEnvironment));
  const alertedRef = useRef<Record<string, boolean>>({});

  const usagePercent = useMemo(() => {
    if (!hostMetrics || !dockerInfo) {
      return null;
    }

    const capacity = resolveDiskCapacity(dockerInfo, hostMetrics.disk.total);
    if (!capacity || capacity <= 0) {
      return null;
    }

    return (hostMetrics.disk.total / capacity) * 100;
  }, [dockerInfo, hostMetrics]);

  useEffect(() => {
    if (!currentId || !currentEnvironment || !isDockerEnvironment) {
      return;
    }

    if (!dockerDiskUsageNotificationsEnabled || usagePercent == null) {
      alertedRef.current[currentId] = false;
      return;
    }

    const isThresholdExceeded = usagePercent >= dockerDiskUsageThresholdPercent;
    const hasAlerted = alertedRef.current[currentId] ?? false;

    if (isThresholdExceeded && !hasAlerted) {
      toast.warning(
        t('detail.activity.diskUsageExceededTitle', { name: currentEnvironment.name }),
        {
          description: t('detail.activity.diskUsageExceededDescription', {
            percent: usagePercent.toFixed(1),
            threshold: dockerDiskUsageThresholdPercent,
            total: formatBytes(hostMetrics?.disk.total ?? 0),
          }),
        }
      );
      alertedRef.current[currentId] = true;
      return;
    }

    if (!isThresholdExceeded && hasAlerted) {
      alertedRef.current[currentId] = false;
    }
  }, [
    currentEnvironment,
    currentId,
    dockerDiskUsageNotificationsEnabled,
    dockerDiskUsageThresholdPercent,
    hostMetrics,
    isDockerEnvironment,
    t,
    usagePercent,
  ]);
}
