// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Checkbox } from '@resources/components/ui/Checkbox';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { ReadableScheduleField } from '@resources/components/ReadableScheduleField';
import type { ContainerBackupInput, ContainerBackupOption } from '../hooks/useContainerBackups';
import type { StorageLocation } from '../../settings/hooks/useStorageLocations';

type BackupSelectionFieldsProps = {
  value: ContainerBackupInput;
  options: ContainerBackupOption[];
  storageLocations: StorageLocation[];
  showCron?: boolean;
  onChange: (patch: Partial<ContainerBackupInput>) => void;
};

function selectedFromValue(value: ContainerBackupInput, option: ContainerBackupOption) {
  if (option.type === 'config') return true;
  if (option.type === 'logs') return value.includeLogs;
  if (option.type === 'filesystem') return value.includeFilesystem;
  if (option.type === 'image') return value.includeImage;
  if (option.key.startsWith('mount:')) return value.selectedMounts.includes(option.key.slice('mount:'.length));
  return false;
}

function patchForOption(value: ContainerBackupInput, option: ContainerBackupOption, checked: boolean): Partial<ContainerBackupInput> {
  if (option.type === 'logs') return { includeLogs: checked };
  if (option.type === 'filesystem') return { includeFilesystem: checked };
  if (option.type === 'image') return { includeImage: checked };
  if (option.key.startsWith('mount:')) {
    const mountPath = option.key.slice('mount:'.length);
    const selected = new Set(value.selectedMounts);
    if (checked) selected.add(mountPath);
    else selected.delete(mountPath);
    return { selectedMounts: Array.from(selected) };
  }
  return {};
}

export function BackupSelectionFields({
  value,
  options,
  storageLocations,
  showCron = false,
  onChange,
}: BackupSelectionFieldsProps) {
  const { t } = useTranslation('containers');

  function toggleStorageLocation(id: string, checked: boolean) {
    const selected = new Set(value.storageLocationIds ?? []);
    if (checked) selected.add(id);
    else selected.delete(id);
    const storageLocationIds = Array.from(selected);
    onChange({ storageLocationIds, storageLocationId: storageLocationIds[0] ?? '' });
  }

  return (
    <div className="space-y-4">
      <div>
        <Label htmlFor="backup-name" className="mb-1 text-xs text-muted-foreground">
          {t('backups.name')}
        </Label>
        <Input
          id="backup-name"
          value={value.name}
          onChange={(event) => onChange({ name: event.target.value })}
          placeholder={t('backups.namePlaceholder')}
          variant="outline"
        />
      </div>

      <div className="grid grid-cols-1 gap-4">
        <div>
          <Label className="mb-1 text-xs text-muted-foreground">
            {t('backups.destinations')}
          </Label>
          <div className="space-y-2 rounded-lg border border-border bg-background p-3">
            <div className="flex items-start gap-3">
              <Checkbox checked disabled aria-label={t('backups.localStorage')} />
              <div>
                <p className="text-sm font-medium text-foreground">{t('backups.localStorage')}</p>
                <p className="text-xs text-muted-foreground">{t('backups.localStorageHint')}</p>
              </div>
            </div>
            {storageLocations.map((location) => (
              <label key={location.id} className="flex items-start gap-3 rounded-md border border-border bg-card p-2">
                <Checkbox
                  checked={(value.storageLocationIds ?? []).includes(location.id)}
                  onCheckedChange={(checked) => toggleStorageLocation(location.id, checked === true)}
                  aria-label={location.name}
                />
                <span className="min-w-0">
                  <span className="block truncate text-sm font-medium text-foreground">{location.name}</span>
                  <span className="block text-xs text-muted-foreground">{location.locationType}</span>
                </span>
              </label>
            ))}
            {storageLocations.length === 0 && (
              <p className="text-xs text-muted-foreground">{t('backups.noExternalDestinations')}</p>
            )}
          </div>
          <p className="mt-1 text-xs text-muted-foreground">{t('backups.destinationHint')}</p>
        </div>
        {showCron && (
          <>
            <ReadableScheduleField
              value={value.cron ?? ''}
              onChange={(cron) => onChange({ cron })}
              label={t('backups.scheduleReadable')}
              hint={t('backups.cronHint')}
            />
            <div className="rounded-lg border border-border bg-background p-3">
              <div className="mb-3">
                <h3 className="text-sm font-medium text-foreground">{t('backups.retentionTitle')}</h3>
                <p className="text-xs text-muted-foreground">{t('backups.retentionDescription')}</p>
              </div>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div>
                  <Label className="mb-1 text-xs text-muted-foreground">
                    {t('backups.retentionCount')}
                  </Label>
                  <NumberInput
                    value={value.retentionCount}
                    onChange={(retentionCount) => onChange({ retentionCount })}
                    min={0}
                    max={1000}
                    size="sm"
                  />
                  <p className="mt-1 text-xs text-muted-foreground">{t('backups.retentionCountHint')}</p>
                </div>
                <div>
                  <Label className="mb-1 text-xs text-muted-foreground">
                    {t('backups.retentionDays')}
                  </Label>
                  <NumberInput
                    value={value.retentionDays}
                    onChange={(retentionDays) => onChange({ retentionDays })}
                    min={0}
                    max={3650}
                    size="sm"
                  />
                  <p className="mt-1 text-xs text-muted-foreground">{t('backups.retentionDaysHint')}</p>
                </div>
              </div>
            </div>
          </>
        )}
      </div>

      <div>
        <h3 className="mb-2 text-sm font-medium text-foreground">{t('backups.whatToBackup')}</h3>
        <div className="space-y-2">
          {options.map((option) => (
            <label
              key={option.key}
              className="flex items-start gap-3 rounded-lg border border-border bg-card p-3"
            >
              <Checkbox
                checked={selectedFromValue(value, option)}
                disabled={option.required}
                onCheckedChange={(checked) => onChange(patchForOption(value, option, checked === true))}
                aria-label={option.label}
              />
              <span className="min-w-0 flex-1">
                <span className="block text-sm font-medium text-foreground">{option.label}</span>
                <span className="block text-xs text-muted-foreground">{option.description}</span>
              </span>
            </label>
          ))}
        </div>
      </div>
    </div>
  );
}
