// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { SwarmGuard } from '../components/SwarmGuard';
import { SwarmTaskTable } from '../components/SwarmTaskTable';

export default function SwarmTasksPage() {
  const { t } = useTranslation('swarm');

  return (
    <SwarmGuard>
      <SwarmTaskTable id="swarm-tasks" title={t('tasks.title')} description={t('tasks.description')} />
    </SwarmGuard>
  );
}
