// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useIsNative } from '@resources/hooks/useInstallKind';
import { Link } from 'react-router';
import { IconArrowRight, IconCircleCheck, IconDatabase, IconKey, IconPlus } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { cn } from '@resources/utils/cn';
import { StorageLocationCard } from './StorageLocationCard';
import { StorageLocationDialog } from './StorageLocationDialog';
import {
  type StorageLocation,
  useBackupEncryptionKeyStatus,
  useStorageLocations,
} from '@resources/hooks/useStorageLocations';
import { PERM } from '@core/auth/permissionKeys';

/**
 * Settings → Storage: the destinations backups and exports are written to.
 *
 * The backup encryption key used to sit at the top of this tab, which read
 * as if encryption were a property of a destination. It is not — one
 * instance-wide key wraps every archive before it is uploaded anywhere — so
 * it now lives on Backups → Settings. What stays here is a read-only status
 * row: enough to answer "are my backups encrypted?" without implying this
 * is where the answer is changed.
 */
export function StorageTab() {
  const { t } = useTranslation('settings');
  const native = useIsNative();
  const { data: locations = [], isLoading } = useStorageLocations();
  const { data: backupKeyStatus } = useBackupEncryptionKeyStatus();
  const [createOpen, setCreateOpen] = useState(false);
  const [editLocation, setEditLocation] = useState<StorageLocation | null>(null);

  const backupKeyActive = backupKeyStatus?.readable ?? false;

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="mc-glass rounded-lg p-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex gap-3">
            <div
              className={cn(
                'flex size-10 shrink-0 items-center justify-center rounded-lg border transition-colors',
                backupKeyActive
                  ? 'border-chart-1/30 bg-chart-1/10 text-chart-1'
                  : 'border-border bg-muted text-muted-foreground',
              )}
            >
              {backupKeyActive ? <IconCircleCheck className="size-5" /> : <IconKey className="size-5" />}
            </div>
            <div>
              <div className="flex flex-wrap items-center gap-2">
                <h3 className="text-sm font-medium text-foreground">{t('storage.encryptionRowTitle')}</h3>
                <Badge variant={backupKeyActive ? 'success' : 'secondary'} className="py-1">
                  {backupKeyActive && <IconCircleCheck className="size-3.5" />}
                  {backupKeyActive ? t('backupKey.activeStatus') : t('storage.encryptionRowInactive')}
                </Badge>
              </div>
              <p className="mt-1 text-xs text-muted-foreground">
                {t(native ? 'storage.encryptionRowDescriptionNative' : 'storage.encryptionRowDescription')}
              </p>
              {backupKeyActive && backupKeyStatus?.keyId && (
                <p className="mt-1 font-mono text-xs text-muted-foreground">
                  {t('backupKey.fingerprint', { value: backupKeyStatus.keyId })}
                </p>
              )}
            </div>
          </div>
          <Button asChild variant="outline" size="sm">
            <Link to="/backups?tab=settings">
              {t('storage.encryptionRowLink')}
              <IconArrowRight className="size-4" />
            </Link>
          </Button>
        </div>
      </div>

      <div className="flex items-center justify-between gap-4">
        <p className="text-sm text-muted-foreground">{t('storage.description')}</p>
        <Button permission={[PERM.SETTINGS_MANAGE, PERM.STORAGE_LOCATIONS_MANAGE]} onClick={() => setCreateOpen(true)}>
          <IconPlus className="size-4" />
          {t('storage.addLocation')}
        </Button>
      </div>

      {locations.length > 0 ? (
        <div className="space-y-3">
          {locations.map((loc) => (
            <StorageLocationCard key={loc.id} location={loc} onEdit={setEditLocation} />
          ))}
        </div>
      ) : (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border py-12 text-center">
          <IconDatabase className="size-10 text-muted-foreground" />
          <p className="mt-3 text-sm text-muted-foreground">{t('storage.noLocations')}</p>
        </div>
      )}

      <StorageLocationDialog open={createOpen} onOpenChange={setCreateOpen} />
      {editLocation && (
        <StorageLocationDialog
          open={!!editLocation}
          onOpenChange={(isOpen) => {
            if (!isOpen) setEditLocation(null);
          }}
          location={editLocation}
        />
      )}
    </div>
  );
}
