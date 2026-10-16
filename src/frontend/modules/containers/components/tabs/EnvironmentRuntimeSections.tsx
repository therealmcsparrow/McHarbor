// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { InfoRow } from '@resources/components/ui/InfoRow';
import { KeyValueEditor } from '../KeyValueEditor';
import type { ContainerInspect } from '@core/types/docker';
import type { EditFormData } from '../../types/edit-form';
import { EditInput, ToggleField } from './EditFieldControls';

type ChangeHandler = <K extends keyof EditFormData>(field: K, value: EditFormData[K]) => void;

type Translator = (key: string) => string;

const DEFAULT_LOG_DRIVER = 'json-file';

export function EnvironmentConsoleSection({
  t,
  container,
  editing,
  editData,
  onFieldChange,
}: {
  t: Translator;
  container: ContainerInspect;
  editing: boolean;
  editData: EditFormData | null;
  onFieldChange: ChangeHandler;
}) {
  return (
    <div className="mc-glass rounded-lg p-6">
      <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
        {t('environment.console')}
      </h3>
      {editing ? (
        <div className="space-y-3">
          <ToggleField
            label={t('environment.tty')}
            checked={editData?.tty ?? false}
            onChange={(value) => onFieldChange('tty', value)}
          />
          <ToggleField
            label={t('environment.openStdin')}
            checked={editData?.openStdin ?? false}
            onChange={(value) => onFieldChange('openStdin', value)}
          />
          <EditInput
            label={t('environment.logDriver')}
            value={editData?.logDriver ?? ''}
            onChange={(value) => onFieldChange('logDriver', value)}
            placeholder={t('placeholders.logDriver')}
          />
        </div>
      ) : (
        <>
          <InfoRow label={t('environment.tty')}>
            {container.Config?.Tty ? t('common:labels.yes') : t('common:labels.no')}
          </InfoRow>
          <InfoRow label={t('environment.openStdin')}>
            {container.Config?.OpenStdin ? t('common:labels.yes') : t('common:labels.no')}
          </InfoRow>
          <InfoRow label={t('environment.logDriver')}>
            {container.HostConfig?.LogConfig?.Type ?? DEFAULT_LOG_DRIVER}
          </InfoRow>
        </>
      )}
    </div>
  );
}

export function EnvironmentLogOptionsSection({
  t,
  editing,
  logOptionEntries,
  onLogOptionChange,
}: {
  t: Translator;
  editing: boolean;
  logOptionEntries: Array<{ key: string; value: string }>;
  onLogOptionChange: (entries: Array<{ key: string; value: string }>) => void;
}) {
  if (!editing && logOptionEntries.length === 0) {
    return null;
  }

  return (
    <div className="mc-glass rounded-lg p-6 lg:col-span-2">
      <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
        {t('environment.logOptions')}
      </h3>
      {editing ? (
        <KeyValueEditor
          entries={logOptionEntries}
          onChange={onLogOptionChange}
          keyLabel={t('edit.key')}
          valueLabel={t('edit.value')}
          addLabel={t('edit.addLogOption')}
        />
      ) : (
        <div className="max-h-48 overflow-y-auto">
          {logOptionEntries.map(({ key, value }) => (
            <div key={key} className="flex gap-2 border-b border-border py-1.5 last:border-0">
              <span className="shrink-0 font-mono text-xs font-medium text-foreground">{key}</span>
              <span className="truncate font-mono text-xs text-muted-foreground">{value}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
