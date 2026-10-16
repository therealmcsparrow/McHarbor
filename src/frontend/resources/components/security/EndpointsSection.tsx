// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// EndpointsSection — the SCIM endpoint list.
//
// Converted from raw `<Table>` primitives to `TablePage`, for the same
// reason `PoliciesSection` was: it sat next to Users and API Keys in the
// security Management group with none of their chrome and a second
// hand-rolled empty state. `header={false}` because the hero card above
// already names the section.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus, IconKey } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { TablePage } from '@resources/layout/TablePage';
import type { SearchMode } from '@resources/utils/search-filter';
import {
  useEndpoints,
  useCreateEndpoint,
  useUpdateEndpoint,
  type Endpoint,
  type EndpointInput,
} from '@resources/hooks/useScimEndpoints';
import { EndpointEditorDialog } from '@resources/components/security/scim/EndpointEditorDialog';
import { TokenRevealDialog } from '@resources/components/security/scim/TokenRevealDialog';
import { SyncPreviewDialog } from '@resources/components/security/scim/SyncPreviewDialog';
import { SyncHistoryDialog } from '@resources/components/security/scim/SyncHistoryDialog';
import { RotateTokenDialog } from '@resources/components/security/scim/RotateTokenDialog';
import { DeleteEndpointDialog } from '@resources/components/security/scim/DeleteEndpointDialog';
import { useScimEndpointColumns } from '@resources/components/security/scim/useScimEndpointColumns';
import { PERM } from '@core/auth/permissionKeys';

export function EndpointsSection() {
  const { t } = useTranslation('scim');
  const { data, isLoading, isFetching, refetch } = useEndpoints();
  const create = useCreateEndpoint();
  const update = useUpdateEndpoint();

  const [editorOpen, setEditorOpen] = useState(false);
  const [editing, setEditing] = useState<Endpoint | null>(null);
  const [pendingDelete, setPendingDelete] = useState<Endpoint | null>(null);
  const [previewFor, setPreviewFor] = useState<Endpoint | null>(null);
  const [historyFor, setHistoryFor] = useState<Endpoint | null>(null);
  const [rotateFor, setRotateFor] = useState<Endpoint | null>(null);
  const [revealedToken, setRevealedToken] = useState<{ name: string; token: string } | null>(null);
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');

  const openCreate = () => {
    setEditing(null);
    setEditorOpen(true);
  };

  const actions = useMemo(
    () => ({
      onPreview: setPreviewFor,
      onHistory: setHistoryFor,
      onRotate: setRotateFor,
      onEdit: (endpoint: Endpoint) => {
        setEditing(endpoint);
        setEditorOpen(true);
      },
      onDelete: setPendingDelete,
    }),
    [],
  );
  const columns = useScimEndpointColumns(actions);

  const handleSubmit = (input: EndpointInput) => {
    if (editing) {
      update.mutate({ id: editing.id, ...input }, { onSuccess: () => setEditorOpen(false) });
      return;
    }
    create.mutate(input, {
      onSuccess: (resp) => {
        setEditorOpen(false);
        if (resp.bearerToken) {
          setRevealedToken({ name: resp.endpoint.name, token: resp.bearerToken });
        }
      },
    });
  };

  return (
    <>
      <TablePage<Endpoint>
        id="scim-endpoints"
        header={false}
        title={t('title')}
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('table.name'),
          fields: (ep) => [ep.name, ep.baseUrl],
        }}
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void refetch(), isFetching },
          actions: (
            <Button permission={PERM.SCIM_MANAGE} size="sm" onClick={openCreate}>
              <IconPlus className="mr-1 size-3.5" />
              {t('actions.new')}
            </Button>
          ),
        }}
        columns={columns}
        data={data ?? []}
        loading={isLoading}
        empty={{
          icon: <IconKey className="size-10 text-muted-foreground" />,
          title: t('empty.title'),
          description: t('empty.description'),
        }}
        getRowId={(ep) => ep.id}
      />

      <EndpointEditorDialog
        open={editorOpen}
        onOpenChange={setEditorOpen}
        initial={editing}
        onSubmit={handleSubmit}
        isSubmitting={create.isPending || update.isPending}
      />
      <DeleteEndpointDialog endpoint={pendingDelete} onOpenChange={(isOpen) => !isOpen && setPendingDelete(null)} />
      <SyncPreviewDialog endpoint={previewFor} onOpenChange={(isOpen) => !isOpen && setPreviewFor(null)} />
      <SyncHistoryDialog endpoint={historyFor} onClose={() => setHistoryFor(null)} />
      <RotateTokenDialog
        endpoint={rotateFor}
        onOpenChange={(isOpen) => !isOpen && setRotateFor(null)}
        onSuccess={(result) => setRevealedToken({ name: result.endpoint.name, token: result.bearerToken })}
      />
      <TokenRevealDialog
        open={!!revealedToken}
        onOpenChange={(isOpen) => !isOpen && setRevealedToken(null)}
        endpointName={revealedToken?.name ?? ''}
        token={revealedToken?.token ?? ''}
      />
    </>
  );
}
