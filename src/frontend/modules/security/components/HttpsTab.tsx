// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useIsNative } from '@resources/hooks/useInstallKind';
import { IconUpload, IconInfoCircle, IconRefresh, IconShieldCheck, IconDownload } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Label } from '@resources/components/ui/Label';
import { Textarea } from '@resources/components/ui/Textarea';
import { Switch } from '@resources/components/ui/Switch';
import { Spinner } from '@resources/components/ui/Spinner';
import { useTLSStatus, useSaveTLS, useRegenerateTLS, useDownloadCA } from '@resources/hooks/useTLS';
import { PERM } from '@core/auth/permissionKeys';
import { HttpsCertificateDetails } from './HttpsCertificateDetails';
import { HttpsRegenerateDialog } from './HttpsRegenerateDialog';

const CERT_PLACEHOLDER = `-----BEGIN CERTIFICATE-----
...
-----END CERTIFICATE-----`;
const KEY_PLACEHOLDER = `-----BEGIN PRIVATE KEY-----
...
-----END PRIVATE KEY-----`;

export function HttpsTab() {
  const { t } = useTranslation('security');
  const native = useIsNative();
  const { data: tls, isLoading } = useTLSStatus();
  const saveTLS = useSaveTLS();
  const regenerateTLS = useRegenerateTLS();
  const downloadCA = useDownloadCA();
  const [certPEM, setCertPEM] = useState('');
  const [keyPEM, setKeyPEM] = useState('');
  const [regenerateOpen, setRegenerateOpen] = useState(false);

  if (isLoading) {
    return (
      <div className="flex h-32 items-center justify-center">
        <Spinner />
      </div>
    );
  }

  const handleUpload = () => {
    if (!certPEM.trim() || !keyPEM.trim()) return;
    saveTLS.mutate(
      { cert: certPEM.trim(), key: keyPEM.trim() },
      {
        onSuccess: () => {
          setCertPEM('');
          setKeyPEM('');
        },
      },
    );
  };

  const handleRegenerate = () => {
    setRegenerateOpen(false);
    regenerateTLS.mutate();
  };

  return (
    <div className="mc-glass space-y-6 rounded-lg p-6">
      <div>
        <h3 className="text-lg font-semibold text-foreground">{t('https.title')}</h3>
        <p className="text-sm text-muted-foreground">{t('https.description')}</p>
      </div>

      <HttpsCertificateDetails tls={tls} />

      {tls?.hasCa && (
        <div className="space-y-3 rounded-lg border border-primary/20 bg-primary/5 p-4">
          <div className="flex items-center gap-2">
            <IconShieldCheck className="size-5 text-primary" />
            <h4 className="font-medium text-foreground">{t('https.ca.title')}</h4>
          </div>
          <p className="text-sm text-muted-foreground">{t('https.ca.description')}</p>
          <Button
            permission={PERM.SETTINGS_VIEW}
            variant="outline"
            onClick={() => downloadCA.mutate()}
            disabled={downloadCA.isPending}
          >
            <IconDownload className="size-4" />
            {t('https.ca.download')}
          </Button>
          <p className="text-xs text-muted-foreground">{t('https.ca.installHint')}</p>
          <div className="border-t border-border pt-3">
            <p className="text-sm font-medium text-foreground">{t('https.pinning.title')}</p>
            <p className="text-sm text-muted-foreground">{t('https.pinning.description')}</p>
          </div>
        </div>
      )}

      <div className="space-y-3">
        <h4 className="font-medium text-foreground">{t('https.uploadTitle')}</h4>
        <div>
          <Label className="mb-2">{t('https.certLabel')}</Label>
          <Textarea
            value={certPEM}
            onChange={(eventItem) => setCertPEM(eventItem.target.value)}
            placeholder={CERT_PLACEHOLDER}
            className="font-mono text-xs"
            rows={6}
          />
        </div>
        <div>
          <Label className="mb-2">{t('https.keyLabel')}</Label>
          <Textarea
            value={keyPEM}
            onChange={(evt) => setKeyPEM(evt.target.value)}
            placeholder={KEY_PLACEHOLDER}
            className="font-mono text-xs"
            rows={6}
          />
        </div>
        <Button
          permission={PERM.SETTINGS_MANAGE}
          onClick={handleUpload}
          disabled={saveTLS.isPending || !certPEM.trim() || !keyPEM.trim()}
        >
          <IconUpload className="size-4" />
          {saveTLS.isPending ? t('https.uploading') : t('https.uploadButton')}
        </Button>
        {saveTLS.isError && <p className="text-sm text-destructive">{t('https.uploadError')}</p>}
        {tls?.certSource === 'self-signed' && (
          <Button
            permission={PERM.SETTINGS_MANAGE}
            variant="outline"
            onClick={() => setRegenerateOpen(true)}
            disabled={saveTLS.isPending}
          >
            <IconRefresh className="size-4" />
            {t('https.regenerate.button')}
          </Button>
        )}
      </div>

      <div className="space-y-4">
        <div className="flex items-center justify-between rounded-lg border border-border p-4">
          <div>
            <p className="font-medium text-foreground">{t('https.enableHttps')}</p>
            <p className="text-sm text-muted-foreground">{t('https.enableHttpsDescription')}</p>
          </div>
          <Switch
            checked={tls?.enabled ?? false}
            onCheckedChange={(enabled) => saveTLS.mutate({ enabled })}
            disabled={saveTLS.isPending || !tls?.hasCert}
          />
        </div>
        <div className="flex items-center justify-between rounded-lg border border-border p-4">
          <div>
            <p className="font-medium text-foreground">{t('https.forceHttps')}</p>
            <p className="text-sm text-muted-foreground">{t('https.forceHttpsDescription')}</p>
          </div>
          <Switch
            checked={tls?.forceHttps ?? false}
            onCheckedChange={(forceHttps) => saveTLS.mutate({ forceHttps })}
            disabled={saveTLS.isPending || !tls?.enabled}
          />
        </div>
      </div>

      <div className="flex items-start gap-3 rounded-lg border border-primary/20 bg-primary/5 p-4">
        <IconInfoCircle className="mt-0.5 size-5 shrink-0 text-primary" />
        <div className="text-sm text-muted-foreground">
          <p className="font-medium text-foreground">{t('https.restartRequired')}</p>
          <p>{t(native ? 'https.restartDescriptionNative' : 'https.restartDescription')}</p>
        </div>
      </div>

      <HttpsRegenerateDialog open={regenerateOpen} onOpenChange={setRegenerateOpen} onConfirm={handleRegenerate} />
    </div>
  );
}
