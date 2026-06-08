// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { ReadableScheduleField } from '@resources/components/ReadableScheduleField';
import type { ConfigField } from '../types';

interface CronFieldProps {
  field: ConfigField;
  value: string;
  onChange: (v: unknown) => void;
  nodeKey?: string;
  timezone?: string | null;
}

export function CronField({ field, value, onChange, nodeKey, timezone }: CronFieldProps) {
  const { t } = useTranslation('common');
  const fieldLabel = nodeKey ? t(`nodes:${nodeKey}.config.${field.key}`, { defaultValue: field.label }) : field.label;

  return (
    <ReadableScheduleField
      value={value}
      onChange={(next) => onChange(next)}
      label={`${fieldLabel}${field.required ? ' *' : ''}`}
      timezone={timezone}
    />
  );
}

