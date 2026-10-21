// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// McHarborUpdateCard — manual version check against the release feed plus the
// one-click self-update. Lives on the Updates tab next to the periodic-check and
// auto-install cards; the About tab only shows build information.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { useIsNative } from '@resources/hooks/useInstallKind';
import { IconRefresh, IconArrowUp, IconCheck, IconDownload, IconListCheck } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import {
  useCurrentUserPermissions,
  hasPermission,
  SYSTEM_RESTART_PERMISSION,
} from '@core/auth/useCurrentUserPermissions';
import { PERM } from '@core/auth/permissionKeys';
import { PreUpgradeCheckDialog } from '@resources/components/support/PreUpgradeCheckDialog';
import { useCheckUpdate, useApplySelfUpdate } from '../hooks/useUpdates';
import { releaseLabel } from '../hooks/releaseLabel';

export function McHarborUpdateCard() {
  const { t } = useTranslation('settings');
  const native = useIsNative();
  const userPermissions = useCurrentUserPermissions();
  const canSelfUpdate = hasPermission(userPermissions.data, SYSTEM_RESTART_PERMISSION);

  const {
    data: versionCheck,
    refetch: checkUpdate,
    isError: checkFailed,
    error: checkError,
    isFetching: isChecking,
  } = useCheckUpdate();
  const applySelfUpdate = useApplySelfUpdate();
  const [selfUpdateOpen, setSelfUpdateOpen] = useState(false);
  const [preUpgradeOpen, setPreUpgradeOpen] = useState(false);

  return (
    <div className="rounded-lg border border-border p-5">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-semibold text-foreground">{t('about.checkForUpdates')}</h3>
          <p className="text-sm text-muted-foreground">{t('about.checkForUpdatesDescription')}</p>
        </div>
        <Button variant="outline" onClick={() => checkUpdate()} disabled={isChecking}>
          {isChecking ? <Spinner size="sm" /> : <IconRefresh className="size-4" />}
          {t('about.checkNow')}
        </Button>
      </div>

      {checkFailed && (
        <ErrorState variant="banner" className="mt-4" title={t('about.checkFailed')} error={checkError} />
      )}

      {!checkFailed && versionCheck && (
        <div className="mt-4 rounded-lg bg-muted p-4">
          {versionCheck.updateAvailable ? (
            <div className="space-y-3">
              <div className="flex items-center gap-2">
                <IconArrowUp className="size-4 text-success" />
                <span className="text-sm font-medium text-foreground">
                  {t('about.updateAvailable', {
                    version: releaseLabel(versionCheck.latestVersion, versionCheck.latestBuild),
                  })}
                </span>
                <Badge variant="default">{t('about.newVersion')}</Badge>
              </div>
              <p className="text-xs text-muted-foreground">
                {t('about.currentVersionLabel')}:{' '}
                {t('about.versionTransition', {
                  current: releaseLabel(versionCheck.currentVersion, versionCheck.currentBuild),
                  latest: releaseLabel(versionCheck.latestVersion, versionCheck.latestBuild),
                })}
              </p>
              {versionCheck.releaseNotes && (
                <p className="max-h-32 overflow-y-auto text-xs whitespace-pre-wrap text-muted-foreground">
                  {versionCheck.releaseNotes}
                </p>
              )}
              {versionCheck.releaseUrl && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => window.open(versionCheck.releaseUrl, '_blank', 'noopener')}
                >
                  {t('about.viewRelease')}
                </Button>
              )}
              <Button
                variant="outline"
                size="sm"
                onClick={() => setPreUpgradeOpen(true)}
                permission={PERM.UPDATES_VIEW}
              >
                <IconListCheck className="size-4" />
                {t('preUpgrade.open', { ns: 'support' })}
              </Button>
              {canSelfUpdate && !native && (
                <Button
                  variant="default"
                  size="sm"
                  onClick={() => setSelfUpdateOpen(true)}
                  disabled={applySelfUpdate.isPending}
                  data-testid="self-update-button"
                >
                  {applySelfUpdate.isPending ? <Spinner size="sm" /> : <IconDownload className="size-4" />}
                  {t('about.updateNow')}
                </Button>
              )}
              {native && <p className="w-full text-xs text-muted-foreground">{t('about.selfUpdateNative')}</p>}
            </div>
          ) : (
            <div className="flex items-center gap-2">
              <IconCheck className="size-4 text-success" />
              <span className="text-sm text-foreground">{t('about.upToDate')}</span>
              <span className="text-xs text-muted-foreground">
                {t('about.versionShort', {
                  version: releaseLabel(versionCheck.currentVersion, versionCheck.currentBuild),
                })}
              </span>
            </div>
          )}
        </div>
      )}

      {versionCheck?.updateAvailable && (
        <PreUpgradeCheckDialog
          open={preUpgradeOpen}
          onOpenChange={setPreUpgradeOpen}
          target={versionCheck.latestVersion}
        />
      )}

      <ConfirmDialog
        open={selfUpdateOpen}
        onOpenChange={(isOpen) => {
          setSelfUpdateOpen(isOpen);
          if (!isOpen && applySelfUpdate.isPending) return;
        }}
        title={t('about.selfUpdateConfirmTitle')}
        description={t('about.selfUpdateConfirmDescription')}
        confirmLabel={t('about.selfUpdateConfirm')}
        variant="destructive"
        loading={applySelfUpdate.isPending}
        onConfirm={() => {
          applySelfUpdate.mutate({});
        }}
      />
    </div>
  );
}
