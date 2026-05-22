// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { cn } from '@resources/utils/cn';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Button } from '@resources/components/ui/Button';
import { CronSchedulePreview } from '@resources/components/CronSchedulePreview';
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
  const [showPresets, setShowPresets] = useState(false);

  const CRON_PRESETS = [
    { label: t('workflows.cronEveryMinute'), value: '* * * * *' },
    { label: t('workflows.cronEvery5Minutes'), value: '*/5 * * * *' },
    { label: t('workflows.cronEvery15Minutes'), value: '*/15 * * * *' },
    { label: t('workflows.cronEveryHour'), value: '0 * * * *' },
    { label: t('workflows.cronEveryDayMidnight'), value: '0 0 * * *' },
    { label: t('workflows.cronEveryMonday9AM'), value: '0 9 * * 1' },
    { label: t('workflows.cronEvery1stOfMonth'), value: '0 0 1 * *' },
  ];

  const activePreset = CRON_PRESETS.find((p) => p.value === value);

  return (
    <div>
      <Label className="mb-1.5 text-xs">{fieldLabel}{field.required && <span className="text-destructive"> *</span>}</Label>
      <Input
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="h-8 font-mono text-xs"
        placeholder="* * * * *"
      />
      {activePreset && (
        <p className="mt-1 text-[10px] text-muted-foreground">{activePreset.label}</p>
      )}
      <Button
        variant="link"
        onClick={() => setShowPresets(!showPresets)}
        className="mt-1 h-auto p-0 text-[10px]"
      >
        {showPresets ? t('workflows.hidePresets') : t('workflows.showPresets')}
      </Button>
      {showPresets && (
        <div className="mt-1.5 space-y-0.5">
          {CRON_PRESETS.map((preset) => (
            <Button
              key={preset.value}
              variant="ghost"
              onClick={() => { onChange(preset.value); setShowPresets(false); }}
              className={cn(
                'flex w-full items-center justify-between rounded-md px-2 py-1 text-xs h-auto',
                value === preset.value && 'bg-muted/50 text-foreground',
              )}
            >
              <span>{preset.label}</span>
              <span className="font-mono text-[10px] text-muted-foreground">{preset.value}</span>
            </Button>
          ))}
        </div>
      )}
      <CronSchedulePreview expression={value} timezone={timezone} />
    </div>
  );
}

