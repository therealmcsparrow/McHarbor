// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconSearch, IconSortAscending, IconSortDescending, IconPlayerPlay, IconPlayerStop, IconDownload } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Spinner } from '@resources/components/ui/Spinner';
import { LogViewer } from '@resources/components/LogViewer';
import { useContainerLogs } from '../../hooks/useContainerLogs';

type LogsTabProps = {
  containerId: string;
  isRunning: boolean;
};

export function LogsTab({ containerId, isRunning }: LogsTabProps) {
  const { t } = useTranslation('containers');
  const [newestFirst, setNewestFirst] = useState(true);
  const [live, setLive] = useState(false);
  const [filter, setFilter] = useState('');
  const { data: logs = '', isLoading } = useContainerLogs(containerId, 500, live);

  const logLines = typeof logs === 'string' ? logs.split('\n') : [];
  const orderedLines = newestFirst ? [...logLines].reverse() : logLines;
  const filteredLines = filter
    ? orderedLines.filter((line) => line.toLowerCase().includes(filter.toLowerCase()))
    : orderedLines;

  // Only auto-scroll in oldest-first + live mode
  const autoScroll = !newestFirst && live;

  const handleExportLogs = () => {
    const content = filteredLines.join('\n');
    const blob = new Blob([content], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `container-${containerId.slice(0, 12)}.log`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  };

  if (isLoading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3">
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
          onClick={() => setNewestFirst((prev) => !prev)}
        >
          {newestFirst
            ? <><IconSortDescending className="h-4 w-4" /> {t('logs.newestFirst')}</>
            : <><IconSortAscending className="h-4 w-4" /> {t('logs.oldestFirst')}</>
          }
        </Button>
        <Button
          variant={live ? 'default' : 'outline'}
          size="sm"
          onClick={() => setLive((prev) => !prev)}
          disabled={!isRunning}
        >
          {live
            ? <><IconPlayerStop className="h-4 w-4" /> {t('logs.live')}</>
            : <><IconPlayerPlay className="h-4 w-4" /> {t('logs.live')}</>
          }
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={handleExportLogs}
          disabled={filteredLines.length === 0}
        >
          <IconDownload className="h-4 w-4" /> {t('logs.export')}
        </Button>
      </div>

      {!isRunning && (
        <div className="rounded-md border border-yellow-500/30 bg-yellow-500/10 px-3 py-2 text-xs text-yellow-500">
          {t('logs.notRunningWarning')}
        </div>
      )}

      <LogViewer
        lines={filteredLines}
        emptyMessage={filter ? t('logs.noMatchingLines') : t('logs.noLogsAvailable')}
        autoScroll={autoScroll}
        className="min-h-0 flex-1"
      />
    </div>
  );
}
