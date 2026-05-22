// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconPlayerPlay,
  IconPlayerStop,
  IconSearch,
  IconSortAscending,
  IconSortDescending,
} from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { LogViewer } from '@resources/components/LogViewer';
import { useContainerLogs } from '../hooks/useContainerLogs';

type LogsModalProps = {
  containerId: string;
  isRunning: boolean;
};

export function LogsModal({ containerId, isRunning }: LogsModalProps) {
  const { t } = useTranslation('containers');
  const [newestFirst, setNewestFirst] = useState(true);
  const [live, setLive] = useState(false);
  const [filter, setFilter] = useState('');
  const { data: logs = '' } = useContainerLogs(containerId, 500, live);

  const logLines = typeof logs === 'string' ? logs.split('\n') : [];
  const orderedLines = newestFirst ? [...logLines].reverse() : logLines;
  const filteredLines = filter
    ? orderedLines.filter((line) => line.toLowerCase().includes(filter.toLowerCase()))
    : orderedLines;

  const autoScroll = !newestFirst && live;

  return (
    <div className="flex flex-col gap-3 px-4 pb-4">
      <div className="flex items-center gap-3">
        <div className="relative flex-1">
          <IconSearch className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder={t('logs.filterPlaceholder')}
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            className="pl-9"
          />
        </div>
        <Button
          variant={newestFirst ? 'default' : 'outline'}
          size="sm"
          onClick={() => setNewestFirst((p) => !p)}
        >
          {newestFirst ? (
            <>
              <IconSortDescending className="h-4 w-4" /> {t('logs.newest')}
            </>
          ) : (
            <>
              <IconSortAscending className="h-4 w-4" /> {t('logs.oldest')}
            </>
          )}
        </Button>
        <Button
          variant={live ? 'default' : 'outline'}
          size="sm"
          onClick={() => setLive((p) => !p)}
          disabled={!isRunning}
        >
          {live ? (
            <>
              <IconPlayerStop className="h-4 w-4" /> {t('logs.live')}
            </>
          ) : (
            <>
              <IconPlayerPlay className="h-4 w-4" /> {t('logs.live')}
            </>
          )}
        </Button>
      </div>
      <LogViewer
        lines={filteredLines}
        emptyMessage={filter ? t('logs.noMatchingLines') : t('logs.noLogsAvailable')}
        autoScroll={autoScroll}
      />
    </div>
  );
}
