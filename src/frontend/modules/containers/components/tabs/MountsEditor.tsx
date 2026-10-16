// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconPlus, IconTrash } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { Checkbox } from '@resources/components/ui/Checkbox';
import { Select } from '@resources/components/ui/Select';
import { useStableListKeys } from '@resources/hooks/useStableListKeys';
import type { MountEntry, MountType } from '../../types/edit-form';

// Example paths are technical tokens, the same in every language.
const BIND_SOURCE_EXAMPLE = '/srv/app/data';
const TARGET_EXAMPLE = '/data';

const FIELD_CLASS =
  'mc-control min-h-11 rounded-md border border-border bg-muted px-2 font-mono text-xs text-foreground focus:border-primary focus:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:min-h-0';

type MountsEditorProps = {
  mounts: MountEntry[];
  onChange: (mounts: MountEntry[]) => void;
};

export function MountsEditor({ mounts, onChange }: MountsEditorProps) {
  const { t } = useTranslation('containers');
  // Keyed by position: a key built from the path would remount the row while it is typed in.
  const keys = useStableListKeys(mounts);
  const typeOptions = [
    { value: 'bind', label: t('mounts.edit.typeBind') },
    { value: 'volume', label: t('mounts.edit.typeVolume') },
    { value: 'tmpfs', label: t('mounts.edit.typeTmpfs') },
  ];

  const update = (index: number, patch: Partial<MountEntry>) =>
    onChange(mounts.map((m, i) => (i === index ? { ...m, ...patch } : m)));
  const fieldLabel = (field: string, index: number) => t('mounts.edit.fieldLabel', { field, number: index + 1 });

  return (
    <div className="space-y-3">
      <Alert variant="warning" title={t('mounts.edit.warningTitle')}>
        {t('mounts.edit.warning')}
      </Alert>
      {mounts.map((m, index) => (
        <div key={keys[index]} className="grid grid-cols-1 items-center gap-2 sm:grid-cols-[120px_1fr_1fr_auto_auto]">
          <Select
            value={m.type}
            onChange={(next) => update(index, { type: next as MountType, source: next === 'tmpfs' ? '' : m.source })}
            options={typeOptions}
            searchable={false}
            ariaLabel={fieldLabel(t('mounts.type'), index)}
            variant="outline"
            className="min-h-11 text-xs sm:min-h-0"
          />
          <input
            type="text"
            value={m.source}
            disabled={m.type === 'tmpfs'}
            onChange={(evt) => update(index, { source: evt.target.value })}
            placeholder={
              m.type === 'volume'
                ? t('mounts.edit.volumePlaceholder')
                : m.type === 'bind'
                  ? BIND_SOURCE_EXAMPLE
                  : undefined
            }
            aria-label={fieldLabel(t('mounts.source'), index)}
            className={FIELD_CLASS}
          />
          <input
            type="text"
            value={m.target}
            onChange={(evt) => update(index, { target: evt.target.value })}
            placeholder={TARGET_EXAMPLE}
            aria-label={fieldLabel(t('mounts.destination'), index)}
            className={FIELD_CLASS}
          />
          <label className="flex items-center gap-2 text-xs text-muted-foreground">
            <Checkbox
              checked={m.readOnly}
              disabled={m.type === 'tmpfs'}
              onCheckedChange={(checked) => update(index, { readOnly: checked === true })}
            />
            {t('mounts.readOnly')}
          </label>
          <Button
            variant="ghost"
            size="icon"
            onClick={() => onChange(mounts.filter((_, i) => i !== index))}
            aria-label={t('mounts.edit.remove', { target: m.target || index + 1 })}
            className="size-11 text-muted-foreground hover:text-destructive sm:size-9"
          >
            <IconTrash className="size-3.5" />
          </Button>
        </div>
      ))}
      <Button
        variant="outline"
        size="sm"
        onClick={() =>
          onChange([...mounts, { type: 'bind', source: '', target: '', readOnly: false, propagation: '' }])
        }
      >
        <IconPlus className="mr-1 size-3.5" />
        {t('mounts.edit.add')}
      </Button>
    </div>
  );
}
