// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconArrowRight, IconCalendarClock, IconCircleCheck, IconKey } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { StatusError } from '@resources/components/ui/StatusError';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import { timeAgo } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { useBackupEncryptionKeyStatus } from '@resources/hooks/useStorageLocations';
import { formatTimestamp } from '../pages/backups-page-helpers';
import type { ContainerBackupPlan, ContainerBackupRun } from '@resources/hooks/useContainerBackups';
import type { BackupsTabId } from './backups-tab-groups';

type PanelProps = {
  title: string;
  icon: ReactNode;
  action?: ReactNode;
  children: ReactNode;
};

/** One overview panel: a titled `mc-glass` block with an optional jump-off. */
function OverviewPanel({ title, icon, action, children }: PanelProps) {
  return (
    <section className="mc-glass flex flex-col rounded-lg p-4">
      <header className="mb-3 flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <span className="text-muted-foreground">{icon}</span>
          <h3 className="truncate text-sm font-semibold text-foreground">{title}</h3>
        </div>
        {action}
      </header>
      <div className="min-h-0 flex-1">{children}</div>
    </section>
  );
}

function EmptyLine({ children }: { children: ReactNode }) {
  return <p className="py-6 text-center text-xs text-muted-foreground">{children}</p>;
}

type NavigateProps = { onNavigate: (tab: BackupsTabId) => void };

/** The next few scheduled plan runs — the answer to "am I covered tonight?". */
export function UpcomingRunsPanel({ plans, onNavigate }: NavigateProps & { plans: ContainerBackupPlan[] }) {
  const { t } = useTranslation('containers');
  const formatPrefs = useLocaleFormat();

  return (
    <OverviewPanel
      title={t('backups.overview.upcomingTitle')}
      icon={<IconCalendarClock className="size-4" />}
      action={
        <Button variant="ghost" size="sm" onClick={() => onNavigate('plans')}>
          {t('backups.overview.viewPlans')}
          <IconArrowRight className="size-4" />
        </Button>
      }
    >
      {plans.length === 0 ? (
        <EmptyLine>{t('backups.overview.upcomingEmpty')}</EmptyLine>
      ) : (
        <ul className="divide-y divide-border">
          {plans.map((plan) => (
            <li key={plan.id} className="flex items-center justify-between gap-3 py-2">
              <div className="min-w-0">
                <p className="truncate text-sm text-foreground">{plan.name}</p>
                <p className="truncate text-xs text-muted-foreground">
                  {plan.containerName || plan.containerId.slice(0, 12)}
                </p>
              </div>
              <span className="shrink-0 text-xs whitespace-nowrap text-muted-foreground">
                {formatTimestamp(plan.nextRunAt, formatPrefs)}
              </span>
            </li>
          ))}
        </ul>
      )}
    </OverviewPanel>
  );
}

/** Failed runs inside the window — the only list on this tab that is a to-do. */
export function RecentFailuresPanel({ runs, onNavigate }: NavigateProps & { runs: ContainerBackupRun[] }) {
  const { t } = useTranslation('containers');

  return (
    <OverviewPanel
      title={t('backups.overview.failuresTitle')}
      icon={<IconAlertTriangle className="size-4" />}
      action={
        <Button variant="ghost" size="sm" onClick={() => onNavigate('runs')}>
          {t('backups.overview.viewRuns')}
          <IconArrowRight className="size-4" />
        </Button>
      }
    >
      {runs.length === 0 ? (
        <EmptyLine>{t('backups.overview.failuresEmpty')}</EmptyLine>
      ) : (
        <ul className="divide-y divide-border">
          {runs.map((run) => (
            <li key={run.id} className="flex items-start justify-between gap-3 py-2">
              <div className="min-w-0">
                <div className="flex min-w-0 items-center gap-2">
                  <Badge variant="destructive" className="px-1.5 py-0 text-[10px]">
                    {t(`backups.operation.${run.operation}`)}
                  </Badge>
                  <p className="truncate text-sm text-foreground">
                    {run.containerName || run.containerId.slice(0, 12)}
                  </p>
                  <StatusError error={run.error} />
                </div>
              </div>
              <span className="shrink-0 text-xs whitespace-nowrap text-muted-foreground">{timeAgo(run.startedAt)}</span>
            </li>
          ))}
        </ul>
      )}
    </OverviewPanel>
  );
}

/**
 * Whether archives are being encrypted, and with which key.
 *
 * Rendered only for readers who hold `settings.view` — the status endpoint
 * is gated on it, so for anyone else this panel could only ever show a
 * failed request.
 */
export function EncryptionStatusPanel({ onNavigate }: NavigateProps) {
  const canView = usePermission(PERM.SETTINGS_VIEW);
  // The status query lives in the child so it is never issued for a reader
  // who would only get a 403 back.
  if (!canView) return null;
  return <EncryptionStatus onNavigate={onNavigate} />;
}

function EncryptionStatus({ onNavigate }: NavigateProps) {
  const { t } = useTranslation('containers');
  const { data: statusValue } = useBackupEncryptionKeyStatus();
  const active = statusValue?.readable ?? false;

  return (
    <OverviewPanel
      title={t('backups.overview.encryptionTitle')}
      icon={active ? <IconCircleCheck className="size-4" /> : <IconKey className="size-4" />}
      action={
        <Button variant="ghost" size="sm" onClick={() => onNavigate('settings')}>
          {t('backups.overview.manageKey')}
          <IconArrowRight className="size-4" />
        </Button>
      }
    >
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant={active ? 'success' : 'secondary'} className="py-1">
          {active && <IconCircleCheck className="size-3.5" />}
          {active ? t('backups.overview.encryptionActive') : t('backups.overview.encryptionInactive')}
        </Badge>
        <p className="text-xs text-muted-foreground">
          {active ? t('backups.overview.encryptionActiveHint') : t('backups.overview.encryptionInactiveHint')}
        </p>
      </div>
      {active && statusValue?.keyId && (
        <p className="mt-2 truncate font-mono text-xs text-muted-foreground">{statusValue.keyId}</p>
      )}
    </OverviewPanel>
  );
}
