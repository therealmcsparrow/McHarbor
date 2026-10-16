// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconBellOff, IconBellRinging, IconSend } from '@tabler/icons-react';
import { ServiceWorkerUnavailableError } from '@core/pwa/register';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { StatusError } from '@resources/components/ui/StatusError';
import {
  detectPushSupport,
  useDisablePush,
  useEnablePush,
  useLocalPushSubscription,
  useSendTestPush,
} from '../hooks/usePushDevice';

const SUPPORT_MESSAGE = {
  unsupported: 'device.unsupported',
  insecure: 'device.insecure',
  'ios-install': 'device.iosInstall',
} as const;

export function PushCard() {
  const { t } = useTranslation('oncall');
  const [label, setLabel] = useState('');
  const support = detectPushSupport();
  const local = useLocalPushSubscription();
  const enable = useEnablePush();
  const disable = useDisablePush();
  const test = useSendTestPush();
  const subscribed = Boolean(local.data);
  const blocked = support === 'supported' && Notification.permission === 'denied';
  const failure = enable.error ?? disable.error ?? test.error;
  const failureSummary =
    failure instanceof ServiceWorkerUnavailableError
      ? t('device.workerUnavailable')
      : failure?.message === 'permission-denied'
        ? t('device.denied')
        : t('state.loadFailed');
  const failureDetail =
    failure instanceof ServiceWorkerUnavailableError
      ? failure.detail
      : failure?.message === 'permission-denied'
        ? null
        : failure?.message;

  return (
    <div className="flex flex-col gap-3 rounded-lg border border-border bg-card p-4">
      <h2 className="font-medium">{t('device.pushTitle')}</h2>
      <p className="text-sm text-muted-foreground">{t('device.pushIntro')}</p>
      <p className="rounded-md bg-muted/40 p-2 text-xs text-muted-foreground">{t('device.transit')}</p>
      {support !== 'supported' ? (
        <p className="text-sm">{t(SUPPORT_MESSAGE[support])}</p>
      ) : blocked ? (
        <p className="text-sm">{t('device.denied')}</p>
      ) : subscribed ? (
        <>
          <p className="text-sm font-medium text-success">{t('device.enabled')}</p>
          <Button variant="outline" className="min-h-12" disabled={test.isPending} onClick={() => test.mutate()}>
            <IconSend className="size-5" aria-hidden="true" />
            {t('device.test')}
          </Button>
          {test.data && (
            <output className="block text-sm">{t('device.testResult', { ...test.data, count: test.data.sent })}</output>
          )}
          <Button variant="ghost" className="min-h-12" disabled={disable.isPending} onClick={() => disable.mutate()}>
            <IconBellOff className="size-5" aria-hidden="true" />
            {t('device.disable')}
          </Button>
        </>
      ) : (
        <>
          <div className="flex flex-col gap-1">
            <Label htmlFor="push-device-label">{t('device.label')}</Label>
            <Input
              id="push-device-label"
              value={label}
              maxLength={80}
              placeholder={t('device.labelPlaceholder')}
              onChange={(evt) => setLabel(evt.target.value)}
              className="min-h-11"
            />
          </div>
          <Button className="min-h-12" disabled={enable.isPending} onClick={() => enable.mutate(label.trim())}>
            <IconBellRinging className="size-5" aria-hidden="true" />
            {t('device.enable')}
          </Button>
        </>
      )}
      {failure && (
        <div className="flex items-start gap-2 text-sm text-destructive">
          <span>{failureSummary}</span>
          <StatusError error={failureDetail} />
        </div>
      )}
    </div>
  );
}
