// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { IconChevronRight, IconFileText, IconRefresh } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { LogViewer } from '@resources/components/LogViewer';
import { api } from '@core/api/client';
import { useNavigate } from 'react-router';
import type { EnvironmentInfo } from '../hooks/useEnvironments';
import { hostLogNoticeText } from './hostLogNotices';
import type { HostLogResult, HostLogSource } from './hostLogNotices';

import { assertSuccess } from '@resources/utils/api-mutation';

type LogSource = HostLogSource;

type EnvironmentHostLogPreviewProps = {
  env: EnvironmentInfo;
};

// no polling: reading host OS logs shells out on the environment, and over the
// agent transport that is a round trip per read. The preview refreshes when the
// operator asks (the Refresh button) or when the selected source changes.
export function EnvironmentHostLogPreview({ env }: EnvironmentHostLogPreviewProps) {
  const { t } = useTranslation('environments');
  const navigate = useNavigate();
  const [source, setSource] = useState<LogSource>('system');
  const previewTail = 15;
  const enabled = !!env?.id;
  const { data, isLoading, isError, error, refetch, isFetching } = useQuery({
    queryKey: ['environment-host-log-preview', env?.id, source],
    queryFn: () =>
      api
        .get<HostLogResult>('/system/os-logs', {
          env: env.id,
          source,
          tail: String(previewTail),
        })
        .then(assertSuccess),
    enabled,
    refetchOnWindowFocus: false,
    staleTime: 30_000,
  });

  useEffect(() => {
    if (!enabled) return;
    void refetch();
  }, [enabled, env?.id, refetch]);

  const sourceBadge = useMemo(
    () => [
      { value: 'system', label: t('detail.hostLogs.sources.system') },
      { value: 'kernel', label: t('detail.hostLogs.sources.kernel') },
      { value: 'auth', label: t('detail.hostLogs.sources.auth') },
      { value: 'docker', label: t('detail.hostLogs.sources.docker') },
    ],
    [t],
  );

  const previewLines = useMemo(() => {
    if (!data) return [];
    // The backend strips every MCHARBOR_NOTICE marker out of `lines` before
    // responding, so what arrives here is log content already.
    return data.lines.slice(-previewTail);
  }, [data, previewTail]);

  // `notices` carries warnings only now - the discovery output that used to be
  // mixed in here arrives separately and is shown in full on the Logs tab.
  const notices = useMemo(() => data?.notices ?? [], [data]);

  const sourceTabs = sourceBadge.map((tab) => (
    <Button
      key={tab.value}
      type="button"
      variant={source === tab.value ? 'default' : 'ghost'}
      size="sm"
      onClick={() => setSource(tab.value as LogSource)}
      className="rounded-md px-2 py-1 text-xs font-medium tracking-wider uppercase"
      aria-pressed={source === tab.value}
    >
      {tab.label}
    </Button>
  ));

  return (
    <Card>
      <CardHeader className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div>
          <CardTitle className="flex items-center gap-2 text-base">
            <IconFileText className="size-5 text-muted-foreground" />
            {t('detail.hostLogPreview.title')}
          </CardTitle>
          <CardDescription>{t('detail.hostLogPreview.description')}</CardDescription>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <div className="flex items-center gap-1 rounded-lg border border-border bg-muted/30 p-1">{sourceTabs}</div>
          <Button variant="outline" size="sm" onClick={() => void refetch()} disabled={isFetching}>
            <IconRefresh className="size-4" />
            {t('detail.hostLogs.refresh')}
          </Button>
          <Button variant="ghost" size="sm" onClick={() => navigate(`/environments/${env.id}?tab=logs`)}>
            {t('detail.hostLogPreview.openFull')}
            <IconChevronRight className="ml-1 size-4" />
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {notices.length > 0 && (
          <div className="space-y-2">
            {notices.map((notice) => (
              <div
                key={notice}
                className="rounded-lg border border-warning/30 bg-warning/10 px-3 py-2 text-sm text-warning"
              >
                {hostLogNoticeText(t, notice)}
              </div>
            ))}
          </div>
        )}
        {isLoading && (
          <div className="flex h-32 items-center justify-center text-xs text-muted-foreground">
            {t('detail.hostLogPreview.loading')}
          </div>
        )}
        {!isLoading && isError && (
          <div className="flex h-32 items-center justify-center rounded-lg border border-border bg-muted/20 px-4 text-center text-sm text-muted-foreground">
            {[t('detail.hostLogs.unavailable'), error?.message].filter(Boolean).join(' — ')}
          </div>
        )}
        {!isLoading && !isError && (
          <>
            <LogViewer
              colorizeSeverity
              lines={previewLines}
              emptyMessage={t('detail.hostLogPreview.empty')}
              className="h-[260px]"
            />
            <div className="flex items-center justify-between text-xs text-muted-foreground">
              <span>
                <Badge variant="secondary">
                  {t('detail.hostLogPreview.lineCount', { count: previewLines.length })}
                </Badge>
              </span>
              <span>{t('detail.hostLogPreview.tailNote', { tail: previewTail })}</span>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}
