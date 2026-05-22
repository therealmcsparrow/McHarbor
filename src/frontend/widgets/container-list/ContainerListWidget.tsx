// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import { useContainers } from '@modules/containers/hooks/useContainers';
import { StatusBadge, CONTAINER_STATUS } from '@resources/components/ui/StatusBadge';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';

export default function ContainerListWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const { data: containers, isLoading } = useContainers(true);

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('containerListWidget.loadingContainers')}
      </div>
    );
  }

  if (!containers || containers.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('containerListWidget.noContainersFound')}
      </div>
    );
  }

  const visible = containers.slice(0, 10);

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <h3 className="shrink-0 px-4 pt-3 pb-2 text-sm font-semibold text-foreground">{t('containerListWidget.title')}</h3>
      <div className="flex-1 overflow-y-auto px-4 pb-2">
        <table className="w-full text-xs">
          <thead>
            <tr className="border-b border-border text-left text-muted-foreground">
              <th className="pb-1.5 font-medium">{t('containerListWidget.name')}</th>
              <th className="pb-1.5 font-medium">{t('containerListWidget.image')}</th>
              <th className="pb-1.5 font-medium">{t('containerListWidget.state')}</th>
            </tr>
          </thead>
          <tbody>
            {visible.map((c) => (
              <tr key={c.Id} className="border-b border-border/50 last:border-0">
                <td className="max-w-[140px] truncate py-1.5 pr-2 text-foreground">
                  {(c.Names?.[0] ?? '').replace(/^\//, '')}
                </td>
                <td className="max-w-[120px] truncate py-1.5 pr-2 text-muted-foreground">
                  {c.Image?.split(':')[0]?.split('/').pop() ?? c.Image}
                </td>
                <td className="py-1.5">
                  <StatusBadge status={c.State ?? 'unknown'} map={CONTAINER_STATUS} className="text-[10px] px-1.5 py-0.5" />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {containers.length > 10 && (
        <div className="shrink-0 border-t border-border px-4 py-2">
          <Link to="/containers" className="text-xs text-primary hover:underline">
            {t('containerListWidget.viewAll', { count: containers.length })} &rarr;
          </Link>
        </div>
      )}
    </div>
  );
}
