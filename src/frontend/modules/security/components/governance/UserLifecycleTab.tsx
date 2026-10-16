// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconUserCheck } from '@tabler/icons-react';
import { SettingsGroup } from '@resources/components/settings/SettingsGroup';
import { useUserLifecycle, type BulkAction, type LifecycleUser } from '../../hooks/useUserLifecycle';
import { BulkUserActionDialog } from './BulkUserActionDialog';
import { LifecycleUserTable, useLifecycleTableFilters } from './LifecycleUserTable';

type PendingBulk = { action: BulkAction; userIds: string[] };

const NO_USERS: LifecycleUser[] = [];

export function UserLifecycleTab() {
  const { t } = useTranslation('security');
  const [inactiveDays, setInactiveDays] = useState(90);
  const [pending, setPending] = useState<PendingBulk | null>(null);
  const [tableGeneration, setTableGeneration] = useState(0);
  const filters = useLifecycleTableFilters();
  const report = useUserLifecycle(inactiveDays, 7);

  const onBulkAction = useCallback((action: BulkAction, users: LifecycleUser[]) => {
    setPending({ action, userIds: users.map((u) => u.id) });
  }, []);

  return (
    <SettingsGroup
      icon={IconUserCheck}
      title={t('governance.lifecycle.title')}
      description={t('governance.lifecycle.description')}
    >
      <LifecycleUserTable
        key={tableGeneration}
        users={report.data?.users ?? NO_USERS}
        counts={report.data?.counts}
        loading={report.isLoading}
        fetching={report.isFetching}
        error={report.error}
        onRefresh={() => void report.refetch()}
        filters={filters}
        inactiveDays={inactiveDays}
        onInactiveDaysChange={setInactiveDays}
        onBulkAction={onBulkAction}
      />

      <BulkUserActionDialog
        action={pending?.action ?? null}
        userIds={pending?.userIds ?? []}
        onOpenChange={(isOpen) => !isOpen && setPending(null)}
        onDone={() => setTableGeneration((n) => n + 1)}
      />
    </SettingsGroup>
  );
}
