// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useSearchParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Spinner } from '@resources/components/ui/Spinner';
import { PageHeader } from '@resources/layout/PageHeader';
import { DeviceApprovalForm } from '../components/DeviceApprovalForm';
import { isCompleteUserCode, normalizeUserCode, useDeviceRequest } from '../hooks/useDeviceSignIn';

export default function DeviceApprovePage() {
  const { t } = useTranslation('security');
  const [params] = useSearchParams();
  const [code, setCode] = useState(() => normalizeUserCode(params.get('code') ?? ''));
  const [decision, setDecision] = useState<boolean | null>(null);
  const request = useDeviceRequest(code, decision === null);

  return (
    <section aria-label={t('device.title')} className="mx-auto w-full max-w-2xl space-y-4 py-6">
      <PageHeader title={t('device.title')} description={t('device.description')} headingLeaf />

      {decision !== null ? (
        <output className="block rounded-lg border border-border bg-card p-4 text-sm">
          {decision ? t('device.approved') : t('device.denied')}
        </output>
      ) : (
        <>
          <div className="space-y-2">
            <Label htmlFor="device-code">{t('device.code')}</Label>
            <Input
              id="device-code"
              value={code}
              autoComplete="off"
              placeholder="BCDF-GHJK"
              className="max-w-xs font-mono uppercase"
              onChange={(e) => setCode(normalizeUserCode(e.target.value))}
            />
          </div>
          {isCompleteUserCode(code) &&
            (request.isLoading ? (
              <Spinner size="md" />
            ) : request.error ? (
              <p className="text-sm text-destructive">{t('device.notFound')}</p>
            ) : request.data ? (
              <DeviceApprovalForm request={request.data} onDecided={setDecision} />
            ) : null)}
        </>
      )}
    </section>
  );
}
