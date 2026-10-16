// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconEye, IconListDetails } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Spinner } from '@resources/components/ui/Spinner';
import { useContainerProcesses } from '../../hooks/useContainerProcesses';
import { useContainerServices } from '../../hooks/useContainerServices';
import { ContainerServicesSection } from '../ContainerServicesSection';
import { ProcessesMobileList } from '../ProcessesMobileList';
import { ProcessContextDrawer, type SelectedProcessContext } from '../ProcessContextDrawer';

type ProcessesTabProps = {
  containerId: string;
  isRunning: boolean;
};

export function ProcessesTab({ containerId, isRunning }: ProcessesTabProps) {
  const { t } = useTranslation('containers');
  const [selectedContext, setSelectedContext] = useState<SelectedProcessContext | null>(null);
  const processesQuery = useContainerProcesses(containerId, isRunning);
  const processes = processesQuery.data;
  const services = useContainerServices(containerId, isRunning);

  const contextKey = `${containerId}|${isRunning}`;
  const [contextFor, setContextFor] = useState(contextKey);
  if (contextFor !== contextKey) {
    setContextFor(contextKey);
    setSelectedContext(null);
  }

  if (!isRunning) {
    return (
      <div className="mc-glass flex h-64 items-center justify-center rounded-lg text-muted-foreground">
        <IconListDetails className="mr-2 size-5" />
        {t('processes.mustBeRunning')}
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <ContainerServicesSection
        result={services.data}
        loading={services.isLoading}
        fetching={services.isFetching}
        error={services.error}
        onRefresh={() => services.refetch()}
        onSelect={(service) =>
          setSelectedContext({
            kind: 'service',
            service,
            initSystem: services.data?.initSystem ?? '',
          })
        }
      />

      <div className="mc-glass rounded-lg p-6">
        <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
          {t('processes.title')}
        </h3>
        {processesQuery.isError ? (
          <ErrorState variant="inline" error={processesQuery.error} onRetry={() => processesQuery.refetch()} />
        ) : processesQuery.isLoading ? (
          <div className="flex h-32 items-center justify-center">
            <Spinner size="md" />
          </div>
        ) : processes && processes.Processes.length > 0 ? (
          <>
            <ProcessesMobileList processes={processes} onSelect={setSelectedContext} />
            <div className="hidden overflow-x-auto md:block">
              <table className="w-full text-left text-xs">
                <thead className="bg-muted">
                  <tr className="border-b border-border">
                    {processes.Titles.map((title) => (
                      <th
                        key={title}
                        scope="col"
                        className="px-3 py-2 font-medium whitespace-nowrap text-muted-foreground"
                      >
                        {title}
                      </th>
                    ))}
                    <th scope="col" className="w-12 px-3 py-2">
                      <span className="sr-only">{t('processes.actions')}</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {processes.Processes.map((process) => (
                    <tr key={process.join('\u0000')} className="border-b border-border last:border-0 hover:bg-muted/50">
                      {process.map((value, index) => (
                        <td
                          key={processes.Titles[index] ?? `column-${index + 1}`}
                          className="px-3 py-2 font-mono whitespace-nowrap text-foreground"
                        >
                          {value}
                        </td>
                      ))}
                      <td className="px-3 py-2 text-right">
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          className="size-11 sm:size-9"
                          onClick={() =>
                            setSelectedContext({
                              kind: 'process',
                              titles: processes.Titles,
                              values: process,
                            })
                          }
                          aria-label={t('processes.viewProcessDetails')}
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
          </>
        ) : (
          <p className="text-sm text-muted-foreground">{t('processes.noProcesses')}</p>
        )}
      </div>

      <ProcessContextDrawer context={selectedContext} onClose={() => setSelectedContext(null)} />
    </div>
  );
}
