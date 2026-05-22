// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconListDetails, IconRefresh } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { Badge } from '@resources/components/ui/Badge';
import { useContainerProcesses } from '../../hooks/useContainerProcesses';
import { useContainerServices } from '../../hooks/useContainerServices';

type ProcessesTabProps = {
  containerId: string;
  isRunning: boolean;
};

function serviceStatusVariant(status: string) {
  switch (status) {
    case 'running':
      return 'success' as const;
    case 'stopped':
    case 'dead':
    case 'exited':
    case 'failed':
      return 'destructive' as const;
    default:
      return 'secondary' as const;
  }
}

export function ProcessesTab({ containerId, isRunning }: ProcessesTabProps) {
  const { t } = useTranslation('containers');
  const { data: processes, isLoading } = useContainerProcesses(containerId, isRunning);
  const {
    data: servicesResult,
    isLoading: servicesLoading,
    refetch: refetchServices,
    isFetching: servicesFetching,
  } = useContainerServices(containerId, isRunning);

  if (!isRunning) {
    return (
      <div className="flex h-64 items-center justify-center rounded-lg border border-border bg-card text-muted-foreground">
        <IconListDetails className="mr-2 h-5 w-5" />
        {t('processes.mustBeRunning')}
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="rounded-lg border border-border bg-card p-6">
        <div className="mb-4 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <h3 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">
              {t('processes.services')}
            </h3>
            {servicesResult?.initSystem && (
              <Badge variant="outline" className="text-[10px] px-2 py-0.5">
                {t('processes.initSystem', { system: servicesResult.initSystem })}
              </Badge>
            )}
          </div>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => refetchServices()}
            disabled={servicesFetching}
            aria-label={t('processes.refreshServices')}
          >
            <IconRefresh className={`h-3.5 w-3.5 ${servicesFetching ? 'animate-spin' : ''}`} />
            {t('processes.refreshServices')}
          </Button>
        </div>

        {servicesLoading ? (
          <div className="flex h-20 items-center justify-center gap-2 text-xs text-muted-foreground">
            <Spinner size="sm" />
            {t('processes.loadingServices')}
          </div>
        ) : servicesResult && servicesResult.services.length > 0 ? (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead>
                <tr className="border-b border-border">
                  <th className="px-3 py-2 font-medium text-muted-foreground">
                    {t('processes.serviceName')}
                  </th>
                  <th className="px-3 py-2 font-medium text-muted-foreground">
                    {t('processes.serviceStatus')}
                  </th>
                </tr>
              </thead>
              <tbody>
                {servicesResult.services.map((svc) => (
                  <tr
                    key={svc.name}
                    className="border-b border-border last:border-0 hover:bg-muted/50"
                  >
                    <td className="px-3 py-2 font-mono text-foreground">{svc.name}</td>
                    <td className="px-3 py-2">
                      <Badge variant={serviceStatusVariant(svc.status)} className="text-[10px] px-2 py-0.5">
                        {svc.status}
                      </Badge>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">{t('processes.noServices')}</p>
        )}
      </div>

      <div className="rounded-lg border border-border bg-card p-6">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('processes.title')}</h3>
        {isLoading ? (
          <div className="flex h-32 items-center justify-center">
            <Spinner size="md" />
          </div>
        ) : processes && processes.Processes.length > 0 ? (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead>
                <tr className="border-b border-border">
                  {processes.Titles.map((title) => (
                    <th
                      key={title}
                      className="whitespace-nowrap px-3 py-2 font-medium text-muted-foreground"
                    >
                      {title}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {processes.Processes.map((proc) => (
                  <tr key={proc.join('\u0000')} className="border-b border-border last:border-0 hover:bg-muted/50">
                    {proc.map((val, j) => (
                      <td key={processes.Titles[j] ?? `column-${j + 1}`} className="whitespace-nowrap px-3 py-2 font-mono text-foreground">
                        {val}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">{t('processes.noProcesses')}</p>
        )}
      </div>
    </div>
  );
}
