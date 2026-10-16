// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconChevronDown, IconChevronRight } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { PlainButton } from '@resources/components/ui/PlainButton';

type DataGridGroupHeaderProps = {
  label: string;
  count: number;
  collapsed: boolean;
  colSpan: number;
  onToggle: () => void;
};

export function DataGridGroupHeader({ label, count, collapsed, colSpan, onToggle }: DataGridGroupHeaderProps) {
  const { t } = useTranslation('common');
  const Chevron = collapsed ? IconChevronRight : IconChevronDown;

  return (
    <tr className="border-b border-border bg-muted/30">
      <td colSpan={colSpan} className="!p-0">
        <PlainButton
          type="button"
          onClick={onToggle}
          aria-expanded={!collapsed}
          aria-label={
            collapsed ? t('dataGrid.expandGroup', { name: label }) : t('dataGrid.collapseGroup', { name: label })
          }
          className="flex w-full items-center gap-2 px-4 py-2 text-left text-sm font-medium text-foreground transition-colors hover:bg-muted/50"
        >
          <Chevron className="size-4 shrink-0 text-muted-foreground" />
          <span className="truncate">{label}</span>
          <Badge variant="secondary">{count}</Badge>
        </PlainButton>
      </td>
    </tr>
  );
}
