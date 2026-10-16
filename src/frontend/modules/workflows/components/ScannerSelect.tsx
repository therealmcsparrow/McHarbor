// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { getAvailableScannersQueryOptions, type ScannerInfo } from '@resources/hooks/useScannerSettings';
import { ResourcePicker, type ResourcePickerItem } from './ResourcePicker';

interface ScannerSelectProps {
  value: string;
  onChange: (value: string) => void;
}

/**
 * A saved node may carry an engine type ("the oldest enabled row of that type" on the
 * backend) rather than an instance id, so the matching instance answers for it too.
 */
export function scannerItems(scanners: ScannerInfo[], value: string): ResourcePickerItem[] {
  const available = scanners.filter((s) => s.available);
  const items = available.map((s) => ({ value: s.id, title: s.label || s.name, subtitle: s.name }));
  if (value && !available.some((s) => s.id === value)) {
    const byType = available.find((s) => s.name === value);
    if (byType) items.unshift({ value, title: byType.label || byType.name, subtitle: byType.name });
  }
  return items;
}

/**
 * The scanner field of a workflow node. The list is whatever the installed scanner
 * plugins registered and an operator configured, so the host never names an engine.
 */
export function ScannerSelect({ value, onChange }: ScannerSelectProps) {
  const { t } = useTranslation('common');
  const allowed = usePermission(PERM.SCANS_VIEW);
  const { data, isLoading } = useQuery({ ...getAvailableScannersQueryOptions(), enabled: allowed });
  const items = scannerItems(data?.scanners ?? [], value);
  const none = allowed && Boolean(data) && items.length === 0;

  return (
    <ResourcePicker
      value={value}
      onChange={onChange}
      isLoading={allowed && isLoading}
      placeholder={t('workflows.scannerSelectPlaceholder')}
      emptyLabel={t('workflows.scannerSelectUnavailable')}
      clearLabel={t('workflows.scannerSelectDefault')}
      items={items}
      permission={PERM.SCANS_VIEW}
      unavailableReason={none ? t('workflows.scannerSelectUnavailable') : undefined}
    />
  );
}
