// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import type { SyncAction, SyncPlan, SyncSummary, UserChange } from '@resources/hooks/useScimEndpoints';

const MAX_ROWS = 200;

const ACTION_VARIANT: Record<SyncAction, 'success' | 'info' | 'warning' | 'destructive' | 'neutral'> = {
  link: 'success',
  unmatched: 'neutral',
  update: 'info',
  disable: 'destructive',
  skip: 'warning',
};

const SUMMARY_KEYS: (keyof SyncSummary)[] = [
  'usersSeen',
  'linked',
  'unmatched',
  'updated',
  'disabled',
  'skipped',
  'unchanged',
  'pendingLink',
];

function ChangeDetail({ change }: { change: UserChange }) {
  const { t } = useTranslation('scim');
  const parts: string[] = [];
  if (change.localUsername) parts.push(t('change.account', { username: change.localUsername }));
  if (change.action === 'unmatched') parts.push(t('change.noAccount'));
  if (change.fields && change.fields.length > 0) {
    parts.push(t('change.fields', { fields: change.fields.map((field) => t(`field.${field}`)).join(', ') }));
  }
  if (change.reason) parts.push(t(`reason.${change.reason}`));
  return <span className="text-xs text-muted-foreground">{parts.join(' · ')}</span>;
}

export function SyncPlanView({ plan }: { plan: SyncPlan }) {
  const { t } = useTranslation('scim');
  const matchedGroups = plan.groups.filter((group) => group.localGroupId).length;
  const rows = plan.users.slice(0, MAX_ROWS);

  return (
    <div className="space-y-4">
      <dl className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        {SUMMARY_KEYS.map((key) => (
          <div key={key} className="rounded-md border border-border p-2">
            <dt className="text-xs text-muted-foreground">{t(`summary.${key}`)}</dt>
            <dd className="text-lg font-semibold text-foreground">{plan.summary[key]}</dd>
          </div>
        ))}
      </dl>

      {rows.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('preview.noChanges')}</p>
      ) : (
        <ul
          className="max-h-[40vh] divide-y divide-border overflow-y-auto rounded-md border border-border"
          aria-label={t('preview.changes')}
        >
          {rows.map((change) => (
            <li key={`${change.action}-${change.scimId}`} className="flex flex-wrap items-center gap-2 px-3 py-2">
              <Badge variant={ACTION_VARIANT[change.action]}>{t(`action.${change.action}`)}</Badge>
              <span className="font-medium text-foreground">{change.userName}</span>
              {change.email && <span className="text-xs text-muted-foreground">{change.email}</span>}
              <ChangeDetail change={change} />
            </li>
          ))}
        </ul>
      )}
      {plan.users.length > MAX_ROWS && (
        <p className="text-xs text-muted-foreground">{t('preview.more', { count: plan.users.length - MAX_ROWS })}</p>
      )}
      <p className="text-xs text-muted-foreground">
        {t('preview.groups', { count: plan.summary.groupsSeen, matched: matchedGroups })}
      </p>
    </div>
  );
}
