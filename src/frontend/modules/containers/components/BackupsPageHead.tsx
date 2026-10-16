// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconHistory } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { PERM } from '@core/auth/permissionKeys';
import { PageHeadActions, PageHeadIdentity } from '@resources/layout/PageHeadIdentity';
import { EnvironmentScopeFilter } from '@resources/components/EnvironmentScopeFilter';
import { BackupsSectionMeta } from './BackupsSectionMeta';
import { backupsTabIcons, backupsTabLabelKeys, type BackupsTabId } from './backups-tab-groups';

type BackupsPageHeadProps = {
  tab: BackupsTabId;
  scopeEnvId: string;
  onScopeChange: (envId: string) => void;
  environmentName: string;
  onRelinkAll: () => void;
  relinkPending: boolean;
};

export function BackupsPageHead({
  tab,
  scopeEnvId,
  onScopeChange,
  environmentName,
  onRelinkAll,
  relinkPending,
}: BackupsPageHeadProps) {
  const { t } = useTranslation('containers');
  const HeaderIcon = backupsTabIcons[tab];

  return (
    <>
      <PageHeadIdentity>
        <span
          aria-hidden="true"
          className="grid size-11 shrink-0 place-items-center rounded-xl bg-primary/12 ring-1 ring-primary/25 ring-inset"
        >
          <HeaderIcon className="size-6 text-primary" />
        </span>
        <div className="min-w-0">
          <h1 className="truncate font-display text-2xl leading-tight font-semibold tracking-tight text-foreground">
            {t(backupsTabLabelKeys[tab])}
          </h1>
          <BackupsSectionMeta activeTab={tab} environmentName={environmentName} />
        </div>
      </PageHeadIdentity>

      <PageHeadActions>
        <div className="flex flex-wrap items-center justify-end gap-1.5">
          <EnvironmentScopeFilter value={scopeEnvId} onChange={onScopeChange} />
          <Button
            variant="outline"
            size="sm"
            permission={PERM.BACKUPS_MANAGE}
            onClick={onRelinkAll}
            disabled={relinkPending}
            data-testid="backups-relink-all"
          >
            <IconHistory className="size-4" aria-hidden="true" />
            {t('backups.relinkAll')}
          </Button>
        </div>
      </PageHeadActions>
    </>
  );
}
