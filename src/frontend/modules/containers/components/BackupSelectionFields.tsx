// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useId } from 'react';
import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import { BackupSchedulingFields } from './BackupSchedulingFields';
import { BackupSelectionOptionRow } from './BackupSelectionOptionRow';
import { fullBackupPatch } from './backupSelectionHelpers';

export { currentSelections, fullBackupPatch, isFullBackup, selectionOf } from './backupSelectionHelpers';
import type { ConfigSection, ContainerBackupInput, ContainerBackupOption } from '@resources/hooks/useContainerBackups';
import type { StorageLocation } from '@resources/hooks/useStorageLocations';

type BackupSelectionFieldsProps = {
  value: ContainerBackupInput;
  options: ContainerBackupOption[];
  storageLocations: StorageLocation[];
  showCron?: boolean;
  // containerId backs the live path picker used by "choose" mode. Without
  // it the middle mode has nothing to browse, so it is not offered.
  containerId?: string;
  configSections?: ConfigSection[];
  // Live container inspect, used to list the individual entries inside
  // each config section.
  containerInspect?: Record<string, unknown>;
  // fullBackup is a UI-only preset: when it is on every non-config
  // option is selected and the per-item list is replaced by a hint.
  // The request payload keeps its normal shape, so the backend needs
  // no notion of "full". Passing onFullBackupChange is what opts a
  // call site into the toggle; omit it and the list renders as before.
  fullBackup?: boolean;
  onFullBackupChange?: (value: boolean) => void;
  // showName off lets a page put BackupNameField at the top of its own
  // section instead of below whatever it renders first.
  showName?: boolean;
  onChange: (patch: Partial<ContainerBackupInput>) => void;
};

/** The backup's name, shared by every form that creates or edits one. */
export function BackupNameField({
  value,
  onChange,
}: {
  value: ContainerBackupInput;
  onChange: (patch: Partial<ContainerBackupInput>) => void;
}) {
  const { t } = useTranslation('containers');
  return (
    <div>
      <Label htmlFor="backup-name" className="mb-1 text-xs text-muted-foreground">
        {t('backups.name')}
      </Label>
      <Input
        id="backup-name"
        value={value.name}
        onChange={(evt) => onChange({ name: evt.target.value })}
        placeholder={t('backups.namePlaceholder')}
        variant="outline"
      />
    </div>
  );
}

export function BackupSelectionFields({
  value,
  options,
  storageLocations,
  showCron = false,
  containerId,
  configSections = [],
  containerInspect,
  fullBackup = false,
  onFullBackupChange,
  showName = true,
  onChange,
}: BackupSelectionFieldsProps) {
  const destinationId = useId();
  const { t } = useTranslation('containers');

  // Every destination is switchable, local included. Clearing the last
  // one is allowed here and caught by the action buttons, so the user
  // can rearrange their selection without the form fighting them.
  function toggleStorageLocation(id: string, checked: boolean) {
    const selected = new Set(value.storageLocationIds ?? []);
    if (checked) selected.add(id);
    else selected.delete(id);
    const storageLocationIds = Array.from(selected);
    onChange({
      storageLocationIds,
      storageLocationId: storageLocationIds[0] ?? '',
    });
  }

  function storageLocationDescription(locationValue: StorageLocation) {
    if (locationValue.locationType === 'local') {
      return t('backups.localStorageHint');
    }
    return locationValue.locationType;
  }

  return (
    <div className="space-y-4">
      {showName && <BackupNameField value={value} onChange={onChange} />}

      <div className="grid grid-cols-1 gap-4">
        <div>
          <Label className="mb-1 text-xs text-muted-foreground">{t('backups.destinations')}</Label>
          <div className="space-y-2 rounded-lg border border-border bg-background p-3">
            {storageLocations.map((loc) => {
              return (
                <label
                  key={loc.id}
                  htmlFor={`${destinationId}-${loc.id}`}
                  className="mc-glass flex items-start gap-3 rounded-md p-2"
                >
                  <Switch
                    id={`${destinationId}-${loc.id}`}
                    checked={(value.storageLocationIds ?? []).includes(loc.id)}
                    onCheckedChange={(checked) => toggleStorageLocation(loc.id, checked)}
                    aria-label={loc.name}
                  />
                  <span className="min-w-0">
                    <span className="block truncate text-sm font-medium text-foreground">{loc.name}</span>
                    <span className="block text-xs text-muted-foreground">{storageLocationDescription(loc)}</span>
                  </span>
                </label>
              );
            })}
            {storageLocations.length === 0 && (
              <p className="text-xs text-muted-foreground">{t('backups.noExternalDestinations')}</p>
            )}
          </div>
          {(value.storageLocationIds ?? []).length === 0 ? (
            <p className="mt-1 text-xs text-destructive">{t('backups.destinationRequired')}</p>
          ) : (
            <p className="mt-1 text-xs text-muted-foreground">{t('backups.destinationHint')}</p>
          )}
        </div>
        {showCron && <BackupSchedulingFields value={value} onChange={onChange} />}
      </div>

      <div>
        <div className="mb-2 flex items-start justify-between gap-3">
          <h3 className="text-sm font-medium text-foreground">{t('backups.whatToBackup')}</h3>
          {onFullBackupChange && (
            <label className="flex shrink-0 items-center gap-2">
              <span className="text-xs text-muted-foreground">{t('backups.fullToggle')}</span>
              <Switch
                checked={fullBackup}
                onCheckedChange={(checked) => {
                  onFullBackupChange(checked);
                  if (checked) onChange(fullBackupPatch(options));
                }}
                aria-label={t('backups.fullToggle')}
              />
            </label>
          )}
        </div>
        {onFullBackupChange && fullBackup ? (
          <p className="rounded-lg border border-border bg-muted/30 p-3 text-xs text-muted-foreground">
            {t('backups.fullHint')}
          </p>
        ) : (
          <div className="space-y-2">
            {options.map((option) => (
              <BackupSelectionOptionRow
                key={option.key}
                value={value}
                options={options}
                option={option}
                containerId={containerId}
                configSections={configSections}
                containerInspect={containerInspect}
                onChange={onChange}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
