// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { PERM } from '@core/auth/permissionKeys';
import { InboxRow } from '../components/InboxRow';
import { QueryState } from '../components/QueryState';
import { StaleNotice } from '@resources/components/StaleNotice';
import { useInbox } from '../hooks/useInbox';

export default function AlertsPage() {
  const { t } = useTranslation('oncall');
  const inbox = useInbox();
  return (
    <section aria-labelledby="oncall-alerts-title">
      <h1 id="oncall-alerts-title" className="mb-3 text-lg font-semibold">
        {t('alerts.title')}
      </h1>
      <StaleNotice cachedAt={inbox.cachedAt} />
      <QueryState
        isLoading={inbox.isLoading}
        error={inbox.error}
        onRetry={() => void inbox.refetch()}
        missingPermission={inbox.canRead ? null : PERM.ALERTS_VIEW}
        isEmpty={inbox.items.length === 0}
        emptyText={t('alerts.empty')}
      >
        <ul className="flex flex-col gap-2">
          {inbox.items.map((item) => (
            <InboxRow key={item.id} item={item} />
          ))}
        </ul>
      </QueryState>
    </section>
  );
}
