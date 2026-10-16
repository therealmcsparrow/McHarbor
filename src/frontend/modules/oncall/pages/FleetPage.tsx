// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { FleetCard } from '../components/FleetCard';
import { QueryState } from '../components/QueryState';
import { StaleNotice } from '@resources/components/StaleNotice';
import { useFleet } from '../hooks/useFleet';

export default function FleetPage() {
  const { t } = useTranslation('oncall');
  const fleet = useFleet();
  return (
    <section aria-labelledby="oncall-fleet-title">
      <h1 id="oncall-fleet-title" className="mb-3 text-lg font-semibold">
        {t('fleet.title')}
      </h1>
      <StaleNotice cachedAt={fleet.cachedAt} />
      <QueryState
        isLoading={fleet.isLoading}
        error={fleet.error}
        onRetry={() => void fleet.refetch()}
        isEmpty={fleet.entries.length === 0}
        emptyText={t('fleet.empty')}
      >
        <ul className="flex flex-col gap-2">
          {fleet.entries.map((entry) => (
            <FleetCard key={entry.environment.id} entry={entry} />
          ))}
        </ul>
      </QueryState>
    </section>
  );
}
