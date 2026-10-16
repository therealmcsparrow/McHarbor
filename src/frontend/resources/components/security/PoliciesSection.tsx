// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// PoliciesSection — the RBAC/ABAC policy list.
//
// This used to draw its own table out of the `<Table>` primitives inside an
// `mc-glass` box, with its own header block and its own empty state. That
// made it the odd one out among the six Management destinations: no search,
// no column menu, no density control, no refresh, no pagination, and a
// second implementation of chrome `TablePage` already owns.
//
// It now uses `TablePage` with `header={false}`, exactly as Users and API
// Keys do — the section's title and description are already on the hero
// card above it, so drawing them again here was saying the same thing
// twice.

import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus, IconShieldLock } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { TablePage } from '@resources/layout/TablePage';
import type { SearchMode } from '@resources/utils/search-filter';
import { DeletePolicyDialog } from './DeletePolicyDialog';
import { usePolicyColumns } from './usePolicyColumns';
import {
  usePolicies,
  useCreatePolicy,
  useUpdatePolicy,
  useDeletePolicy,
  useTogglePolicy,
  type Policy,
  type PolicyInput,
} from '@resources/hooks/usePolicies';
import { PolicyEditorDialog } from '@resources/components/security/abac/PolicyEditorDialog';
import { PolicyTesterDialog } from '@resources/components/security/abac/PolicyTesterDialog';
import { PERM } from '@core/auth/permissionKeys';

export function PoliciesSection() {
  const { t } = useTranslation('abac');
  const { data, isLoading, isFetching, refetch } = usePolicies();
  const create = useCreatePolicy();
  const update = useUpdatePolicy();
  const remove = useDeletePolicy();
  const toggle = useTogglePolicy();

  const [editorOpen, setEditorOpen] = useState(false);
  const [editing, setEditing] = useState<Policy | null>(null);
  const [pendingDelete, setPendingDelete] = useState<Policy | null>(null);
  const [testing, setTesting] = useState<Policy | null>(null);
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');

  const openCreate = () => {
    setEditing(null);
    setEditorOpen(true);
  };
  const openEdit = useCallback((p: Policy) => {
    setEditing(p);
    setEditorOpen(true);
  }, []);

  const handleSubmit = (input: PolicyInput) => {
    if (editing) {
      update.mutate({ id: editing.id, ...input }, { onSuccess: () => setEditorOpen(false) });
    } else {
      create.mutate(input, { onSuccess: () => setEditorOpen(false) });
    }
  };

  const confirmDelete = () => {
    if (!pendingDelete) return;
    remove.mutate(pendingDelete.id, {
      onSuccess: () => setPendingDelete(null),
    });
  };

  const columns = usePolicyColumns({
    toggle,
    onTest: setTesting,
    onEdit: openEdit,
    onDelete: setPendingDelete,
  });

  return (
    <>
      <TablePage<Policy>
        id="abac-policies"
        // Rendered inside the security tabs, which already carry the title.
        header={false}
        title={t('title')}
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('table.name'),
          fields: (p) => [p.name, p.description, p.resourceType],
        }}
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void refetch(), isFetching },
          actions: (
            <Button permission={PERM.RBAC_POLICIES_MANAGE} size="sm" onClick={openCreate}>
              <IconPlus className="mr-1 size-3.5" />
              {t('actions.new')}
            </Button>
          ),
        }}
        columns={columns}
        data={data ?? []}
        loading={isLoading}
        empty={{
          icon: <IconShieldLock className="size-10 text-muted-foreground" />,
          title: t('empty.title'),
          description: t('empty.description'),
        }}
        getRowId={(p) => p.id}
      />

      <PolicyEditorDialog
        open={editorOpen}
        onOpenChange={setEditorOpen}
        initial={editing}
        onSubmit={handleSubmit}
        isSubmitting={create.isPending || update.isPending}
      />

      <DeletePolicyDialog
        policyName={pendingDelete?.name}
        isOpen={!!pendingDelete}
        isPending={remove.isPending}
        onCancel={() => setPendingDelete(null)}
        onConfirm={confirmDelete}
      />

      {testing && (
        <PolicyTesterDialog
          open={!!testing}
          onOpenChange={(o) => !o && setTesting(null)}
          policyId={testing.id}
          policyName={testing.name}
          policyExpression={testing.expression}
          policyResourceType={testing.resourceType}
        />
      )}
    </>
  );
}
