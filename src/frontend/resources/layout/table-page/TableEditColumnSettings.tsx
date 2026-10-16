// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAdjustmentsHorizontal } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Popover, PopoverContent, PopoverTrigger } from '@resources/components/ui/Popover';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { TABLE_COLUMN_WIDTHS, type TableColumnAlign, type TableColumnWidth } from './types';

const ALIGN_VALUES: TableColumnAlign[] = ['auto', 'left', 'center', 'right'];

type TableEditColumnSettingsProps = {
  id: string;
  label: string;
  size: TableColumnWidth;
  headerAlign: TableColumnAlign;
  cellAlign: TableColumnAlign;
  onResize: (id: string, size: TableColumnWidth) => void;
  onAlign: (id: string, part: 'header' | 'cell', value: TableColumnAlign) => void;
  /** Undefined for a column that cannot be ordered by — actions, selection, display-only cells. */
  sortable?: boolean;
  onSortable: (id: string, value: boolean) => void;
};

export function TableEditColumnSettings({
  id,
  label,
  size,
  headerAlign,
  cellAlign,
  onResize,
  onAlign,
  sortable,
  onSortable,
}: TableEditColumnSettingsProps) {
  const { t } = useTranslation('common');
  const sizeOptions = (['auto', ...TABLE_COLUMN_WIDTHS] as TableColumnWidth[]).map((value) => ({
    value,
    label: t(`tableViews.size.${value}`),
  }));
  const alignOptions = ALIGN_VALUES.map((value) => ({ value, label: t(`tableViews.align.${value}`) }));

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label={t('tableViews.columnSettings', { column: label })}>
          <IconAdjustmentsHorizontal className="size-4" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="flex w-64 flex-col gap-3">
        <div className="flex flex-col gap-1">
          <span className="text-xs font-medium text-muted-foreground">{t('tableViews.width')}</span>
          <Select
            value={size}
            onChange={(value) => onResize(id, value as TableColumnWidth)}
            options={sizeOptions}
            searchable={false}
            variant="outline"
            ariaLabel={t('tableViews.width')}
          />
        </div>
        <div className="flex flex-col gap-1">
          <span className="text-xs font-medium text-muted-foreground">{t('tableViews.headerAlign')}</span>
          <Select
            value={headerAlign}
            onChange={(value) => onAlign(id, 'header', value as TableColumnAlign)}
            options={alignOptions}
            searchable={false}
            variant="outline"
            ariaLabel={t('tableViews.headerAlign')}
          />
        </div>
        <div className="flex flex-col gap-1">
          <span className="text-xs font-medium text-muted-foreground">{t('tableViews.cellAlign')}</span>
          <Select
            value={cellAlign}
            onChange={(value) => onAlign(id, 'cell', value as TableColumnAlign)}
            options={alignOptions}
            searchable={false}
            variant="outline"
            ariaLabel={t('tableViews.cellAlign')}
          />
        </div>
        {sortable !== undefined && (
          <div className="flex items-start justify-between gap-3">
            <div className="flex flex-col">
              <span className="text-xs font-medium text-muted-foreground">{t('tableViews.sortable')}</span>
              <span className="text-xs text-muted-foreground">{t('tableViews.sortableHint')}</span>
            </div>
            <Switch
              checked={sortable}
              aria-label={t('tableViews.sortable')}
              onCheckedChange={(value) => onSortable(id, value)}
            />
          </div>
        )}
      </PopoverContent>
    </Popover>
  );
}
