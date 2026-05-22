// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { IconSearch } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Spinner } from '@resources/components/ui/Spinner';
import { LogViewer } from '@resources/components/LogViewer';
import { useStackLogs } from '../../hooks/useStacks';

type LogsTabProps = {
  stackName: string;
};

export function LogsTab({ stackName }: LogsTabProps) {
  const { t } = useTranslation('stacks');
  const { data: logs, isLoading } = useStackLogs(stackName, 1000);
  const [activeService, setActiveService] = useState<string | null>(null);
  const [filter, setFilter] = useState('');

  useEffect(() => {
    setActiveService(null);
    setFilter('');
  }, [stackName]);

  const serviceNames = logs ? Object.keys(logs).sort() : [];
  const selectedService = activeService ?? serviceNames[0] ?? null;
  const rawLogs = selectedService && logs ? logs[selectedService] ?? '' : '';
  const logLines = rawLogs.split('\n').filter(Boolean);
  const filteredLines = filter
    ? logLines.filter((line) => line.toLowerCase().includes(filter.toLowerCase()))
    : logLines;

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
        <div className="flex items-center gap-1 overflow-x-auto">
          {serviceNames.map((name) => (
            <Button
              key={name}
              variant={selectedService === name ? 'default' : 'outline'}
              size="sm"
              onClick={() => setActiveService(name)}
            >
              {name}
            </Button>
          ))}
        </div>
        <div className="relative flex-1">
          <IconSearch className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder={t('logs.filterPlaceholder')}
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            className="pl-9"
          />
        </div>
      </div>

      <LogViewer
        lines={filteredLines}
        emptyMessage={filter ? t('logs.noMatchingLines') : t('logs.noLogsAvailable')}
        className="min-h-0 flex-1"
      />
    </div>
  );
}
