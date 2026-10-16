// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCloudDownload, IconRefresh } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { StatusError } from '@resources/components/ui/StatusError';
import { TablePage } from '@resources/layout/TablePage';
import { formatDate } from '@resources/utils/format';
import type { SearchMode } from '@resources/utils/search-filter';
import {
  useAgentReleases,
  useCheckAgentReleases,
  usePullAgentRelease,
  useSetAgentTarget,
  releaseLabel,
  type AgentRelease,
  type AgentReleaseBuild,
} from '../hooks/useAgentReleases';
import { useAgentReleaseColumns } from './AgentReleaseColumns';
import { AgentReleaseDeleteDialog } from './AgentReleaseDeleteDialog';
import { AgentReleasePullPicker } from './AgentReleasePullPicker';
import { storedCovers } from './agent-release-display';

function rowKey(r: AgentRelease): string {
  return r.build ? `${r.version}-${r.build}` : r.version;
}

export function AgentReleasesCard() {
  const { t } = useTranslation('settings');
  const { data, isPending, isFetching, error, refetch } = useAgentReleases();
  const check = useCheckAgentReleases();
  const pull = usePullAgentRelease();
  const setTarget = useSetAgentTarget();
  const [deleting, setDeleting] = useState<AgentReleaseBuild | null>(null);
  const [query, setQuery] = useState('');
  const [mode, setMode] = useState<SearchMode>('contains');

  const releases = useMemo(() => data?.releases ?? [], [data]);
  const state = data?.state;
  const latest = state?.latestVersion ?? '';
  const latestBuild = state?.latestBuild ?? '';
  const latestStored = storedCovers(releases, latest, latestBuild);

  const onSetTarget = useCallback(
    (release: AgentRelease) => setTarget.mutate({ version: release.version, build: release.build }),
    [setTarget],
  );
  const onDelete = useCallback(
    (release: AgentRelease) => setDeleting({ version: release.version, build: release.build }),
    [],
  );
  const columns = useAgentReleaseColumns({
    settingTarget: setTarget.isPending,
    onSetTarget,
    onDelete,
  });

  return (
    <div className="mc-glass rounded-lg p-6">
      <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="text-sm font-medium text-foreground">{t('agentReleases.title')}</h3>
          <p className="mt-0.5 text-xs text-muted-foreground">{t('agentReleases.subtitle')}</p>
          <p className="mt-1 text-xs text-muted-foreground">
            {data?.target
              ? t('agentReleases.targetIs', {
                  version: releaseLabel({ version: data.target, build: data.targetBuild }),
                })
              : t('agentReleases.noTarget')}
          </p>
        </div>
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <span>
            {latest ? t('agentReleases.latest', { version: latest }) : t('agentReleases.latestUnknown')}
            {state?.lastCheckedAt
              ? ` · ${t('agentReleases.lastChecked', { date: formatDate(state.lastCheckedAt) })}`
              : ''}
          </span>
          <StatusError error={state?.lastError} title={t('agentReleases.checkFailed')} />
        </div>
      </div>

      <TablePage
        id="settings-agent-releases"
        header={false}
        title={t('agentReleases.title')}
        search={{
          value: query,
          onChange: setQuery,
          mode,
          onModeChange: setMode,
          placeholder: t('agentReleases.searchPlaceholder'),
          fields: (r) => [r.version, r.build, r.status, r.sourceUrl, r.imageRef, r.pulledBy],
        }}
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void refetch(), isFetching },
          export: { filename: 'mcharbor-agent-releases' },
          actions: (
            <div className="flex items-center gap-2">
              <Button
                size="sm"
                variant="outline"
                permission={PERM.UPDATES_MANAGE}
                disabled={check.isPending}
                onClick={() => check.mutate()}
              >
                {check.isPending ? (
                  <Spinner className="mr-1.5 h-4 w-4" />
                ) : (
                  <IconRefresh className="mr-1.5 h-4 w-4" aria-hidden="true" />
                )}
                {t('agentReleases.checkNow')}
              </Button>
              {latest && !latestStored ? (
                <Button
                  size="sm"
                  permission={PERM.UPDATES_MANAGE}
                  disabled={pull.isPending}
                  onClick={() => pull.mutate({ version: latest, build: latestBuild || undefined })}
                >
                  {pull.isPending ? (
                    <Spinner className="mr-1.5 h-4 w-4" />
                  ) : (
                    <IconCloudDownload className="mr-1.5 h-4 w-4" aria-hidden="true" />
                  )}
                  {t('agentReleases.download', {
                    version: releaseLabel({ version: latest, build: latestBuild }),
                  })}
                </Button>
              ) : null}
              <AgentReleasePullPicker
                available={state?.available ?? []}
                releases={releases}
                latest={latest}
                latestBuild={latestBuild}
                pending={pull.isPending}
                onPull={(version, build) => pull.mutate({ version, build })}
              />
            </div>
          ),
        }}
        columns={columns}
        data={releases}
        loading={isPending}
        error={error ?? undefined}
        onRetry={() => void refetch()}
        empty={{ title: t('agentReleases.empty') }}
        getRowId={rowKey}
        getRowTestId={(r) => `agent-release-${rowKey(r)}`}
      />

      <AgentReleaseDeleteDialog release={deleting} onOpenChange={(nextOpen) => !nextOpen && setDeleting(null)} />
    </div>
  );
}
