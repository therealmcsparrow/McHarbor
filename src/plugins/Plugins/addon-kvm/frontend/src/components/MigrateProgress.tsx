// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import { ProgressBar } from '@resources/components/ui/ProgressBar';
import { Spinner } from '@resources/components/ui/Spinner';
import { StatusError } from '@resources/components/ui/StatusError';
import type { MigrationStatus } from '../hooks/useMigration';

/**
 * A migration's progress, read from the server every poll.
 *
 * The step names come from the orchestrator and the agents as they report,
 * so they are shown as sent rather than mapped onto a list that would miss
 * the next one an agent adds.
 */
export function MigrateProgress({ status: migration, targetName }: { status: MigrationStatus; targetName: string }) {
  const { t } = useTranslation('vm');

  return (
    <div className="space-y-3">
      <p className="text-sm text-foreground">{t(`migrate.state.${migration.state}`, { target: targetName })}</p>
      {migration.state === 'running' && (
        <div className="space-y-2 rounded-md border border-border p-3">
          <div className="flex items-center gap-3">
            <Spinner className="h-4 w-4" />
            <p className="min-w-0 flex-1 text-sm text-foreground">
              {t('migrate.percent', { percent: migration.percent })}
            </p>
          </div>
          {migration.step && <p className="text-xs text-muted-foreground">{migration.step}</p>}
          <ProgressBar value={migration.percent} />
          <p className="text-xs text-muted-foreground">{t('migrate.runningHint')}</p>
        </div>
      )}
      {migration.warning && <Alert variant="warning" description={migration.warning} />}
      {migration.state === 'failed' && <StatusError error={migration.error} title={t('migrate.failed')} />}
    </div>
  );
}
