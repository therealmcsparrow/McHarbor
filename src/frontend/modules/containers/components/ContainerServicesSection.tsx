// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconEye, IconRefresh } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Spinner } from '@resources/components/ui/Spinner';
import type { ContainerService, ContainerServicesResult } from '../hooks/useContainerServices';

type ContainerServicesSectionProps = {
  result?: ContainerServicesResult;
  loading: boolean;
  fetching: boolean;
  error?: unknown;
  onRefresh: () => void;
  onSelect: (service: ContainerService) => void;
};

function serviceStatusVariant(statusValue: string) {
  if (statusValue === 'running') return 'success' as const;
  if (['stopped', 'dead', 'exited', 'failed'].includes(statusValue)) return 'destructive' as const;
  return 'secondary' as const;
}

export function ContainerServicesSection({
  result,
  loading,
  fetching,
  error,
  onRefresh,
  onSelect,
}: ContainerServicesSectionProps) {
  const { t } = useTranslation('containers');
  return (
    <div className="mc-glass rounded-lg p-6">
      <div className="mb-4 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <h3 className="text-sm font-semibold tracking-wider text-muted-foreground uppercase">
            {t('processes.services')}
          </h3>
          {result?.initSystem ? (
            <Badge variant="outline" className="px-2 py-0.5 text-[10px]">
              {t('processes.initSystem', { system: result.initSystem })}
            </Badge>
          ) : null}
        </div>
        <Button
          variant="ghost"
          size="sm"
          onClick={onRefresh}
          disabled={fetching}
          aria-label={t('processes.refreshServices')}
        >
          <IconRefresh className={`size-3.5 ${fetching ? 'animate-spin' : ''}`} />
          {t('processes.refreshServices')}
        </Button>
      </div>
      {error ? (
        <ErrorState variant="inline" error={error} onRetry={onRefresh} />
      ) : loading ? (
        <div className="flex h-20 items-center justify-center gap-2 text-xs text-muted-foreground">
          <Spinner size="sm" />
          {t('processes.loadingServices')}
        </div>
      ) : result && result.services.length > 0 ? (
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-muted">
              <tr className="border-b border-border">
                <th scope="col" className="px-3 py-2 font-medium text-muted-foreground">
                  {t('processes.serviceName')}
                </th>
                <th scope="col" className="px-3 py-2 font-medium text-muted-foreground">
                  {t('processes.serviceStatus')}
                </th>
                <th scope="col" className="w-12 px-3 py-2">
                  <span className="sr-only">{t('processes.actions')}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {result.services.map((service) => (
                <tr key={service.name} className="border-b border-border last:border-0 hover:bg-muted/50">
                  <td className="px-3 py-2 font-mono text-foreground">{service.name}</td>
                  <td className="px-3 py-2">
                    <Badge variant={serviceStatusVariant(service.status)} className="px-2 py-0.5 text-[10px]">
                      {service.status}
                    </Badge>
                  </td>
                  <td className="px-3 py-2 text-right">
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      className="size-11 sm:size-9"
                      onClick={() => onSelect(service)}
                      aria-label={t('processes.viewServiceDetails', {
                        name: service.name,
                      })}
                      aria-haspopup="dialog"
                    >
                      <IconEye className="size-4" />
                    </Button>
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
  );
}
