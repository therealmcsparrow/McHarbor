// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus } from '@tabler/icons-react';
import { TablePage } from '@resources/layout/TablePage';
import { matchesSearchFields, type SearchMode } from '@resources/utils/search-filter';
import { Button } from '@resources/components/ui/Button';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { COMPUTE_PERM } from '../permissions';
import { useDeleteNetwork, useNetworks } from '../hooks/useCompute';
import { useHostEnvironment } from '../hooks/useHostEnvironment';
import { useComputeView } from '../hooks/useComputeView';
import { CardGrid } from '../components/CardGrid';
import { NetworkCard } from '../components/NetworkCard';
import { useNetworkColumns } from '../components/NetworkColumns';
import { ViewToggle } from '../components/ViewToggle';
import { CreateNetworkDialog } from '../components/CreateNetworkDialog';
import { LeaseViewer } from '../components/LeaseViewer';
import type { Network } from '../types';

/** Guest networks across every host. */
export function NetworksPage({ environments }: { environments: { id: string; name: string }[] }) {
  const { t } = useTranslation('compute');
  // Scoped to the host's topbar selection, like every other page in this
  // plugin. An empty selection means "no host chosen yet" and shows all.
  const environmentId = useHostEnvironment();
  const { data, isLoading, isFetching, isError, refetch } = useNetworks(environmentId || undefined);
  const remove = useDeleteNetwork();
  const { viewMode, setViewMode } = useComputeView('networks');

  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Network | null>(null);
  const [deleting, setDeleting] = useState<Network | null>(null);
  const [viewingLeases, setViewingLeases] = useState<Network | null>(null);

  const hostName = useCallback(
    (envId: string) => environments.find((env) => env.id === envId)?.name ?? envId,
    [environments],
  );

  const networks = useMemo(() => data ?? [], [data]);

  const searchFields = useCallback(
    (network: Network) => [
      network.name,
      hostName(network.environmentId),
      network.bridge,
      network.subnetCidr,
      t(`networks.mode.${network.mode}`),
    ],
    [hostName, t],
  );

  const cardNetworks = useMemo(() => {
    if (!query.trim()) return networks;
    return networks.filter((network) => matchesSearchFields(query, searchMode, searchFields(network)).matched);
  }, [networks, query, searchMode, searchFields]);

  const columns = useNetworkColumns({
    hostName,
    onViewLeases: setViewingLeases,
    onEdit: setEditing,
    onDelete: setDeleting,
  });

  if (isError) {
    return <ErrorState onRetry={() => void refetch()} />;
  }

  return (
    <>
      <TablePage
        id="compute-networks"
        title={t('networks.title')}
        description={t('networks.subtitle')}
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('networks.searchPlaceholder'),
          fields: searchFields,
        }}
        actions={<ViewToggle viewMode={viewMode} setViewMode={setViewMode} />}
        toolbar={{
          actions: (
            <Button size="sm" permission={COMPUTE_PERM.MANAGE} onClick={() => setCreating(true)}>
              <IconPlus className="h-4 w-4" /> {t('actions.createNetwork')}
            </Button>
          ),
          columns: viewMode === 'table',
          density: viewMode === 'table',
          refresh: { onRefresh: () => void refetch(), isFetching },
          export: { filename: 'mcharbor-compute-networks' },
        }}
        columns={columns}
        data={networks}
        loading={isLoading}
        empty={{ title: t('networks.empty.title'), description: t('networks.empty.body') }}
        getRowId={(network) => network.id}
        body={
          viewMode === 'table' ? undefined : (
            <div className="min-h-0 flex-1 overflow-auto" data-scroll-exempt>
              <CardGrid
                count={cardNetworks.length}
                isLoading={isLoading}
                filtered={networks.length > 0}
                emptyLabel={t('networks.empty.title')}
              >
                {cardNetworks.map((network) => (
                  <NetworkCard
                    key={network.id}
                    network={network}
                    hostName={hostName(network.environmentId)}
                    onViewLeases={setViewingLeases}
                    onEdit={setEditing}
                    onDelete={setDeleting}
                  />
                ))}
              </CardGrid>
            </div>
          )
        }
      />

      {creating && <CreateNetworkDialog environments={environments} onClose={() => setCreating(false)} />}

      {editing && (
        <CreateNetworkDialog environments={environments} network={editing} onClose={() => setEditing(null)} />
      )}

      {deleting && (
        <ConfirmDialog
          open
          onOpenChange={(isOpen) => !isOpen && setDeleting(null)}
          title={t('networks.delete.title', { name: deleting.name })}
          description={t(`networks.delete.${deleting.mode}Body`)}
          confirmLabel={t('networks.delete.submit')}
          variant="destructive"
          onConfirm={() => {
            remove.mutate(deleting.id, { onSuccess: () => setDeleting(null) });
          }}
        />
      )}

      {viewingLeases && <LeaseViewer network={viewingLeases} onClose={() => setViewingLeases(null)} />}
    </>
  );
}
