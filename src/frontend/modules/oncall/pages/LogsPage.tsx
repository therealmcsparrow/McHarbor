// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { Link, useParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconArrowLeft, IconPlayerPause, IconPlayerPlay } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Spinner } from '@resources/components/ui/Spinner';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useContainerLogs } from '@resources/hooks/useContainerLogs';
import { QueryState } from '../components/QueryState';

const TAIL = 300;

export function filterLines(text: string, search: string): string[] {
  const lines = text.split('\n').filter((line) => line !== '');
  const needle = search.trim().toLowerCase();
  return needle ? lines.filter((line) => line.toLowerCase().includes(needle)) : lines;
}

function LogView({ containerId, follow, search }: { containerId: string; follow: boolean; search: string }) {
  const { t } = useTranslation('oncall');
  const logs = useContainerLogs(containerId, TAIL, follow);
  const lines = useMemo(() => filterLines(logs.data ?? '', search), [logs.data, search]);
  return (
    <QueryState
      isLoading={logs.isLoading}
      error={logs.error}
      onRetry={() => void logs.refetch()}
      isEmpty={lines.length === 0}
      emptyText={search ? t('logs.noMatches') : t('logs.empty')}
    >
      {logs.connectionState === 'reconnecting' && (
        <output className="mb-2 block text-xs text-muted-foreground">{t('logs.reconnecting')}</output>
      )}
      <pre className="min-h-0 flex-1 overflow-auto rounded-md bg-muted/40 p-2 font-mono text-xs leading-relaxed break-all whitespace-pre-wrap">
        {lines.join('\n')}
      </pre>
    </QueryState>
  );
}

export default function LogsPage() {
  const { t } = useTranslation('oncall');
  const { envId = '', containerId = '' } = useParams();
  const canRead = usePermission(PERM.LOGS_VIEW);
  const currentEnv = useEnvironmentStore((s) => s.currentId);
  const setCurrentEnv = useEnvironmentStore((s) => s.setCurrentId);
  const [follow, setFollow] = useState(true);
  const [search, setSearch] = useState('');

  useEffect(() => {
    if (envId && currentEnv !== envId) setCurrentEnv(envId);
  }, [envId, currentEnv, setCurrentEnv]);

  const back = `/oncall/containers/${encodeURIComponent(envId)}/${encodeURIComponent(containerId)}`;
  return (
    <section aria-labelledby="oncall-logs-title" className="flex min-h-0 flex-1 flex-col gap-3">
      <div className="flex items-center gap-2">
        <Button asChild variant="ghost" size="icon" className="size-11" aria-label={t('container.back')}>
          <Link to={back}>
            <IconArrowLeft className="size-5" aria-hidden="true" />
          </Link>
        </Button>
        <h1 id="oncall-logs-title" className="flex-1 text-lg font-semibold">
          {t('logs.title')}
        </h1>
        <Button variant="outline" className="min-h-11" aria-pressed={follow} onClick={() => setFollow((v) => !v)}>
          {follow ? (
            <IconPlayerPause className="size-4" aria-hidden="true" />
          ) : (
            <IconPlayerPlay className="size-4" aria-hidden="true" />
          )}
          {follow ? t('logs.pause') : t('logs.follow')}
        </Button>
      </div>
      <Input
        type="search"
        value={search}
        onChange={(evt) => setSearch(evt.target.value)}
        placeholder={t('logs.search')}
        aria-label={t('logs.search')}
        className="min-h-11"
      />
      {!canRead ? (
        <p className="py-8 text-center text-sm text-muted-foreground">
          {t('state.noAccess', { permission: PERM.LOGS_VIEW })}
        </p>
      ) : currentEnv !== envId ? (
        <Spinner size="lg" className="mx-auto" />
      ) : (
        <LogView containerId={containerId} follow={follow} search={search} />
      )}
    </section>
  );
}
