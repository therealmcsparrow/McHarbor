// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCheck, IconEdit, IconX } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { toNumberOrNull, toNumberText } from '@resources/utils/numberText';

type InlineEditFieldProps = {
  label: string;
  value: string;
  displayValue: ReactNode;
  numeric?: boolean;
  disabled?: boolean;
  onSave: (value: string) => Promise<void>;
};

export function InlineEditField({
  label,
  value,
  displayValue,
  numeric = false,
  disabled = false,
  onSave,
}: InlineEditFieldProps) {
  const { t } = useTranslation('containers');
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(value);
  const [saving, setSaving] = useState(false);
  const [appliedValue, setAppliedValue] = useState(value);
  if (appliedValue !== value) {
    setAppliedValue(value);
    setDraft(value);
  }
  const editorRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (editing) editorRef.current?.querySelector('input')?.focus();
  }, [editing]);

  const cancel = () => {
    setDraft(value);
    setEditing(false);
  };
  const save = async () => {
    setSaving(true);
    try {
      await onSave(draft);
      setEditing(false);
    } catch {
      // Mutation feedback is handled by the shared API error pipeline.
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="flex min-h-10 items-center gap-2 border-b border-border py-2 last:border-0">
      <span className="w-36 shrink-0 text-sm font-medium text-muted-foreground">{label}</span>
      {editing ? (
        <div ref={editorRef} className="flex min-w-0 flex-1 items-center gap-1.5">
          {numeric ? (
            <NumberInput
              nullable
              value={toNumberOrNull(draft)}
              onChange={(next) => setDraft(toNumberText(next))}
              onKeyDown={(evt2) => {
                if (evt2.key === 'Escape') cancel();
                if (evt2.key === 'Enter') void save();
              }}
              aria-label={t('inlineEdit.inputLabel', { field: label })}
              className="min-w-0 flex-1"
            />
          ) : (
            <Input
              type="text"
              value={draft}
              onChange={(changeEvent) => setDraft(changeEvent.target.value)}
              onKeyDown={(evt) => {
                if (evt.key === 'Escape') cancel();
                if (evt.key === 'Enter') void save();
              }}
              aria-label={t('inlineEdit.inputLabel', { field: label })}
              variant="outline"
              className="min-w-0"
            />
          )}
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={() => void save()}
            disabled={saving}
            aria-label={t('inlineEdit.save', { field: label })}
          >
            <IconCheck className="size-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={cancel}
            disabled={saving}
            aria-label={t('inlineEdit.cancel', { field: label })}
          >
            <IconX className="size-4" />
          </Button>
        </div>
      ) : (
        <div className="flex min-w-0 flex-1 items-center justify-between gap-2">
          <span className="min-w-0 text-sm break-words text-foreground">{displayValue}</span>
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={() => setEditing(true)}
            disabled={disabled}
            aria-label={t('inlineEdit.edit', { field: label })}
          >
            <IconEdit className="size-4" />
          </Button>
        </div>
      )}
    </div>
  );
}
