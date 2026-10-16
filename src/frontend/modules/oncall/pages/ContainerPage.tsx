// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { Link, useParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconArrowLeft, IconFileText, IconRefresh } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';
import { Button } from '@resources/components/ui/Button';
import { StatusBadge, CONTAINER_STATUS } from '@resources/components/ui/StatusBadge';
import { ContainerFacts } from '../components/ContainerFacts';
import { InboxRow } from '../components/InboxRow';
import { QueryState } from '../components/QueryState';
import { RestartDialog } from '../components/RestartDialog';
import { StaleNotice } from '@resources/components/StaleNotice';
import { useOnline } from '@resources/hooks/useOnline';
import { useInbox } from '../hooks/useInbox';
import { useOnCallContainer } from '../hooks/useOnCallContainer';

export default function ContainerPage() {
  const { t } = useTranslation('oncall');
  const { envId = '', containerId = '' } = useParams();
  const container = useOnCallContainer(envId, containerId);
  const inbox = useInbox();
  const online = useOnline();
  const [restartOpen, setRestartOpen] = useState(false);
  const inspect = container.data?.data;
  const itemName = (inspect?.Name ?? containerId).replace(/^[/]/, '');
  const related = inbox.items
    .filter((item) => item.subjectId === containerId || item.subjectId === inspect?.Id)
    .slice(0, 5);
  const logsPath = `/oncall/containers/${encodeURIComponent(envId)}/${encodeURIComponent(containerId)}/logs`;

  return (
    <section aria-labelledby="oncall-container-title" className="flex flex-col gap-4">
      <div className="flex items-center gap-2">
        <Button asChild variant="ghost" size="icon" className="size-11" aria-label={t('container.back')}>
          <Link to="/oncall/fleet">
            <IconArrowLeft className="size-5" aria-hidden="true" />
          </Link>
        </Button>
        <h1 id="oncall-container-title" className="min-w-0 flex-1 truncate text-lg font-semibold">
          {itemName}
        </h1>
        {inspect && <StatusBadge status={String(inspect.State.Status)} map={CONTAINER_STATUS} />}
      </div>
      <StaleNotice cachedAt={container.data?.cachedAt ?? null} />
      <QueryState
        isLoading={container.isLoading}
        error={container.error}
        onRetry={() => void container.refetch()}
        isEmpty={!inspect}
        emptyText={t('state.loadFailed')}
      >
        {inspect && <ContainerFacts container={inspect} />}
      </QueryState>
      <div className="grid grid-cols-2 gap-2">
        <Button asChild variant="outline" className="min-h-12">
          <Link to={logsPath}>
            <IconFileText className="size-5" aria-hidden="true" />
            {t('container.logs')}
          </Link>
        </Button>
        <Button
          className="min-h-12"
          permission={PERM.CONTAINERS_MANAGE}
          disabled={!online || !inspect}
          title={online ? undefined : t('container.restartOffline')}
          onClick={() => setRestartOpen(true)}
        >
          <IconRefresh className="size-5" aria-hidden="true" />
          {t('container.restart')}
        </Button>
      </div>
      {related.length > 0 && (
        <div>
          <h2 className="mb-2 text-sm font-medium">{t('container.recentAlerts')}</h2>
          <ul className="flex flex-col gap-2">
            {related.map((item) => (
              <InboxRow key={item.id} item={item} />
            ))}
          </ul>
        </div>
      )}
      <RestartDialog
        envId={envId}
        containerId={containerId}
        name={itemName}
        open={restartOpen}
        onOpenChange={setRestartOpen}
      />
    </section>
  );
}
