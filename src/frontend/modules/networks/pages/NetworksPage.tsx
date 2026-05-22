// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconLayoutGrid, IconLayoutList, IconPlus } from '@tabler/icons-react';
import { DataGrid } from '@resources/components/DataGrid';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Button } from '@resources/components/ui/Button';
import { PageHeader } from '@resources/layout/PageHeader';
import { NetworkCardGrid } from '../components/NetworkCardGrid';
import { NetworkCreateDialog } from '../components/NetworkCreateDialog';
import { useNetworks } from '../hooks/useNetworks';
import { useNetworksPageConfig } from '../hooks/useNetworksPageConfig';
import { useRemoveNetwork } from '../hooks/useNetworks';
import { useNetworksViewStore } from '../stores/networks-view';

export default function NetworksPage() {
  const navigate = useNavigate();
  const { t } = useTranslation('networks');
  const { data: networks = [], isLoading } = useNetworks();
  const removeNetwork = useRemoveNetwork();
  const { viewMode, setViewMode } = useNetworksViewStore();
  const [createOpen, setCreateOpen] = useState(false);
  const [confirmTarget, setConfirmTarget] = useState<string | null>(null);
  const { columns, batchActions } = useNetworksPageConfig(setConfirmTarget);

  return (
    <div className="space-y-6">
      <PageHeader
        title={t('title')}
        description={t('description', { count: networks.length })}
        actions={
          <>
            <Button onClick={() => setCreateOpen(true)}>
              <IconPlus className="h-4 w-4" /> {t('create.title')}
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
      />

      {viewMode === 'table' ? (
        <DataGrid
          data={networks}
          columns={columns}
          searchKey="Name"
          searchPlaceholder={t('searchPlaceholder')}
          loading={isLoading}
          emptyMessage={t('emptyMessage')}
          onRowClick={(row) => navigate(`/networks/${row.Id}`)}
          tableFixed
          selectable
          batchActions={batchActions}
          getRowId={(row) => row.Id}
        />
      ) : (
        <NetworkCardGrid
          networks={networks}
          isLoading={isLoading}
          onClick={(network) => navigate(`/networks/${network.Id}`)}
          onRemove={setConfirmTarget}
        />
      )}

      <NetworkCreateDialog open={createOpen} onOpenChange={setCreateOpen} />

      <ConfirmDialog
        open={confirmTarget !== null}
        onOpenChange={(open) => !open && setConfirmTarget(null)}
        title={t('confirm.removeTitle')}
        description={t('confirm.removeDescription')}
        onConfirm={() => {
          if (confirmTarget) {
            removeNetwork.mutate(confirmTarget);
          }
          setConfirmTarget(null);
        }}
        loading={removeNetwork.isPending}
      />
    </div>
  );
}
