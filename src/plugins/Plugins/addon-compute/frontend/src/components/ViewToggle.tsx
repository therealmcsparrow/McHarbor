// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconLayoutGrid, IconLayoutList } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import type { ViewMode } from '../hooks/useComputeView';

/** The table/card switch, in the same shape the environments page uses. */
export function ViewToggle({ viewMode, setViewMode }: { viewMode: ViewMode; setViewMode: (next: ViewMode) => void }) {
  const { t } = useTranslation('compute');

  return (
    <div className="flex items-center rounded-lg border border-border">
      <Button
        variant={viewMode === 'table' ? 'default' : 'ghost'}
        size="icon-sm"
        onClick={() => setViewMode('table')}
        aria-label={t('overview.tableView')}
      >
        <IconLayoutList className="h-4 w-4" />
      </Button>
      <Button
        variant={viewMode === 'card' ? 'default' : 'ghost'}
        size="icon-sm"
        onClick={() => setViewMode('card')}
        aria-label={t('overview.cardView')}
      >
        <IconLayoutGrid className="h-4 w-4" />
      </Button>
    </div>
  );
}
