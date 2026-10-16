// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Label } from '@resources/components/ui/Label';
import { MultiSelect } from '@resources/components/ui/MultiSelect';
import type { ConfigField } from '../types';

interface MultiSelectFieldProps {
  field: ConfigField;
  value: unknown;
  onChange: (v: unknown) => void;
  nodeKey?: string;
  fieldLabel: string;
  /** Overrides `field.options` — used by the resource-backed multiselects. */
  options?: { value: string; label: string; description?: string }[];
  /** Shown as the only row when a prerequisite (environment, container, run) is missing. */
  blockedLabel?: string;
  isLoading?: boolean;
}

/**
 * Coerces whatever is stored in `node.config` into the string array the
 * MultiSelect works with. Workflows saved before a field became a
 * multiselect still hold the comma-separated string the executor used to
 * split, and `configStringSlice` on the Go side accepts both shapes, so
 * the migration is purely presentational.
 */
export function toStringList(value: unknown): string[] {
  if (Array.isArray(value)) {
    return value.map((item) => String(item).trim()).filter(Boolean);
  }
  if (typeof value !== 'string') {
    return [];
  }
  const text = value.trim();
  if (text === '') {
    return [];
  }
  if (text.startsWith('[')) {
    try {
      const parsed: unknown = JSON.parse(text);
      if (Array.isArray(parsed)) {
        return parsed.map((item) => String(item).trim()).filter(Boolean);
      }
    } catch {
      // fall through to the comma split
    }
  }
  return text
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean);
}

/** Generic many-of-many config field: static `options`, optional typed entries. */
export function MultiSelectField({
  field,
  value,
  onChange,
  nodeKey,
  fieldLabel,
  options,
  blockedLabel,
  isLoading,
}: MultiSelectFieldProps) {
  const { t } = useTranslation('common');
  const stored = value != null ? value : field.default;
  const list = toStringList(stored);

  const source: { value: string; label: string; description?: string }[] = options ?? field.options ?? [];
  const resolvedOptions = source.map((opt) => ({
    value: opt.value,
    label:
      nodeKey && !options
        ? t(`nodes:${nodeKey}.options.${field.key}.${opt.value}`, {
            // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: workflow node definitions include custom and plugin nodes whose i18n may be absent, so the definition label is the fallback
            defaultValue: opt.label,
          })
        : opt.label,
    description: opt.description,
  }));

  const placeholder = blockedLabel
    ? blockedLabel
    : isLoading
      ? t('workflows.loadingOptions')
      : field.allowCustom
        ? t('select.typeToAddPlaceholder')
        : t('workflows.selectPlaceholder');

  return (
    <div>
      <Label className="mb-1.5 text-xs">
        {fieldLabel}
        {field.required && <span className="text-destructive"> *</span>}
      </Label>
      <MultiSelect
        value={list}
        onChange={(next) => onChange(next)}
        options={resolvedOptions}
        placeholder={placeholder}
        disabled={Boolean(blockedLabel)}
        allowCustom={field.allowCustom}
        ariaLabel={fieldLabel}
        className="w-full text-xs"
      />
    </div>
  );
}
