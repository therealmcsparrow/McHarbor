// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus, IconPackage, IconApps, IconPuzzle, IconDeviceDesktop } from '@tabler/icons-react';
import type { TablerIcon } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { TableSkeleton } from '@resources/components/ui/Skeleton';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Section } from '@resources/components/ui/Form';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import {
  RENDERABLE_CATALOG_KINDS,
  visibleCatalogKinds,
  type Catalog,
  type VisibleCatalogKind,
  useCatalogs,
  useDeleteCatalog,
  useUpdateCatalog,
} from '../hooks/useCatalogs';
import { AddCatalogDialog } from './AddCatalogDialog';
import { EditCatalogDialog } from './EditCatalogDialog';
import { CatalogTestDialog } from './CatalogTestDialog';
import { CatalogRow } from './CatalogRow';
import { PERM } from '@core/auth/permissionKeys';

const KIND_ICON: Record<VisibleCatalogKind, TablerIcon> = {
  docker_app: IconApps,
  plugin: IconPuzzle,
  vm_image: IconDeviceDesktop,
};

export function CatalogsTab() {
  const { t } = useTranslation('settings');
  const catalogs = useCatalogs();
  const updateCatalog = useUpdateCatalog();
  const deleteCatalog = useDeleteCatalog();
  const [addOpen, setAddOpen] = useState(false);
  const [pendingDelete, setPendingDelete] = useState<Catalog | null>(null);
  const [testTarget, setTestTarget] = useState<Catalog | null>(null);
  const [editTarget, setEditTarget] = useState<Catalog | null>(null);

  const addCatalog = () => setAddOpen(true);

  const kinds = useMemo(() => visibleCatalogKinds(catalogs.data), [catalogs.data]);

  const grouped = useMemo(() => {
    const map = Object.fromEntries(RENDERABLE_CATALOG_KINDS.map((kind) => [kind, [] as Catalog[]])) as Record<
      VisibleCatalogKind,
      Catalog[]
    >;
    (catalogs.data ?? []).forEach((c) => {
      // A kind with no section is dropped rather than pushed. Themes
      // have never had one, and a backend that has not yet run the
      // migration retiring the widget and node kinds can still return
      // rows for them - neither should be able to crash the page.
      const bucket = map[c.kind as VisibleCatalogKind];
      if (bucket) {
        bucket.push(c);
      }
    });
    return map;
  }, [catalogs.data]);

  const handleToggle = (catalog: Catalog) => {
    updateCatalog.mutate({ id: catalog.id, patch: { enabled: !catalog.enabled } });
  };

  const handleConfirmDelete = async () => {
    if (!pendingDelete) return;
    await deleteCatalog.mutateAsync(pendingDelete.id);
    setPendingDelete(null);
  };

  if (catalogs.isLoading) {
    return <TableSkeleton rows={6} columns={4} />;
  }

  if (catalogs.isError) {
    return <ErrorState title={t('catalogs.errorTitle')} error={catalogs.error} onRetry={() => catalogs.refetch()} />;
  }

  const totalCount = kinds.reduce((sum, kind) => sum + grouped[kind].length, 0);

  return (
    <div className="space-y-4">
      <Section icon={<IconPackage />} title={t('catalogs.title')} help={t('catalogs.help')}>
        <div className="mb-4 flex justify-end">
          <Button permission={PERM.SETTINGS_MANAGE} size="sm" variant="outline" onClick={addCatalog}>
            <IconPlus className="size-4" />
            {t('catalogs.addSubmit')}
          </Button>
        </div>
        {totalCount === 0 ? (
          <EmptyState
            icon={<IconPackage className="size-6" />}
            title={t('catalogs.emptyTitle')}
            description={t('catalogs.emptyDescription')}
          />
        ) : null}
      </Section>

      {kinds.map((kind) => {
        const items = grouped[kind];
        const Icon = KIND_ICON[kind];
        return (
          <Section key={kind} icon={<Icon />} title={t(`catalogs.kind.${kind}`)} help={t(`catalogs.kindHelp.${kind}`)}>
            {items.length === 0 ? (
              <p className="rounded-md border border-dashed border-border p-4 text-center text-sm text-muted-foreground">
                {t('catalogs.noneForKind')}
              </p>
            ) : (
              <ul className="mc-glass divide-y divide-border rounded-lg">
                {items.map((c) => (
                  <CatalogRow
                    key={c.id}
                    catalog={c}
                    togglePending={updateCatalog.isPending}
                    deletePending={deleteCatalog.isPending}
                    onTest={setTestTarget}
                    onEdit={setEditTarget}
                    onToggle={handleToggle}
                    onDelete={setPendingDelete}
                  />
                ))}
              </ul>
            )}
          </Section>
        );
      })}

      <AddCatalogDialog open={addOpen} onOpenChange={setAddOpen} />

      <EditCatalogDialog
        catalog={editTarget}
        open={!!editTarget}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) setEditTarget(null);
        }}
      />

      <CatalogTestDialog
        catalog={testTarget}
        open={!!testTarget}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) setTestTarget(null);
        }}
      />

      <ConfirmDialog
        open={!!pendingDelete}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) setPendingDelete(null);
        }}
        title={t('catalogs.deleteConfirmTitle')}
        description={pendingDelete ? t('catalogs.deleteConfirmDescription', { name: pendingDelete.name }) : ''}
        confirmLabel={t('common.delete')}
        variant="destructive"
        loading={deleteCatalog.isPending}
        onConfirm={handleConfirmDelete}
      />
    </div>
  );
}
