// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import { IconClockHour4, IconDeviceFloppy } from '@tabler/icons-react';
import { toast } from 'sonner';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { usePermission } from '@core/auth/usePermission';
import { PERM } from '@core/auth/permissionKeys';
import {
  useAuditRetention,
  useUpdateAuditRetention,
  type AuditRetentionView,
  type AuditSeverity,
} from '../hooks/useAuditCenter';
import { AuditRetentionRow, type RetentionDraftRow } from './AuditRetentionRow';

type Draft = Record<AuditSeverity, RetentionDraftRow>;

function draftFrom(view: AuditRetentionView): Draft {
  const draft = {} as Draft;
  for (const severity of view.severities) {
    const override = view.overrides[severity];
    draft[severity] =
      override === null || override === undefined
        ? { inherit: true, days: view.defaultDays }
        : { inherit: false, days: override };
  }
  return draft;
}

export function changedOverrides(
  view: AuditRetentionView,
  draft: Draft,
): Partial<Record<AuditSeverity, number | null>> {
  const changes: Partial<Record<AuditSeverity, number | null>> = {};
  for (const severity of view.severities) {
    const row = draft[severity];
    const next = row.inherit ? null : row.days;
    const current = view.overrides[severity] ?? null;
    if (next !== current) changes[severity] = next;
  }
  return changes;
}

function RetentionForm({ view }: { view: AuditRetentionView }) {
  const { t } = useTranslation('audit');
  const canManage = usePermission(PERM.SETTINGS_MANAGE);
  const [draft, setDraft] = useState<Draft>(() => draftFrom(view));
  const update = useUpdateAuditRetention();
  const changes = useMemo(() => changedOverrides(view, draft), [view, draft]);
  const dirty = Object.keys(changes).length > 0;

  const save = () => {
    if (!dirty || update.isPending) return;
    update.mutate(changes, {
      onSuccess: () => toast.success(t('retention.saved')),
    });
  };

  return (
    <div className="space-y-3">
      {view.severities.map((severity) => (
        <AuditRetentionRow
          key={severity}
          severity={severity}
          row={draft[severity]}
          defaultDays={view.defaultDays}
          maxDays={view.maxDays}
          disabled={!canManage || update.isPending}
          onChange={(next) => setDraft((current) => ({ ...current, [severity]: next }))}
        />
      ))}
      {update.isError && (
        <Alert variant="destructive" title={t('retention.saveFailed')} description={update.error.message} />
      )}
      <div className="flex justify-end">
        <Button permission={PERM.SETTINGS_MANAGE} onClick={save} disabled={!dirty || update.isPending}>
          {update.isPending ? <Spinner size="sm" /> : <IconDeviceFloppy className="size-4" />}
          {t('retention.save')}
        </Button>
      </div>
    </div>
  );
}

export function AuditRetentionPanel() {
  const { t } = useTranslation('audit');
  const { data, isLoading, isError, error } = useAuditRetention();

  return (
    <div className="mc-glass space-y-4 rounded-lg p-6">
      <div>
        <h3 className="flex items-center gap-2 text-sm font-medium text-foreground">
          <IconClockHour4 className="size-4 text-muted-foreground" aria-hidden="true" />
          {t('retention.title')}
        </h3>
        <p className="mt-1 text-xs text-muted-foreground">{t('retention.description')}</p>
      </div>
      {isLoading && <Spinner size="sm" />}
      {isError && <Alert variant="destructive" title={t('retention.loadFailed')} description={error.message} />}
      {data && (
        <>
          <p className="text-sm text-foreground">
            {t('retention.default')}:{' '}
            <span className="font-medium">
              {data.defaultDays === 0 ? t('retention.forever') : t('retention.days', { count: data.defaultDays })}
            </span>
          </p>
          <p className="text-xs text-muted-foreground">
            {t('retention.defaultHint')}{' '}
            <Link to="/settings?tab=retention" className="text-primary underline">
              {t('retention.openSettings')}
            </Link>
          </p>
          <RetentionForm key={JSON.stringify(data.overrides) + data.defaultDays} view={data} />
          <p className="text-xs text-muted-foreground">
            {t('retention.zeroHint')} {t('retention.chainNote')}
          </p>
        </>
      )}
    </div>
  );
}
