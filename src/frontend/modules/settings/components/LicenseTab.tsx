// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Settings -> System -> License, sitting immediately before About.
//
// It answers three questions in the order people ask them: what am I
// licensed for, how much of it am I using, and how do I change it. The
// third is a key the operator pastes — mcharbor.eu issues the key, and
// this install only claims a seat against it.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconExternalLink, IconRefresh, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { Grid, Stack } from '@resources/components/ui/Layout';
import { SectionHeader } from '@resources/components/ui/SectionHeader';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { appToast } from '@resources/utils/toast';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { PERM } from '@core/auth/permissionKeys';
import { IconCertificate } from '@tabler/icons-react';
import { useLicenseDetail, useLicenseStatus, useRefreshLicense, useReleaseLicense } from '../license/hooks/useLicense';
import { InstanceCard } from '../license/components/InstanceCard';
import { LicenseStatusCard } from '../license/components/LicenseStatusCard';
import { LicenseUsageTable } from '../license/components/LicenseUsageTable';
import { ActivateLicenseDialog } from '../license/components/ActivateLicenseDialog';

// Keys are issued at mcharbor.eu, not in here — the request page opens in a
// new tab so an operator mid-activation does not lose this one.
const licenseRequestUrl = 'https://www.mcharbor.eu/licensing/request';

export function LicenseTab() {
  const { t } = useTranslation('license');
  const { denied } = usePermissionGate(PERM.LICENSE_VIEW);

  const { data: detail, isLoading } = useLicenseDetail(!denied);
  const { data: licenseStatus } = useLicenseStatus();
  const refresh = useRefreshLicense();
  const release = useReleaseLicense();

  const [activateOpen, setActivateOpen] = useState(false);
  const [releaseOpen, setReleaseOpen] = useState(false);

  // The tab is reachable from the settings rail whatever the role, so it
  // says why it is empty rather than rendering a blank panel.
  if (denied) {
    return (
      <EmptyState icon={<IconCertificate className="size-6" />} title={t('title')} description={t('noPermission')} />
    );
  }

  if (isLoading || !detail || !licenseStatus) {
    return <Skeleton className="h-64 w-full" />;
  }

  const license = detail.license;

  return (
    <Stack gap={6}>
      <SectionHeader
        title={t('title')}
        description={t('description')}
        action={
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              permission={PERM.LICENSE_MANAGE}
              disabled={!license || refresh.isPending}
              onClick={() =>
                refresh.mutate(undefined, {
                  onSuccess: () => appToast.success(t('actions.refreshed')),
                })
              }
            >
              <IconRefresh className="size-4" /> {t('actions.refresh')}
            </Button>
            <Button
              variant="outline"
              size="sm"
              permission={PERM.LICENSE_MANAGE}
              disabled={!license}
              onClick={() => setReleaseOpen(true)}
            >
              <IconTrash className="size-4" /> {t('actions.release')}
            </Button>
            <Button variant="outline" size="sm" asChild>
              <a href={licenseRequestUrl} target="_blank" rel="noreferrer">
                <IconExternalLink className="size-4" /> {t('activate.request')}
              </a>
            </Button>
            <Button permission={PERM.LICENSE_MANAGE} onClick={() => setActivateOpen(true)}>
              {license ? t('activate.change') : t('activate.open')}
            </Button>
          </div>
        }
      />

      <Grid columns="grid-cols-1 lg:grid-cols-2" gap={4}>
        <LicenseStatusCard status={licenseStatus} license={license} serverConfigured={detail.serverConfigured} />
        <InstanceCard instanceId={detail.instanceId} instanceType={detail.instanceType} hostname={detail.hostname} />
      </Grid>

      <LicenseUsageTable />

      <ActivateLicenseDialog open={activateOpen} onOpenChange={setActivateOpen} hasLicense={Boolean(license)} />

      <ConfirmDialog
        open={releaseOpen}
        onOpenChange={setReleaseOpen}
        title={t('actions.releaseConfirmTitle')}
        description={t('actions.releaseConfirmBody')}
        confirmLabel={t('actions.release')}
        variant="destructive"
        loading={release.isPending}
        onConfirm={() =>
          release.mutate(undefined, {
            onSuccess: () => {
              appToast.success(t('actions.released'));
              setReleaseOpen(false);
            },
          })
        }
      />
    </Stack>
  );
}
