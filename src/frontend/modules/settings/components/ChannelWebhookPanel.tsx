// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { PERM } from '@core/auth/permissionKeys';
import { type ItsmChannel, type RotateResult, useRotateWebhookToken, useSetWebhookSecret } from '../hooks/useItsm';

export function ChannelWebhookPanel({ channel }: { channel: ItsmChannel }) {
  const { t } = useTranslation('settings');
  const rotate = useRotateWebhookToken();
  const setSecret = useSetWebhookSecret();
  const [confirming, setConfirming] = useState(false);
  const [shown, setShown] = useState<RotateResult | null>(null);
  const [vendorSecret, setVendorSecret] = useState('');

  return (
    <section className="space-y-3 rounded-md border border-border p-3">
      <h3 className="text-sm font-semibold text-foreground">{t('communications.itsm.title')}</h3>
      <p className="text-xs text-muted-foreground">
        {t(
          channel.webhookSupported
            ? 'communications.itsm.webhookHint'
            : channel.pollSupported
              ? 'communications.itsm.pollOnly'
              : 'communications.itsm.oneWay',
        )}
      </p>
      {channel.webhookSupported && (
        <>
          {shown ? (
            <div className="space-y-2">
              <Alert variant="warning">{t('communications.itsm.showOnce')}</Alert>
              <Label htmlFor="itsm-url">{t('communications.itsm.url')}</Label>
              <Input id="itsm-url" readOnly value={shown.url} onFocus={(e) => e.target.select()} />
              <Label htmlFor="itsm-secret">{t('communications.itsm.secret')}</Label>
              <Input id="itsm-secret" readOnly value={shown.secret} onFocus={(e) => e.target.select()} />
            </div>
          ) : (
            <p className="text-sm">
              {t(channel.tokenSet ? 'communications.itsm.tokenSet' : 'communications.itsm.tokenUnset')}
            </p>
          )}
          <Button variant="outline" permission={PERM.COMMUNICATIONS_MANAGE} onClick={() => setConfirming(true)}>
            {t(channel.tokenSet ? 'communications.itsm.rotate' : 'communications.itsm.create')}
          </Button>
          <div className="space-y-1">
            <Label htmlFor="itsm-vendor-secret">{t('communications.itsm.vendorSecret')}</Label>
            <div className="flex gap-2">
              <Input
                id="itsm-vendor-secret"
                type="password"
                autoComplete="off"
                value={vendorSecret}
                onChange={(e) => setVendorSecret(e.target.value)}
              />
              <Button
                variant="outline"
                permission={PERM.COMMUNICATIONS_MANAGE}
                disabled={!vendorSecret.trim() || setSecret.isPending}
                onClick={() =>
                  setSecret.mutate(
                    { channelId: channel.id, secret: vendorSecret.trim() },
                    { onSuccess: () => setVendorSecret('') },
                  )
                }
              >
                {t('communications.itsm.saveSecret')}
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">{t('communications.itsm.vendorSecretHint')}</p>
            {setSecret.error && <Alert variant="destructive">{setSecret.error.message}</Alert>}
          </div>
        </>
      )}
      <ConfirmDialog
        open={confirming}
        onOpenChange={(nextOpen) => {
          if (nextOpen) return;
          setConfirming(false);
          rotate.reset();
        }}
        title={t('communications.itsm.rotateTitle')}
        description={t('communications.itsm.rotateDescription')}
        confirmLabel={t('communications.itsm.rotateConfirm')}
        loading={rotate.isPending}
        permission={PERM.COMMUNICATIONS_MANAGE}
        onConfirm={() =>
          !rotate.isPending &&
          rotate.mutate(channel.id, {
            onSuccess: (res) => {
              setShown(res);
              setConfirming(false);
            },
          })
        }
      >
        {rotate.error && <Alert variant="destructive">{rotate.error.message}</Alert>}
      </ConfirmDialog>
    </section>
  );
}
