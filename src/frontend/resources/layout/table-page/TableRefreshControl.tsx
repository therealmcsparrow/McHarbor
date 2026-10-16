// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { IconRefresh } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';
import { AUTO_REFRESH_OPTIONS } from './types';

type TableRefreshControlProps = {
  onRefresh: () => void;
  isFetching: boolean;
  intervalMs: number;
  onIntervalChange: (ms: number) => void;
};

export function TableRefreshControl({ onRefresh, isFetching, intervalMs, onIntervalChange }: TableRefreshControlProps) {
  const { t } = useTranslation('common');

  // Held in a ref so a changing callback identity does not restart the interval
  // — otherwise a page that recreates `refetch` each render would reset the
  // countdown on every keystroke in the search box.
  const refreshRef = useRef(onRefresh);
  useEffect(() => {
    refreshRef.current = onRefresh;
  }, [onRefresh]);

  useEffect(() => {
    if (intervalMs <= 0) return undefined;
    const timer = window.setInterval(() => {
      // A backgrounded tab should not keep polling the API.
      if (document.visibilityState === 'visible') {
        refreshRef.current();
      }
    }, intervalMs);
    return () => window.clearInterval(timer);
  }, [intervalMs]);

  const options = AUTO_REFRESH_OPTIONS.map((ms) => ({
    value: String(ms),
    label: ms === 0 ? t('table.autoRefreshOff') : t('table.autoRefreshSeconds', { count: ms / 1000 }),
  }));

  return (
    <div className="flex items-center gap-1.5">
      <Button
        variant="outline"
        size="icon-sm"
        aria-label={t('table.refresh')}
        disabled={isFetching}
        onClick={onRefresh}
      >
        {isFetching ? <Spinner size="sm" /> : <IconRefresh className="size-4" />}
      </Button>
      <Select
        value={String(intervalMs)}
        onChange={(value) => onIntervalChange(Number(value))}
        options={options}
        searchable={false}
        variant="outline"
        ariaLabel={t('table.autoRefresh')}
        className="min-w-24"
      />
    </div>
  );
}
