// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import { formatDate } from '@resources/utils/format';
import { IconBook2 } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { useResolvedRunbooks } from '@resources/hooks/useRunbooks';
import type { RunbookTarget } from '@resources/types/runbooks';

type RunbookBannerProps = {
  target: RunbookTarget;
};

function runbookPath(target: RunbookTarget, runbookId: string, runId?: string): string {
  const params = new URLSearchParams();
  if (target.environmentId) params.set('env', target.environmentId);
  if (target.container) params.set('container', target.container);
  if (target.alertEventId) params.set('alertEventId', target.alertEventId);
  if (runId) params.set('run', runId);
  const query = params.toString();
  return `/oncall/runbooks/${encodeURIComponent(runbookId)}${query ? `?${query}` : ''}`;
}

export function RunbookBanner({ target }: RunbookBannerProps) {
  const { t } = useTranslation('runbooks');
  const formatPrefs = useLocaleFormat();
  const { runbooks, openRuns } = useResolvedRunbooks(target);
  const runbook = runbooks[0];
  if (!runbook) return null;
  const isOpen = openRuns.find((run) => run.runbookId === runbook.id);

  return (
    <div className="mt-2 flex flex-col gap-1 rounded-md border border-border bg-muted/40 p-2">
      <p className="flex items-center gap-2 text-sm font-medium text-foreground">
        <IconBook2 className="size-4 shrink-0" aria-hidden="true" />
        {runbook.name}
      </p>
      {isOpen && (
        <p className="text-xs text-muted-foreground">
          {t('banner.inProgress', { who: isOpen.startedByName, at: formatDate(isOpen.startedAt, formatPrefs) })}
        </p>
      )}
      <Button asChild variant="outline" className="mt-1 min-h-11 w-full">
        <Link to={runbookPath(target, runbook.id, isOpen?.id)}>{isOpen ? t('banner.resume') : t('banner.start')}</Link>
      </Button>
    </div>
  );
}
