// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { KeyValueEditor } from '../KeyValueEditor';
import type { ContainerInspect } from '@core/types/docker';
import type { EditFormData } from '../../types/edit-form';

type LabelsTabProps = {
  container: ContainerInspect;
  editing: boolean;
  editData: EditFormData | null;
  onFieldChange: <K extends keyof EditFormData>(field: K, value: EditFormData[K]) => void;
};

export function LabelsTab({ container, editing, editData, onFieldChange }: LabelsTabProps) {
  const { t } = useTranslation('containers');

  const labelEntries = useMemo(() => {
    const labels = editing ? (editData?.labels ?? {}) : (container.Config?.Labels ?? {});
    return Object.entries(labels).map(([key, value]) => ({ key, value }));
  }, [editing, editData?.labels, container.Config?.Labels]);

  const handleLabelChange = useCallback(
    (entries: Array<{ key: string; value: string }>) => {
      onFieldChange('labels', Object.fromEntries(entries.map((e) => [e.key, e.value])));
    },
    [onFieldChange],
  );

  return (
    <div className="grid grid-cols-1 gap-6">
      <div className="rounded-lg border border-border bg-card p-6">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('environment.labels')}</h3>
        {editing ? (
          <KeyValueEditor
            entries={labelEntries}
            onChange={handleLabelChange}
            keyLabel={t('edit.key')}
            valueLabel={t('edit.value')}
            addLabel={t('edit.addLabel')}
          />
        ) : labelEntries.length > 0 ? (
          <div className="max-h-96 overflow-y-auto">
            {labelEntries.map(({ key, value }) => (
              <div key={key} className="flex gap-2 border-b border-border py-1.5 last:border-0">
                <span className="shrink-0 font-mono text-xs font-medium text-foreground">{key}</span>
                <span className="truncate font-mono text-xs text-muted-foreground">{value}</span>
              </div>
            ))}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">{t('overview.noLabels')}</p>
        )}
      </div>
    </div>
  );
}
