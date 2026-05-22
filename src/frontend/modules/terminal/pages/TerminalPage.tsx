// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { lazy, Suspense, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconTerminal } from '@tabler/icons-react';
import { PageHeader } from '@resources/layout/PageHeader';
import { Button } from '@resources/components/ui/Button';
import { Select } from '@resources/components/ui/Select';
import { useContainers } from '@resources/hooks/useContainers';
import { truncateId } from '@resources/utils/format';

const TerminalSession = lazy(() => import('../components/TerminalSession'));

export default function TerminalPage() {
  const { t } = useTranslation('terminal');
  const { data: containers = [] } = useContainers();
  const runningContainers = containers.filter((c) => c.State === 'running');

  const [selectedId, setSelectedId] = useState('');
  const [sessionId, setSessionId] = useState('');
  const sessionActive = sessionId.length > 0;

  const handleConnect = () => {
    if (!selectedId) {
      return;
    }

    setSessionId(selectedId);
  };

  const handleDisconnect = () => {
    setSessionId('');
  };

  return (
    <div className="flex h-full flex-col space-y-4">
      <PageHeader title={t('title')} description={t('description')} />

      <div className="flex items-center gap-3">
        <Select
          variant="outline"
          value={selectedId}
          onChange={(value) => {
            setSelectedId(value);
            if (sessionId && value !== sessionId) {
              setSessionId('');
            }
          }}
          placeholder={t('selectContainerPlaceholder')}
          options={runningContainers.map((c) => ({
            value: c.Id,
            label: c.Names?.[0]?.replace(/^\//, '') ?? truncateId(c.Id),
          }))}
          className="w-64"
        />
        <Button onClick={handleConnect} disabled={!selectedId || sessionActive}>
          <IconTerminal className="h-4 w-4" /> {t('connect')}
        </Button>
      </div>

      {sessionActive ? (
        <Suspense
          fallback={
            <div className="min-h-[400px] flex-1 rounded-lg border border-border bg-[#0a0a0a] p-4">
              <div className="h-full min-h-[360px] animate-pulse rounded bg-neutral-900/80" />
            </div>
          }
        >
          <TerminalSession containerId={sessionId} active={sessionActive} onDisconnect={handleDisconnect} />
        </Suspense>
      ) : (
        <div className="flex min-h-[400px] flex-1 items-center justify-center rounded-lg border border-dashed border-border bg-card/40 px-6 text-center text-sm text-muted-foreground">
          {t('description')}
        </div>
      )}
    </div>
  );
}
