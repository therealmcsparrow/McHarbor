// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useDeployments } from '@modules/deployments/hooks/useDeployments';
import { Badge } from '@resources/components/ui/Badge';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';

export default function DeploymentStatusWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const { data: deployments, isLoading, isError } = useDeployments();

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('loading')}
      </div>
    );
  }

  if (isError || !deployments) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('deploymentStatusWidget.notAvailable')}
      </div>
    );
  }

  if (deployments.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('deploymentStatusWidget.noDeployments')}
      </div>
    );
  }

  const visible = deployments.slice(0, 10);

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <h3 className="shrink-0 px-4 pt-3 pb-2 text-sm font-semibold text-foreground">
        {t('deploymentStatusWidget.title')}
      </h3>
      <div className="flex-1 overflow-y-auto px-4 pb-2">
        <table className="w-full text-xs">
          <thead>
            <tr className="border-b border-border text-left text-muted-foreground">
              <th className="pb-1.5 font-medium">{t('deploymentStatusWidget.name')}</th>
              <th className="pb-1.5 font-medium">{t('deploymentStatusWidget.namespace')}</th>
              <th className="pb-1.5 font-medium">{t('deploymentStatusWidget.replicas')}</th>
              <th className="pb-1.5 font-medium">{t('deploymentStatusWidget.status')}</th>
            </tr>
          </thead>
          <tbody>
            {visible.map((d) => {
              const ready = d.available ?? 0;
              const desired = d.desiredReplicas ?? 0;
              const isHealthy = ready >= desired && desired > 0;
              return (
                <tr key={`${d.namespace}-${d.name}`} className="border-b border-border/50 last:border-0">
                  <td className="max-w-[120px] truncate py-1.5 pr-2 text-foreground">{d.name}</td>
                  <td className="max-w-[80px] truncate py-1.5 pr-2 text-muted-foreground">{d.namespace}</td>
                  <td className="py-1.5 pr-2 font-mono text-muted-foreground">
                    {ready}/{desired}
                  </td>
                  <td className="py-1.5">
                    <Badge
                      variant={isHealthy ? 'success' : 'warning'}
                      className="text-[10px] px-1.5 py-0.5"
                    >
                      {isHealthy ? t('deploymentStatusWidget.healthy') : t('deploymentStatusWidget.progressing')}
                    </Badge>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
