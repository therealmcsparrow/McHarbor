// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import { Spinner } from '@resources/components/ui/Spinner';
import { PageHeader } from '@resources/layout/PageHeader';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { SupportBundleForm } from '../components/SupportBundleForm';
import { SupportBundleHistory } from '../components/SupportBundleHistory';
import { useSupportBundles } from '@resources/hooks/useSupport';

export default function SupportBundlePage() {
  const { t } = useTranslation('support');
  const canView = usePermission(PERM.SUPPORT_VIEW);
  const bundles = useSupportBundles(canView);
  const busy = bundles.data?.items.some((b) => b.status === 'running') ?? false;

  return (
    <div className="space-y-6">
      <PageHeader title={t('bundle.title')} description={t('bundle.description')} headingLeaf />
      {!canView ? (
        <Alert variant="warning">{t('permission.requires', { ns: 'common', permission: PERM.SUPPORT_VIEW })}</Alert>
      ) : (
        <>
          <SupportBundleForm busy={busy} />
          {bundles.isLoading ? (
            <Spinner />
          ) : bundles.error ? (
            <Alert variant="destructive">{bundles.error.message}</Alert>
          ) : bundles.data ? (
            <SupportBundleHistory list={bundles.data} />
          ) : null}
        </>
      )}
    </div>
  );
}
