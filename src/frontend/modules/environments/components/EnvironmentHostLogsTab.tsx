// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { IconFileText, IconRefresh } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';
import { LogViewer } from '@resources/components/LogViewer';
import { api } from '@core/api/client';

type LogSource = 'system' | 'kernel' | 'auth' | 'docker';

type HostLogResult = {
  source: LogSource;
  tail: number;
  lines: string[];
  notices: string[];
  fetchedAt: string;
};

type EnvironmentHostLogsTabProps = {
  envId: string;
};

export function EnvironmentHostLogsTab({ envId }: EnvironmentHostLogsTabProps) {
  const { t } = useTranslation('environments');
  const [source, setSource] = useState<LogSource>('system');
  const [tail, setTail] = useState(200);
  const enabled = !!envId;
  const { data, isLoading, isFetching, isError, refetch } = useQuery({
    queryKey: ['environment-host-logs', envId, source, tail],
    queryFn: () =>
      api
        .get<HostLogResult>('/system/os-logs', {
          env: envId,
          source,
          tail: String(tail),
        })
        .then((r) => r.data),
    enabled,
    refetchOnWindowFocus: false,
    staleTime: 30_000,
  });

  useEffect(() => {
    if (!enabled) return;
    void refetch();
  }, [enabled, envId, refetch]);

  const sourceOptions = useMemo(
    () => [
      { value: 'system', label: t('detail.hostLogs.sources.system') },
      { value: 'kernel', label: t('detail.hostLogs.sources.kernel') },
      { value: 'auth', label: t('detail.hostLogs.sources.auth') },
      { value: 'docker', label: t('detail.hostLogs.sources.docker') },
    ],
    [t],
  );

  return (
    <Card>
      <CardHeader className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <CardTitle className="flex items-center gap-2 text-base">
            <IconFileText className="size-5 text-muted-foreground" />
            {t('detail.hostLogs.title')}
          </CardTitle>
          <CardDescription>{t('detail.hostLogs.description')}</CardDescription>
        </div>
        <div className="grid gap-2 sm:grid-cols-[180px_110px_auto]">
          <Select
            value={source}
            onChange={(value) => setSource(value as LogSource)}
            options={sourceOptions}
            searchable={false}
            ariaLabel={t('detail.hostLogs.sourceLabel')}
          />
          <Input
            type="number"
            min={1}
            max={1000}
            value={tail}
            aria-label={t('detail.hostLogs.tailLabel')}
            onChange={(event) => {
              const next = Number(event.target.value);
              setTail(Number.isFinite(next) ? Math.min(1000, Math.max(1, next)) : 1);
            }}
          />
          <Button variant="outline" onClick={() => void refetch()} disabled={isFetching}>
            <IconRefresh className="size-4" />
            {t('detail.hostLogs.refresh')}
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        {!enabled && (
          <div className="rounded-lg border border-border bg-muted/20 px-4 py-3 text-sm text-muted-foreground">
            {t('detail.hostLogs.unavailable')}
          </div>
        )}
        {enabled && data?.notices && data.notices.length > 0 && (
          <div className="mb-3 space-y-2">
            {data.notices.map((notice) => (
              <div
                key={notice}
                className="rounded-lg border border-yellow-500/30 bg-yellow-500/10 px-3 py-2 text-sm text-yellow-800 dark:text-yellow-200"
              >
                {t(`detail.hostLogs.notices.${notice}`, {
                  defaultValue: t('detail.hostLogs.notices.generic'),
                })}
              </div>
            ))}
          </div>
        )}
        {enabled && isLoading && (
          <div className="flex h-64 items-center justify-center">
            <Spinner />
          </div>
        )}
        {enabled && isError && !isLoading && (
          <div className="flex h-64 items-center justify-center rounded-lg border border-border bg-muted/20 text-sm text-muted-foreground">
            {t('detail.hostLogs.unavailable')}
          </div>
        )}
        {enabled && !isLoading && !isError && (
          <LogViewer
            lines={data?.lines ?? []}
            emptyMessage={t('detail.hostLogs.empty')}
            className="h-[480px]"
          />
        )}
      </CardContent>
    </Card>
  );
}