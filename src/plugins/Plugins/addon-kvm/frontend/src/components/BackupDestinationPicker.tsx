// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import type { StorageLocationRef } from '../hooks/useStorageLocations';

export function BackupDestinationPicker({
  available,
  selectedIds,
  onToggle,
}: {
  available: StorageLocationRef[];
  selectedIds: string[];
  onToggle: (id: string) => void;
}) {
  const { t } = useTranslation('vm');

  return (
    <div>
      <Label>{t('backups.form.destinations')}</Label>
      <p className="mb-2 text-sm text-muted-foreground">{t('backups.form.destinationsHint')}</p>
      {available.length === 0 ? (
        <p className="rounded-md bg-muted p-3 text-sm text-muted-foreground">{t('backups.noDestinations')}</p>
      ) : (
        <div className="space-y-2">
          {available.map((loc) => (
            <label key={loc.id} className="flex items-center justify-between gap-2">
              <span className="text-sm text-foreground">
                {loc.name}
                <span className="ml-2 text-xs text-muted-foreground">{loc.locationType}</span>
              </span>
              <Switch
                checked={selectedIds.includes(loc.id)}
                aria-label={loc.name}
                onCheckedChange={() => onToggle(loc.id)}
              />
            </label>
          ))}
        </div>
      )}
    </div>
  );
}
