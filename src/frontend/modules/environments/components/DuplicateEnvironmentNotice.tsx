// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCopyCheck } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { PERM } from '@core/auth/permissionKeys';
import { useDuplicateEnvironments } from '../hooks/useEnvironmentMigration';
import { MergeEnvironmentsDialog } from './MergeEnvironmentsDialog';

type DuplicateEnvironmentNoticeProps = {
  env: { id: string; name: string };
};

export function DuplicateEnvironmentNotice({ env }: DuplicateEnvironmentNoticeProps) {
  const { t } = useTranslation('environments');
  const { data: groups } = useDuplicateEnvironments();
  const [mergeOpen, setMergeOpen] = useState(false);

  const group = useMemo(
    () => (groups ?? []).find((entry) => entry.members.some((member) => member.id === env.id)),
    [groups, env.id],
  );

  if (!group) return null;

  const others = group.members.filter((member) => member.id !== env.id);
  if (others.length === 0) return null;

  return (
    <>
      <div className="rounded-lg border border-warning/40 bg-warning/10 p-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="flex items-start gap-2">
            <IconCopyCheck className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
            <div>
              <p className="text-sm font-medium text-foreground">
                {t('duplicates.title', { hostname: group.hostname || t('duplicates.thisHost') })}
              </p>
              <p className="mt-1 text-sm text-muted-foreground">
                {t('duplicates.body', {
                  others: others.map((member) => member.name).join(', '),
                })}
              </p>
            </div>
          </div>
          <Button permission={PERM.ENVIRONMENTS_MANAGE} variant="outline" size="sm" onClick={() => setMergeOpen(true)}>
            {t('merge.action')}
          </Button>
        </div>
      </div>

      {mergeOpen && (
        <MergeEnvironmentsDialog
          open={mergeOpen}
          onOpenChange={setMergeOpen}
          target={env}
          candidates={others}
          defaultSourceId={others[0]?.id}
        />
      )}
    </>
  );
}
