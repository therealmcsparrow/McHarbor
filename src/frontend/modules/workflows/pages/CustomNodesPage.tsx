// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus, IconPencil, IconTrash, IconBolt } from '@tabler/icons-react';
import { PageHeader } from '@resources/layout/PageHeader';
import { PageBody } from '@resources/layout/PageBody';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Alert } from '@resources/components/ui/Alert';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Spinner } from '@resources/components/ui/Spinner';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { useCustomNodeSync, type CustomNodeWithCode } from '../hooks/useCustomNodeSync';
import { useDeleteCustomNode } from '../hooks/useCustomNodes';
import { CustomNodeDialog } from '../components/CustomNodeDialog';

/**
 * Custom nodes: the authoring surface for user-written workflow nodes.
 *
 * The read half shipped long ago — `useCustomNodeSync` merges whatever
 * `GET /custom-nodes` returns into the palette — but the four mutating routes
 * were gated by `custom_nodes.manage`, which nothing reached. This page is
 * that control surface, so the key now governs something a user can see.
 *
 * Every string comes from the `common.customNodes.*` block, which already
 * existed fully translated in all six locales before the screen did.
 */
export default function CustomNodesPage() {
  const { t } = useTranslation('common');
  const query = useCustomNodeSync();
  const deleteNode = useDeleteCustomNode();
  const canView = usePermission(PERM.CUSTOM_NODES_VIEW);

  const [editing, setEditing] = useState<CustomNodeWithCode | undefined>();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [pendingDelete, setPendingDelete] = useState<CustomNodeWithCode | null>(null);

  const nodes = query.data ?? [];

  function openCreate() {
    setEditing(undefined);
    setDialogOpen(true);
  }

  function openEdit(node: CustomNodeWithCode) {
    setEditing(node);
    setDialogOpen(true);
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-6">
      <PageHeader
        headingLeaf
        title={t('customNodes.title')}
        description={t('customNodes.description')}
        actions={
          <Button permission={PERM.CUSTOM_NODES_MANAGE} onClick={openCreate}>
            <IconPlus className="size-4" aria-hidden="true" />
            {t('customNodes.createNode')}
          </Button>
        }
      />

      <PageBody className="gap-6">
        {!canView ? (
          <Alert
            variant="warning"
            title={t('permission.denied')}
            description={t('permission.requires', { permission: PERM.CUSTOM_NODES_VIEW })}
          />
        ) : query.isLoading ? (
          <div className="flex justify-center py-16">
            <Spinner />
          </div>
        ) : nodes.length === 0 ? (
          <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-border py-16">
            <IconBolt className="size-12 text-muted-foreground/30" aria-hidden="true" />
            <p className="mt-4 text-sm text-muted-foreground">{t('customNodes.noCustomNodes')}</p>
          </div>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {nodes.map((node) => (
              <div key={node.key} className="rounded-lg border border-border bg-card p-4">
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <h3 className="truncate text-sm font-medium text-foreground">{node.label}</h3>
                    <p className="truncate text-xs text-muted-foreground">{node.key}</p>
                  </div>
                  <Badge variant="secondary">{node.category}</Badge>
                </div>
                {node.description && (
                  <p className="mt-2 line-clamp-2 text-xs text-muted-foreground">{node.description}</p>
                )}
                <div className="mt-3 flex justify-end gap-1">
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={t('customNodes.editNode')}
                    permission={PERM.CUSTOM_NODES_MANAGE}
                    onClick={() => openEdit(node)}
                  >
                    <IconPencil className="size-4" aria-hidden="true" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={t('customNodes.deleteNode')}
                    permission={PERM.CUSTOM_NODES_MANAGE}
                    onClick={() => setPendingDelete(node)}
                  >
                    <IconTrash className="size-4" aria-hidden="true" />
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </PageBody>

      {dialogOpen && <CustomNodeDialog open={dialogOpen} node={editing} onClose={() => setDialogOpen(false)} />}

      <ConfirmDialog
        open={!!pendingDelete}
        onOpenChange={(next) => !next && setPendingDelete(null)}
        title={t('customNodes.deleteNode')}
        description={t('customNodes.deleteConfirm')}
        onConfirm={() => {
          if (pendingDelete) deleteNode.mutate(pendingDelete.key);
          setPendingDelete(null);
        }}
      />
    </div>
  );
}
