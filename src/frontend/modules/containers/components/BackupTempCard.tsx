// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconFolderCog } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';
import { Alert } from '@resources/components/ui/Alert';
import { Badge } from '@resources/components/ui/Badge';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { SettingsGroup } from '@resources/components/settings/SettingsGroup';
import { ApiError, translateErrorCode } from '@resources/utils/api-mutation';
import { formatBytes } from '@resources/utils/format';
import { useBackupTempStatus, type BackupTempStatus } from '../hooks/useBackupTemp';
import { BackupTempCleanup } from './BackupTempCleanup';
import { BackupTempLocationField } from './BackupTempLocationField';

function Readout({ temp }: { temp: BackupTempStatus }) {
  const { t } = useTranslation('containers');
  const stat = 'flex flex-col gap-0.5';
  const label = 'text-xs text-muted-foreground';
  return (
    <div className="space-y-2">
      {temp.envInvalidCode && (
        <Alert
          variant="warning"
          title={t('backupTemp.envInvalidTitle')}
          description={t('backupTemp.envInvalidBody', {
            reason: translateErrorCode(temp.envInvalidCode) ?? temp.envInvalidCode,
          })}
        />
      )}
      <dl className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
        <div className={`${stat} col-span-2 sm:col-span-4`}>
          <dt className={label}>{t('backupTemp.inUse')}</dt>
          <dd className="flex flex-wrap items-center gap-2">
            <code className="font-mono text-xs break-all">{temp.path}</code>
            <Badge variant="secondary">{t(`backupTemp.source.${temp.source}`)}</Badge>
          </dd>
        </div>
        <div className={stat}>
          <dt className={label}>{t('backupTemp.size')}</dt>
          <dd>{formatBytes(temp.sizeBytes)}</dd>
        </div>
        <div className={stat}>
          <dt className={label}>{t('backupTemp.entries')}</dt>
          <dd>{temp.entries}</dd>
        </div>
        <div className={stat}>
          <dt className={label}>{t('backupTemp.activeRuns')}</dt>
          <dd>{temp.activeRuns}</dd>
        </div>
      </dl>
      {temp.activeRuns > 0 && <p className="text-xs text-muted-foreground">{t('backupTemp.activeNote')}</p>}
      {temp.retiredPaths.length > 0 && (
        <p className="text-xs text-muted-foreground">{t('backupTemp.retired', { count: temp.retiredPaths.length })}</p>
      )}
    </div>
  );
}

/**
 * Backups → Settings: where backup and restore runs spool their temp data, with a manual clean-up.
 * Renders outside the tab's `settings.view` gate; `/container-backups/temp` needs a global `backups.manage`.
 */
export function BackupTempCard() {
  const { t } = useTranslation('containers');
  const { t: tSettings } = useTranslation('settings');
  const query = useBackupTempStatus();
  const scopedOnly = query.error instanceof ApiError && query.error.status === 403;
  const failed = query.isError && !scopedOnly;

  let body;
  if (scopedOnly) {
    body = (
      <Alert
        variant="warning"
        title={t('permission.denied', { ns: 'common' })}
        description={t('backupTemp.scopedOnly', { permission: PERM.BACKUPS_MANAGE })}
      />
    );
  } else if (failed) {
    body = <ErrorState variant="banner" error={query.error} onRetry={() => void query.refetch()} />;
  } else {
    body = (
      <div className="space-y-4" aria-busy={query.isLoading}>
        <BackupTempLocationField />
        {query.isLoading && <Skeleton className="h-16 w-full" />}
        {query.data && <Readout temp={query.data} />}
        <BackupTempCleanup temp={query.data} />
      </div>
    );
  }

  return (
    <SettingsGroup
      icon={IconFolderCog}
      title={t('backupTemp.title')}
      description={t('backupTemp.description')}
      scope={tSettings('general.scopeGlobal')}
    >
      {body}
    </SettingsGroup>
  );
}
