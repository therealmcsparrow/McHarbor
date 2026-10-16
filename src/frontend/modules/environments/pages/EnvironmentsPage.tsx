// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconLayoutGrid, IconLayoutList, IconPlus } from '@tabler/icons-react';
import { TablePage } from '@resources/layout/TablePage';
import { matchesSearchFields, type SearchMode } from '@resources/utils/search-filter';
import { Button } from '@resources/components/ui/Button';
import { EnvironmentCardGrid } from '../components/EnvironmentCardGrid';
import { useEnvironmentColumns } from '../components/EnvironmentsColumns';
import { useVersionInfo } from '../hooks/useEnvironments';
import {
  useEnvironmentList,
  useTestEnvironment,
  useRemoveEnvironment,
  useCreateInstallToken,
  type EnvironmentListItem,
} from '../hooks/useEnvironmentActions';
import type { AgentTokenData } from './environments-page-types';
import { EnvironmentsPageDialogs } from './EnvironmentsPageDialogs';
import { useEnvironmentsViewStore } from '../stores/environments-view';
import { PERM } from '@core/auth/permissionKeys';

export default function EnvironmentsPage() {
  const { t } = useTranslation('environments');
  const { data: environments = [], isLoading, isFetching, refetch } = useEnvironmentList();
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const { data: versionInfo } = useVersionInfo();
  const testEnv = useTestEnvironment();
  const removeEnv = useRemoveEnvironment();
  const createInstallToken = useCreateInstallToken();
  const { viewMode, setViewMode } = useEnvironmentsViewStore();

  const [createOpen, setCreateOpen] = useState(false);
  const [confirmTarget, setConfirmTarget] = useState<string | null>(null);
  const [agentTokenData, setAgentTokenData] = useState<AgentTokenData | null>(null);
  const [testTarget, setTestTarget] = useState<{ id: string; name: string } | null>(null);

  const columns = useEnvironmentColumns({
    onTest: (id) => {
      const env = environments.find((entry) => entry.id === id);
      setTestTarget({ id, name: env?.name ?? id });
      testEnv.mutate(id);
    },
    onInstall: (id) => {
      createInstallToken.mutate(id, {
        onSuccess: (data) => {
          if (data.agentToken) {
            setAgentTokenData({ token: data.agentToken, installScript: data, mode: 'installInfo' });
          }
        },
      });
    },
    onRemove: (id) => setConfirmTarget(id),
  });

  const handleShowInstallInfo = (id: string) => {
    createInstallToken.mutate(id, {
      onSuccess: (data) => {
        if (data.agentToken) {
          setAgentTokenData({ token: data.agentToken, installScript: data, mode: 'installInfo' });
        }
      },
    });
  };

  const searchFields = (env: EnvironmentListItem) => [
    env.name,
    env.connectionType,
    env.host,
    env.dockerVersion,
    env.agentStatus,
    env.agentHostname,
  ];
  const cardEnvironments = query.trim()
    ? environments.filter((env) => matchesSearchFields(query, searchMode, searchFields(env)).matched)
    : environments;

  const handleTestFromCard = (id: string) => {
    const env = environments.find((entry) => entry.id === id);
    setTestTarget({ id, name: env?.name ?? id });
    testEnv.mutate(id);
  };

  return (
    <>
      <TablePage
        id="environments"
        title={t('title')}
        description={t('description')}
        actions={
          <>
            <Button permission={PERM.ENVIRONMENTS_MANAGE} onClick={() => setCreateOpen(true)}>
              <IconPlus className="h-4 w-4" /> {t('addEnvironment')}
            </Button>
            <div className="h-6 w-px bg-border" />
            <div className="flex items-center rounded-lg border border-border">
              <Button
                variant={viewMode === 'table' ? 'default' : 'ghost'}
                size="icon-sm"
                onClick={() => setViewMode('table')}
                aria-label={t('tableView')}
              >
                <IconLayoutList className="h-4 w-4" />
              </Button>
              <Button
                variant={viewMode === 'card' ? 'default' : 'ghost'}
                size="icon-sm"
                onClick={() => setViewMode('card')}
                aria-label={t('cardView')}
              >
                <IconLayoutGrid className="h-4 w-4" />
              </Button>
            </div>
          </>
        }
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('searchPlaceholder'),
          // Deliberately not socketPath — a socket path is not something to
          // surface through a search box.
          fields: searchFields,
        }}
        toolbar={{
          columns: viewMode === 'table',
          density: viewMode === 'table',
          refresh: { onRefresh: () => void refetch(), isFetching },
          export: { filename: 'mcharbor-environments' },
        }}
        columns={columns}
        data={environments}
        loading={isLoading}
        empty={{ title: t('emptyMessage') }}
        getRowId={(env) => env.id}
        body={
          viewMode === 'table' ? undefined : (
            <div className="min-h-0 flex-1 overflow-auto" data-scroll-exempt>
              <EnvironmentCardGrid
                environments={cardEnvironments}
                hasEnvironments={environments.length > 0}
                isLoading={isLoading}
                mcharborVersion={versionInfo?.mcharbor.version}
                onTest={handleTestFromCard}
                onInstall={handleShowInstallInfo}
                onRemove={setConfirmTarget}
              />
            </div>
          )
        }
      />

      <EnvironmentsPageDialogs
        createOpen={createOpen}
        setCreateOpen={setCreateOpen}
        setAgentTokenData={setAgentTokenData}
        confirmTarget={confirmTarget}
        setConfirmTarget={setConfirmTarget}
        onConfirmRemove={(id) => removeEnv.mutate(id)}
        removePending={removeEnv.isPending}
        agentTokenData={agentTokenData}
        testTarget={testTarget}
        setTestTarget={setTestTarget}
        testResult={testEnv.data}
        testPending={testEnv.isPending}
      />
    </>
  );
}
