// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// IdentityTab — the identity provider list.
//
// Was a stack of `ProviderCard`s under a hand-rolled header and empty
// state. A provider row carries exactly the fields a table column carries —
// brand mark, name, type, enabled — so it is now a `TablePage` like the
// rest of the security Management group, and the per-row controls moved to
// `IdentityProviderActions`.

import { useCallback, useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconPlus, IconFingerprint } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { TablePage } from '@resources/layout/TablePage';
import type { SearchMode } from '@resources/utils/search-filter';
import { useIdentityAdapters } from '@resources/hooks/useIdentityAdapters';
import { useIdentityProviders, type IdentityProvider as IdentityProviderValue } from '../hooks/useIdentityProviders';
import { useGroups } from '../hooks/useGroups';
import { useIdentityProviderColumns, useProviderTypeLabel } from './IdentityProviderColumns';
import { CreateProviderDialog } from './CreateProviderDialog';
import { EditProviderDialog } from './EditProviderDialog';
import { PERM } from '@core/auth/permissionKeys';

export function IdentityTab() {
  const { t } = useTranslation('security');
  const navigate = useNavigate();
  const { data: providers, isLoading, isFetching, refetch } = useIdentityProviders();
  // Adapter catalog for the per-row brand icon and for the "no plugin
  // installed" empty state below. Already in the query cache from
  // CreateProviderDialog / EditProviderDialog in the common flow.
  const { data: adapters, isLoading: adaptersLoading } = useIdentityAdapters();
  const { data: groups } = useGroups();
  const groupNameById = useMemo(() => new Map((groups ?? []).map((g) => [g.id, g.name])), [groups]);
  const [createOpen, setCreateOpen] = useState(false);
  const [editProvider, setEditProvider] = useState<IdentityProviderValue | null>(null);
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');

  const typeLabel = useProviderTypeLabel();
  const handleEdit = useCallback((provider: IdentityProviderValue) => setEditProvider(provider), []);
  const columns = useIdentityProviderColumns({
    adapters,
    typeLabel,
    onEdit: handleEdit,
    groupNameById,
  });

  // No plugin installed and nothing configured: the Add button would
  // open an empty picker, so render the same store-pointing EmptyState
  // as the AI / Communications / Logging tabs instead of the table.
  // Providers configured before their plugin was removed still list,
  // so an operator can see and clean up what stopped.
  const nothingInstalled =
    !isLoading && !adaptersLoading && Object.keys(adapters ?? {}).length === 0 && (providers ?? []).length === 0;

  if (nothingInstalled) {
    return (
      <EmptyState
        icon={<IconFingerprint className="size-6" />}
        title={t('identity.noPluginsInstalledTitle')}
        description={t('identity.noPluginsInstalledDescription')}
        primaryAction={{
          label: t('identity.openAppStore'),
          onClick: () => navigate('/store?tab=plugins'),
        }}
      />
    );
  }

  return (
    <>
      <TablePage<IdentityProviderValue>
        id="identityProviders"
        // Rendered inside the security tabs, which already carry the title.
        header={false}
        title={t('identity.title')}
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('identity.name'),
          fields: (provider) => [provider.name, typeLabel(provider.providerType)],
        }}
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void refetch(), isFetching },
          actions: (
            <Button permission={PERM.IDENTITY_PROVIDERS_MANAGE} size="sm" onClick={() => setCreateOpen(true)}>
              <IconPlus className="mr-1 size-3.5" />
              {t('identity.addProvider')}
            </Button>
          ),
        }}
        columns={columns}
        data={providers ?? []}
        loading={isLoading}
        empty={{
          icon: <IconFingerprint className="size-10 text-muted-foreground" />,
          title: t('identity.noProviders'),
          description: t('identity.noProvidersDescription'),
        }}
        getRowId={(provider) => provider.id}
      />

      <CreateProviderDialog open={createOpen} onOpenChange={setCreateOpen} />

      {editProvider && (
        <EditProviderDialog
          open={!!editProvider}
          onOpenChange={(isOpen) => {
            if (!isOpen) setEditProvider(null);
          }}
          provider={editProvider}
        />
      )}
    </>
  );
}
