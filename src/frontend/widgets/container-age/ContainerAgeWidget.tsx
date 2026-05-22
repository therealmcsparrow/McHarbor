// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useContainers } from '@modules/containers/hooks/useContainers';
import { timeAgo } from '@resources/utils/format';
import { StatusBadge, CONTAINER_STATUS } from '@resources/components/ui/StatusBadge';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';

export default function ContainerAgeWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const { data: containers, isLoading } = useContainers(true);

  const sorted = useMemo(() => {
    if (!containers) return [];
    return [...containers]
      .sort((a, b) => a.Created - b.Created)
      .slice(0, 8);
  }, [containers]);

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('loading')}
      </div>
    );
  }

  if (sorted.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('containerAgeWidget.noContainers')}
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <h3 className="shrink-0 px-4 pt-3 pb-2 text-sm font-semibold text-foreground">
        {t('containerAgeWidget.title')}
      </h3>
      <div className="flex-1 overflow-y-auto px-4 pb-2">
        <table className="w-full text-xs">
          <thead>
            <tr className="border-b border-border text-left text-muted-foreground">
              <th className="pb-1.5 font-medium">{t('containerAgeWidget.container')}</th>
              <th className="pb-1.5 font-medium">{t('containerAgeWidget.age')}</th>
              <th className="pb-1.5 font-medium">{t('containerAgeWidget.state')}</th>
            </tr>
          </thead>
          <tbody>
            {sorted.map((c) => (
              <tr key={c.Id} className="border-b border-border/50 last:border-0">
                <td className="max-w-[140px] truncate py-1.5 pr-2 text-foreground">
                  {(c.Names?.[0] ?? '').replace(/^\//, '')}
                </td>
                <td className="py-1.5 pr-2 text-muted-foreground">
                  {timeAgo(new Date(c.Created * 1000).toISOString())}
                </td>
                <td className="py-1.5">
                  <StatusBadge status={c.State ?? 'unknown'} map={CONTAINER_STATUS} className="text-[10px] px-1.5 py-0.5" />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
