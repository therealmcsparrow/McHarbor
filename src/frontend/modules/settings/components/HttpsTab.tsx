// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconCertificate,
  IconUpload,
  IconInfoCircle,
} from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Label } from '@resources/components/ui/Label';
import { Textarea } from '@resources/components/ui/Textarea';
import { Switch } from '@resources/components/ui/Switch';
import { Spinner } from '@resources/components/ui/Spinner';
import { formatDateOnly } from '@resources/utils/format';
import { useTLSStatus, useSaveTLS, getCertExpiryStatus } from '../hooks/useTLS';

export function HttpsTab() {
  const { t } = useTranslation('settings');
  const { data: tls, isLoading } = useTLSStatus();
  const saveTLS = useSaveTLS();
  const [certPEM, setCertPEM] = useState('');
  const [keyPEM, setKeyPEM] = useState('');

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
      }
    );
  };

  const handleToggleEnabled = (checked: boolean) => {
    saveTLS.mutate({ enabled: checked });
  };

  const handleToggleForce = (checked: boolean) => {
    saveTLS.mutate({ forceHttps: checked });
  };

  const expiryStatus = tls?.certInfo?.notAfter
    ? getCertExpiryStatus(tls.certInfo.notAfter)
    : null;

  return (
    <div className="space-y-6">
      {/* Certificate Info */}
      {tls?.hasCert && tls.certInfo ? (
        <div className="space-y-3 rounded-lg border border-border p-4">
          <div className="flex items-center gap-2">
            <IconCertificate className="size-5 text-primary" />
            <h4 className="font-medium text-foreground">{t('https.certificate')}</h4>
            {expiryStatus && (
              <Badge variant={expiryStatus.variant}>{expiryStatus.label}</Badge>
            )}
          </div>
          <div className="grid grid-cols-2 gap-x-6 gap-y-2 text-sm">
            <div>
              <span className="text-muted-foreground">{t('https.subject')}</span>{' '}
              <span className="text-foreground">{tls.certInfo.subject || '\u2014'}</span>
            </div>
            <div>
              <span className="text-muted-foreground">{t('https.issuer')}</span>{' '}
              <span className="text-foreground">{tls.certInfo.issuer || '\u2014'}</span>
            </div>
            <div>
              <span className="text-muted-foreground">{t('https.validFrom')}</span>{' '}
              <span className="text-foreground">
                {formatDateOnly(tls.certInfo.notBefore)}
              </span>
            </div>
            <div>
              <span className="text-muted-foreground">{t('https.validUntil')}</span>{' '}
              <span className="text-foreground">
                {formatDateOnly(tls.certInfo.notAfter)}
              </span>
            </div>
            <div>
              <span className="text-muted-foreground">{t('https.serial')}</span>{' '}
              <span className="font-mono text-xs text-foreground">
                {tls.certInfo.serialNumber}
              </span>
            </div>
            {tls.certInfo.dnsNames && tls.certInfo.dnsNames.length > 0 && (
              <div>
                <span className="text-muted-foreground">{t('https.dnsNames')}</span>{' '}
                <span className="text-foreground">{tls.certInfo.dnsNames.join(', ')}</span>
              </div>
            )}
          </div>
        </div>
      ) : (
        <div className="rounded-lg border border-dashed border-border p-4 text-center">
          <IconCertificate className="mx-auto size-8 text-muted-foreground/50" />
          <p className="mt-2 text-sm text-muted-foreground">{t('https.noCertificate')}</p>
        </div>
      )}

      {/* Upload Section */}
      <div className="space-y-3">
        <h4 className="font-medium text-foreground">{t('https.uploadTitle')}</h4>
        <div>
          <Label className="mb-2">{t('https.certLabel')}</Label>
          <Textarea
            value={certPEM}
            onChange={(e) => setCertPEM(e.target.value)}
            placeholder="-----BEGIN CERTIFICATE-----&#10;...&#10;-----END CERTIFICATE-----"
            className="font-mono text-xs"
            rows={6}
          />
        </div>
        <div>
          <Label className="mb-2">{t('https.keyLabel')}</Label>
          <Textarea
            value={keyPEM}
            onChange={(e) => setKeyPEM(e.target.value)}
            placeholder="-----BEGIN PRIVATE KEY-----&#10;...&#10;-----END PRIVATE KEY-----"
            className="font-mono text-xs"
            rows={6}
          />
        </div>
        <Button
          onClick={handleUpload}
          disabled={saveTLS.isPending || !certPEM.trim() || !keyPEM.trim()}
        >
          <IconUpload className="size-4" />
          {saveTLS.isPending ? t('https.uploading') : t('https.uploadButton')}
        </Button>
        {saveTLS.isError && (
          <p className="text-sm text-destructive">
            {(saveTLS.error as Error)?.message || t('https.uploadError')}
          </p>
        )}
      </div>

      {/* Toggle Switches */}
      <div className="space-y-4">
        <div className="flex items-center justify-between rounded-lg border border-border p-4">
          <div>
            <p className="font-medium text-foreground">{t('https.enableHttps')}</p>
            <p className="text-sm text-muted-foreground">
              {t('https.enableHttpsDescription')}
            </p>
          </div>
          <Switch
            checked={tls?.enabled ?? false}
            onCheckedChange={handleToggleEnabled}
            disabled={saveTLS.isPending || !tls?.hasCert}
          />
        </div>
        <div className="flex items-center justify-between rounded-lg border border-border p-4">
          <div>
            <p className="font-medium text-foreground">{t('https.forceHttps')}</p>
            <p className="text-sm text-muted-foreground">
              {t('https.forceHttpsDescription')}
            </p>
          </div>
          <Switch
            checked={tls?.forceHttps ?? false}
            onCheckedChange={handleToggleForce}
            disabled={saveTLS.isPending || !tls?.enabled}
          />
        </div>
      </div>

      {/* Restart Notice */}
      <div className="flex items-start gap-3 rounded-lg border border-blue-500/20 bg-blue-500/5 p-4">
        <IconInfoCircle className="mt-0.5 size-5 shrink-0 text-blue-400" />
        <div className="text-sm text-muted-foreground">
          <p className="font-medium text-foreground">{t('https.restartRequired')}</p>
          <p>{t('https.restartDescription')}</p>
        </div>
      </div>
    </div>
  );
}
