// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useState } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconLock, IconPlus, IconTerminal2 } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { TablePage } from '@resources/layout/TablePage';
import type { SearchMode } from '@resources/utils/search-filter';
import {
  useAPIKeys,
  useDeleteAPIKey,
  useRevokeAPIKey,
  type APIKey,
  type CreateAPIKeyResult,
} from '../hooks/useAPIKeys';
import { APIKeyFormDialog } from './APIKeyFormDialog';
import { APIKeyTokenDialog } from './APIKeyTokenDialog';
import { CLIDownloadDialog } from './CLIDownloadDialog';
import { useAPIKeyColumns } from './apiKeyColumns';
import { PERM } from '@core/auth/permissionKeys';
import { useAnyPermission } from '@core/auth/usePermission';
import { RotateAPIKeyDialog } from './governance/RotateAPIKeyDialog';

type PendingAction = { id: string; mode: 'revoke' | 'delete' };

export function APIKeysTab() {
  const { t } = useTranslation('security');
  const navigate = useNavigate();
  const { data: keys = [], isLoading, isFetching, refetch } = useAPIKeys();
  const [formOpen, setFormOpen] = useState(false);
  const [cliOpen, setCliOpen] = useState(false);
  const [editing, setEditing] = useState<APIKey | null>(null);
  const [pending, setPending] = useState<PendingAction | null>(null);
  const [createdKey, setCreatedKey] = useState<CreateAPIKeyResult | null>(null);
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const revokeKey = useRevokeAPIKey();
  const deleteKey = useDeleteAPIKey();
  const [rotating, setRotating] = useState<APIKey | null>(null);
  const canSee = useAnyPermission([PERM.API_KEYS_VIEW, PERM.API_KEYS_MANAGE]);

  const onView = useCallback((key: APIKey) => navigate(`/security/api-keys/${key.id}`), [navigate]);
  const onEdit = useCallback((key: APIKey) => {
    setEditing(key);
    setFormOpen(true);
  }, []);
  const onRevoke = useCallback((key: APIKey) => setPending({ id: key.id, mode: 'revoke' }), []);
  const onDelete = useCallback((key: APIKey) => setPending({ id: key.id, mode: 'delete' }), []);

  const columns = useAPIKeyColumns({ onView, onEdit, onRevoke, onDelete, onRotate: setRotating });

  if (!canSee) {
    return (
      <EmptyState
        icon={<IconLock className="size-6" />}
        title={t('apiKeys.title')}
        description={t('apiKeys.noAccess')}
      />
    );
  }

  return (
    <>
      <TablePage
        id="api-keys"
        // Rendered inside the security settings tabs.
        header={false}
        title={t('apiKeys.description')}
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('apiKeys.name'),
          // Deliberately excludes the key prefix: an API key identifier is not
          // something to make searchable, and it is never exported either.
          fields: (key) => [key.name, key.username],
        }}
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void refetch(), isFetching },
          // No export here. Every column that is not key material is already
          // visible, and an export of an API-key table is a file of credentials
          // waiting to be mishandled.
          actions: (
            <>
              <Button variant="outline" size="sm" onClick={() => setCliOpen(true)}>
                <IconTerminal2 className="mr-1 size-3.5" />
                {t('cli.button')}
              </Button>
              <Button
                permission={PERM.API_KEYS_MANAGE}
                size="sm"
                onClick={() => {
                  setEditing(null);
                  setFormOpen(true);
                }}
              >
                <IconPlus className="mr-1 size-3.5" />
                {t('apiKeys.createKey')}
              </Button>
            </>
          ),
        }}
        columns={columns}
        data={keys}
        loading={isLoading}
        empty={{ title: t('apiKeys.noKeys') }}
        getRowId={(key) => key.id}
      />

      <CLIDownloadDialog open={cliOpen} onOpenChange={setCliOpen} />

      <APIKeyFormDialog
        open={formOpen}
        onOpenChange={(openState) => {
          setFormOpen(openState);
          if (!openState) setEditing(null);
        }}
        existing={editing}
        onCreated={(result) => {
          setFormOpen(false);
          setEditing(null);
          setCreatedKey(result);
        }}
      />

      {createdKey && (
        <APIKeyTokenDialog open={!!createdKey} onOpenChange={() => setCreatedKey(null)} keyResult={createdKey} />
      )}

      <RotateAPIKeyDialog
        apiKey={rotating}
        onOpenChange={(isOpen) => !isOpen && setRotating(null)}
        onRotated={setCreatedKey}
      />

      <ConfirmDialog
        open={!!pending}
        onOpenChange={() => setPending(null)}
        title={pending?.mode === 'delete' ? t('apiKeys.deleteKey') : t('apiKeys.revokeKey')}
        description={pending?.mode === 'delete' ? t('apiKeys.deleteKeyConfirm') : t('apiKeys.revokeKeyConfirm')}
        onConfirm={() => {
          if (!pending) return;
          const mutation = pending.mode === 'delete' ? deleteKey : revokeKey;
          mutation.mutate(pending.id, { onSuccess: () => setPending(null) });
        }}
        loading={revokeKey.isPending || deleteKey.isPending}
      />
    </>
  );
}
