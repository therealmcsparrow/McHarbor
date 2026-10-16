// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { useQuery } from '@tanstack/react-query';
import {
  IconAlertTriangle,
  IconCircleCheck,
  IconCircleDot,
  IconPlayerPause,
  IconPlayerStop,
  IconStack2,
} from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Spinner } from '@resources/components/ui/Spinner';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess } from '@resources/utils/api-mutation';

type ContainerService = {
  Name: string;
  Status: string;
  Sub?: string;
};

type ContainerServicesResult = {
  InitSystem: string;
  Services: ContainerService[];
};

type ServicesTabProps = {
  containerId: string;
  isRunning: boolean;
};

function statusVariant(serviceStatus: string): 'success' | 'destructive' | 'warning' | 'secondary' {
  const s = serviceStatus.toLowerCase();
  if (s === 'running' || s === 'active' || s === 'started') return 'success';
  if (s === 'stopped' || s === 'inactive' || s === 'failed' || s === 'dead') return 'destructive';
  if (s === 'paused' || s === 'waiting' || s === 'scheduled') return 'warning';
  return 'secondary';
}

function statusIcon(serviceStatus: string) {
  const s = serviceStatus.toLowerCase();
  if (s === 'running' || s === 'active' || s === 'started') return IconCircleCheck;
  if (s === 'stopped' || s === 'inactive' || s === 'failed' || s === 'dead') return IconPlayerStop;
  if (s === 'paused') return IconPlayerPause;
  if (s === 'waiting' || s === 'scheduled') return IconCircleDot;
  return IconAlertTriangle;
}

export function ServicesTab({ containerId, isRunning }: ServicesTabProps) {
  const { t } = useTranslation('containers');
  const envId = useEnvironmentStore((s) => s.currentId);

  const query = useQuery({
    queryKey: ['container-services', envId, containerId],
    queryFn: () =>
      api
        .get<ContainerServicesResult>(`/containers/${containerId}/services`, envId ? { env: envId } : {})
        .then(assertSuccess),
    enabled: isRunning,
    refetchOnMount: 'always',
  });

  if (!isRunning) {
    return (
      <div className="mc-glass rounded-lg p-6 text-center text-sm text-muted-foreground">
        {t('services.containerStopped')}
      </div>
    );
  }

  if (query.isLoading) {
    return (
      <div className="mc-glass flex h-32 items-center justify-center rounded-lg">
        <Spinner size="md" />
      </div>
    );
  }

  if (query.isError) {
    return <ErrorState variant="banner" title={t('services.error')} error={query.error} />;
  }

  const data = query.data;
  const services = data?.Services ?? [];

  return (
    <div className="space-y-4">
      <div className="mc-glass flex items-center justify-between rounded-lg p-4">
        <div className="flex items-center gap-2">
          <IconStack2 className="size-5 text-primary" />
          <div className="text-sm font-medium text-foreground">{t('services.title')}</div>
        </div>
        <Badge variant="outline">{t('services.initSystem', { system: data?.InitSystem ?? '—' })}</Badge>
      </div>

      {services.length === 0 ? (
        <div className="mc-glass rounded-lg p-6 text-center text-sm text-muted-foreground">{t('services.empty')}</div>
      ) : (
        <div className="mc-glass overflow-hidden rounded-lg">
          <table className="w-full text-sm">
            <thead className="bg-muted/50 text-xs tracking-wider text-muted-foreground uppercase">
              <tr>
                <th scope="col" className="px-4 py-2 text-left">
                  {t('services.colName')}
                </th>
                <th scope="col" className="px-4 py-2 text-left">
                  {t('services.colStatus')}
                </th>
                <th scope="col" className="px-4 py-2 text-left">
                  {t('services.colSub')}
                </th>
              </tr>
            </thead>
            <tbody>
              {services.map((svc, i) => {
                const Icon = statusIcon(svc.Status);
                return (
                  <tr key={`${svc.Name}-${i}`} className="border-t border-border">
                    <td className="px-4 py-2 font-mono text-xs text-foreground">{svc.Name}</td>
                    <td className="px-4 py-2">
                      <div className="flex items-center gap-2">
                        <Icon className="size-4" />
                        <Badge variant={statusVariant(svc.Status)}>{svc.Status}</Badge>
                      </div>
                    </td>
                    <td className="px-4 py-2 text-xs text-muted-foreground">{svc.Sub ?? '—'}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
