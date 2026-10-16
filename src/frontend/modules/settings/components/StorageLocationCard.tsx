// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconDatabaseImport, IconExternalLink, IconPencil, IconPlugConnected, IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Switch } from '@resources/components/ui/Switch';
import {
  type StorageLocation,
  useAuthorizeStorageLocation,
  useDeleteStorageLocation,
  useMigrateStorageLocationBackups,
  useTestStorageLocation,
  useUpdateStorageLocation,
} from '@resources/hooks/useStorageLocations';
import { useStorageAdapters } from '../hooks/useStorageAdapters';
import { storageProvider } from './storage-location-options';
import { StorageLocationTestDialog } from './StorageLocationTestDialog';
import { StorageLocationUsage } from './StorageLocationUsage';
import { PERM } from '@core/auth/permissionKeys';

type StorageLocationCardProps = {
  location: StorageLocation;
  onEdit: (location: StorageLocation) => void;
};

function locationSummary(storageLocation: StorageLocation) {
  if (storageLocation.locationType === 'local') {
    return storageLocation.basePath ?? '';
  }
  // Plugin-owned destinations — the host doesn't know how to
  // summarize them. The plugin's icon + name already convey the
  // destination type on the card; leave the summary blank so the
  // card height stays consistent.
  return '';
}

function requiresConsent(storageLocation: StorageLocation) {
  // Every non-local destination uses plugin-owned OAuth. The
  // host has no opinion about which locations need a consent step
  // — the plugin's consent flow handles its own.
  return storageLocation.locationType !== 'local';
}

export function StorageLocationCard({ location: storageLocation, onEdit }: StorageLocationCardProps) {
  const { t } = useTranslation('settings');
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [testOpen, setTestOpen] = useState(false);
  const authorizeLocation = useAuthorizeStorageLocation();
  const deleteLocation = useDeleteStorageLocation();
  const migrateBackups = useMigrateStorageLocationBackups();
  const updateLocation = useUpdateStorageLocation();
  const testLocation = useTestStorageLocation();
  const { data: pluginAdapters } = useStorageAdapters();
  const provider = storageProvider(storageLocation.locationType, pluginAdapters);
  const summary = locationSummary(storageLocation) || t('storage.noEndpoint');
  const protectedLocal = storageLocation.id === 'default-local-backup';

  function handleDelete() {
    deleteLocation.mutate(storageLocation.id, {
      onSuccess: () => setDeleteOpen(false),
    });
  }

  function handleAuthorize() {
    authorizeLocation.mutate(storageLocation.id, {
      onSuccess: (data) => {
        window.location.assign(data.authorizationUrl);
      },
    });
  }

  function handleMigrateBackups() {
    migrateBackups.mutate(storageLocation.id);
  }

  function handleTest() {
    setTestOpen(true);
    testLocation.mutate(storageLocation.id);
  }

  return (
    <>
      <div className="mc-glass flex items-center gap-4 rounded-lg p-4">
        <div className="flex size-10 shrink-0 items-center justify-center rounded-lg border border-border bg-muted text-muted-foreground">
          <span className="flex size-5 items-center justify-center">{provider.icon}</span>
        </div>

        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h4 className="truncate text-sm font-medium text-foreground">{storageLocation.name}</h4>
            <Badge variant={storageLocation.enabled ? 'default' : 'secondary'}>
              {storageLocation.enabled ? t('storage.enabled') : t('storage.disabled')}
            </Badge>
            <Badge variant="outline">{provider.labelIsKey ? t(provider.label) : provider.label}</Badge>
          </div>
          <p className="mt-1 truncate text-xs text-muted-foreground">{summary}</p>
          <StorageLocationUsage locationId={storageLocation.id} enabled={storageLocation.enabled} />
        </div>

        <div className="flex items-center gap-1">
          {requiresConsent(storageLocation) && (
            <Button
              permission={PERM.SETTINGS_MANAGE}
              variant="outline"
              size="sm"
              disabled={authorizeLocation.isPending}
              onClick={handleAuthorize}
            >
              <IconExternalLink className="size-4" />
              {t('storage.connectConsent')}
            </Button>
          )}
          <Button
            permission={PERM.SETTINGS_MANAGE}
            variant="outline"
            size="sm"
            disabled={!storageLocation.enabled || testLocation.isPending}
            onClick={handleTest}
            data-testid={`storage-test-${storageLocation.id}`}
            aria-label={t('storage.test.button')}
          >
            {testLocation.isPending ? (
              <span className="inline-block size-4 animate-spin rounded-full border-2 border-current border-t-transparent" />
            ) : (
              <IconPlugConnected className="size-4" />
            )}
            {t('storage.test.button')}
          </Button>
          {storageLocation.locationType === 'local' && (
            <Button
              permission={PERM.SETTINGS_MANAGE}
              variant="outline"
              size="sm"
              disabled={!storageLocation.enabled || migrateBackups.isPending}
              onClick={handleMigrateBackups}
            >
              <IconDatabaseImport className="size-4" />
              {migrateBackups.isPending ? t('storage.migratingBackups') : t('storage.migrateBackups')}
            </Button>
          )}
          <Switch
            checked={storageLocation.enabled}
            aria-label={t('storage.toggleEnabled')}
            disabled={protectedLocal || updateLocation.isPending}
            onCheckedChange={(enabled) => updateLocation.mutate({ id: storageLocation.id, enabled })}
          />
          <Button
            permission={[PERM.SETTINGS_MANAGE, PERM.STORAGE_LOCATIONS_MANAGE]}
            variant="ghost"
            size="icon-sm"
            aria-label={t('storage.editLocation')}
            onClick={() => onEdit(storageLocation)}
          >
            <IconPencil className="size-4" />
          </Button>
          <Button
            permission={[PERM.SETTINGS_MANAGE, PERM.STORAGE_LOCATIONS_DELETE]}
            variant="ghost"
            size="icon-sm"
            aria-label={t('storage.deleteLocation')}
            disabled={protectedLocal}
            onClick={() => setDeleteOpen(true)}
          >
            <IconTrash className="size-4 text-destructive" />
          </Button>
        </div>
      </div>

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t('storage.confirmDeleteTitle')}
        description={t('storage.confirmDeleteDescription')}
        confirmLabel={t('storage.confirmDeleteLabel')}
        loading={deleteLocation.isPending}
        onConfirm={handleDelete}
      />

      <StorageLocationTestDialog
        result={testLocation.data ?? null}
        pending={testLocation.isPending}
        locationName={storageLocation.name}
        open={testOpen}
        onOpenChange={setTestOpen}
      />
    </>
  );
}
