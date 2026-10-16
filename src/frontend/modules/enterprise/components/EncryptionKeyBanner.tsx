// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { useEncryptionKeySource } from '@resources/hooks/useSupport';

export const KEY_BANNER_DISMISSED = 'mcharbor-encryption-key-banner-dismissed';

function readDismissed(): boolean {
  try {
    return localStorage.getItem(KEY_BANNER_DISMISSED) === '1';
  } catch {
    return false;
  }
}

function writeDismissed() {
  try {
    localStorage.setItem(KEY_BANNER_DISMISSED, '1');
  } catch {
    return;
  }
}

export function EncryptionKeyBanner({ className }: { className?: string }) {
  const { t } = useTranslation('support');
  const canView = usePermission(PERM.SUPPORT_VIEW);
  const [dismissed, setDismissed] = useState(readDismissed);
  const source = useEncryptionKeySource(canView && !dismissed);
  if (!canView || dismissed || source.data?.source !== 'file') return null;

  const dismiss = () => {
    writeDismissed();
    setDismissed(true);
  };

  return (
    <Alert variant="warning" className={className} title={t('keyBanner.title')}>
      <p>{t('keyBanner.description')}</p>
      <div className="mt-2 flex flex-wrap gap-2">
        <Button variant="outline" size="sm" asChild>
          <Link to="/enterprise/diagnostics">{t('keyBanner.open')}</Link>
        </Button>
        <Button variant="ghost" size="sm" onClick={dismiss}>
          {t('keyBanner.dismiss')}
        </Button>
      </div>
    </Alert>
  );
}
