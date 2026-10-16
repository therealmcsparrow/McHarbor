// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconEye, IconEyeOff } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import type { ConfigField } from '../types';

interface ConfigTextFieldProps {
  field: ConfigField;
  strVal: string;
  onChange: (v: unknown) => void;
  fieldLabel: string;
}

function maskSecretValue(value: string): string {
  if (!value) {
    return '';
  }

  const visibleLength = Math.min(value.length, 32);
  const masked = '•'.repeat(visibleLength);
  return value.length > visibleLength ? `${masked}…` : masked;
}

export function ConfigTextField({ field, strVal, onChange, fieldLabel }: ConfigTextFieldProps) {
  const { t } = useTranslation('common');
  const [showSecret, setShowSecret] = useState(false);
  const secretToggleLabel = showSecret
    ? t('workflows.hideSensitiveField', { field: fieldLabel })
    : t('workflows.showSensitiveField', { field: fieldLabel });

  if (field.type === 'textarea') {
    return (
      <div>
        <div className="mb-1.5 flex items-center justify-between gap-2">
          <Label className="text-xs">
            {fieldLabel}
            {field.required && <span className="text-destructive"> *</span>}
          </Label>
          {field.secret ? (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              aria-label={secretToggleLabel}
              onClick={() => setShowSecret((current) => !current)}
            >
              {showSecret ? t('workflows.hide') : t('workflows.show')}
            </Button>
          ) : null}
        </div>
        <textarea
          value={field.secret && !showSecret ? maskSecretValue(strVal) : strVal}
          onChange={(e) => onChange(e.target.value)}
          rows={3}
          className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-xs ring-offset-background placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
          readOnly={field.secret && !showSecret}
        />
      </div>
    );
  }

  return (
    <div>
      <Label className="mb-1.5 text-xs">
        {fieldLabel}
        {field.required && <span className="text-destructive"> *</span>}
      </Label>
      <div className="flex items-center gap-2">
        <Input
          type={field.secret && !showSecret ? 'password' : 'text'}
          value={strVal}
          onChange={(e) => onChange(e.target.value)}
          className={cn('text-xs', field.type === 'expression' && 'font-mono')}
        />
        {field.secret ? (
          <Button
            type="button"
            variant="outline"
            size="icon-sm"
            aria-label={secretToggleLabel}
            onClick={() => setShowSecret((current) => !current)}
          >
            {showSecret ? <IconEyeOff className="size-3.5" /> : <IconEye className="size-3.5" />}
          </Button>
        ) : null}
      </div>
    </div>
  );
}
