// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconClock, IconPencil, IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { StatusError } from '@resources/components/ui/StatusError';
import { PERM } from '@core/auth/permissionKeys';
import { describeCron } from '@resources/utils/schedule';
import type { Runbook } from '@resources/types/runbooks';
import { RunbookExportMenu } from './RunbookExportMenu';
import { useAttachLabel } from '../hooks/useAttachLabel';

type RunbookCardGridProps = {
  runbooks: Runbook[];
  onEdit: (id: string) => void;
  onRemove: (id: string) => void;
};

export function RunbookCardGrid({ runbooks, onEdit, onRemove }: RunbookCardGridProps) {
  const { t } = useTranslation('runbooks');
  const { t: tCommon } = useTranslation('common');
  const attachLabel = useAttachLabel();

  return (
    <div className="grid gap-4 p-4 sm:grid-cols-2 xl:grid-cols-3">
      {runbooks.map((runbook) => (
        <div
          key={runbook.id}
          className="group flex flex-col gap-3 rounded-xl border border-border bg-card p-4 transition-colors hover:border-primary/30"
        >
          <div className="flex items-start justify-between gap-2">
            <div className="min-w-0">
              <h3 className="truncate font-medium text-foreground">{runbook.name}</h3>
              <p className="mt-0.5 line-clamp-2 text-sm text-muted-foreground">{runbook.description}</p>
            </div>
            <Badge variant={runbook.enabled ? 'success' : 'secondary'} className="shrink-0">
              {runbook.enabled ? t('list.enabled') : t('list.disabled')}
            </Badge>
          </div>

          <div className="flex flex-wrap items-center gap-1.5">
            <Badge variant="secondary">{attachLabel(runbook.attachKind).label}</Badge>
            <Badge variant="outline">{t('list.stepCount', { count: runbook.steps.length })}</Badge>
            {runbook.automation?.onAlert && <Badge variant="outline">{t('list.automationOnAlert')}</Badge>}
          </div>

          {runbook.automation?.cron && (
            <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <IconClock className="size-3.5 shrink-0" aria-hidden="true" />
              <span className="truncate">{describeCron(runbook.automation.cron, tCommon)}</span>
              <span className="shrink-0 font-mono opacity-70">{runbook.automation.cron}</span>
            </p>
          )}

          <div className="mt-auto flex items-center justify-between gap-2 pt-1">
            <StatusError error={runbook.automation?.lastError} />
            <div className="ml-auto flex items-center gap-1.5">
              <Button variant="outline" size="sm" permission={PERM.RUNBOOKS_MANAGE} onClick={() => onEdit(runbook.id)}>
                <IconPencil className="size-4" aria-hidden="true" />
                {t('list.edit')}
              </Button>
              <RunbookExportMenu id={runbook.id} name={runbook.name} />
              <Button
                variant="ghost"
                size="icon"
                aria-label={t('list.delete')}
                permission={PERM.RUNBOOKS_MANAGE}
                onClick={() => onRemove(runbook.id)}
              >
                <IconTrash className="size-4" aria-hidden="true" />
              </Button>
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}
