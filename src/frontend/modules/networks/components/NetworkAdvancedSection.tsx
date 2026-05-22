// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconChevronDown, IconChevronRight } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Label } from '@resources/components/ui/Label';
import { KeyValueRows } from './KeyValueRows';
import type { KeyValuePair } from './KeyValueRows';

interface NetworkAdvancedSectionProps {
  open: boolean;
  onToggle: () => void;
  customOptions: KeyValuePair[];
  onCustomOptionsChange: (items: KeyValuePair[]) => void;
  labels: KeyValuePair[];
  onLabelsChange: (items: KeyValuePair[]) => void;
}

export function NetworkAdvancedSection({
  open,
  onToggle,
  customOptions,
  onCustomOptionsChange,
  labels,
  onLabelsChange,
}: NetworkAdvancedSectionProps) {
  const { t } = useTranslation('networks');

  return (
    <div className="border-t border-border pt-4">
      <Button
        type="button"
        variant="ghost"
        size="sm"
        onClick={onToggle}
        className="flex items-center gap-1 text-sm font-medium text-muted-foreground hover:text-foreground transition-colors"
      >
        {open ? (
          <IconChevronDown className="h-4 w-4" />
        ) : (
          <IconChevronRight className="h-4 w-4" />
        )}
        {t('create.advanced')}
      </Button>
      {open && (
        <div className="mt-3 space-y-4">
          <div>
            <Label className="mb-2">
              {t('create.customOptions')}
            </Label>
            <p className="text-xs text-muted-foreground mb-2">
              {t('create.customOptionsHelp')}
            </p>
            <KeyValueRows
              items={customOptions}
              onChange={onCustomOptionsChange}
              keyPlaceholder={t('create.optionKey')}
              valuePlaceholder={t('create.optionValue')}
              addLabel={t('create.add')}
            />
          </div>
          <div>
            <Label className="mb-2">{t('create.labels')}</Label>
            <KeyValueRows
              items={labels}
              onChange={onLabelsChange}
              keyPlaceholder={t('create.labelKey')}
              valuePlaceholder={t('create.labelValue')}
              addLabel={t('create.add')}
            />
          </div>
        </div>
      )}
    </div>
  );
}
