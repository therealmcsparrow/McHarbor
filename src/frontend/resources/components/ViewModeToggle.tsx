// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// ViewModeToggle — the table/timeline switch that sits in a page's topbar.
//
// The list pages that offer a card grid each grew their own two-button toggle
// (containers, images, networks, stacks, volumes, environments). The log pages
// switch between a table and a timeline instead, and there are two of them, so
// the control lives here rather than being written twice more.

import { IconLayoutList, IconTimeline } from '@tabler/icons-react';
import { useTranslation } from 'react-i18next';
import { Button } from './ui/Button';
import { Tooltip, TooltipContent, TooltipTrigger } from './ui/Tooltip';

export type ListViewMode = 'table' | 'timeline';

type ViewModeToggleProps = {
  value: ListViewMode;
  onChange: (mode: ListViewMode) => void;
};

const MODES: readonly { mode: ListViewMode; icon: typeof IconLayoutList; labelKey: string }[] = [
  { mode: 'table', icon: IconLayoutList, labelKey: 'view.table' },
  { mode: 'timeline', icon: IconTimeline, labelKey: 'view.timeline' },
];

export function ViewModeToggle({ value, onChange }: ViewModeToggleProps) {
  const { t } = useTranslation('common');

  return (
    <fieldset
      className="m-0 flex min-w-0 items-center rounded-lg border border-border p-0"
      aria-label={t('view.label')}
    >
      {MODES.map(({ mode, icon: Icon, labelKey }) => (
        <Tooltip key={mode}>
          <TooltipTrigger asChild>
            <Button
              variant={value === mode ? 'default' : 'ghost'}
              size="icon-sm"
              aria-pressed={value === mode}
              onClick={() => onChange(mode)}
              aria-label={t(labelKey)}
            >
              <Icon className="size-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t(labelKey)}</TooltipContent>
        </Tooltip>
      ))}
    </fieldset>
  );
}
