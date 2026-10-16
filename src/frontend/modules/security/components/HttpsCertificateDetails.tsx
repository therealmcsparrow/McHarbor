// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCertificate, IconAlertTriangle, IconCopy } from '@tabler/icons-react';
import { toast } from 'sonner';
import { Button } from '@resources/components/ui/Button';
import { copyToClipboard } from '@resources/utils/clipboard';
import { Badge } from '@resources/components/ui/Badge';
import { formatDateOnly } from '@resources/utils/format';
import { getCertExpiryStatus } from '@resources/hooks/useTLS';
import type { TLSStatus } from '@core/types/tls';

const tlsVersionLabel = (minor: number) => `TLS 1.${minor - 11}`;

type HttpsCertificateDetailsProps = {
  tls: TLSStatus | undefined;
};

/**
 * What the server is currently presenting: issuer, validity window, names,
 * and the negotiated-protocol settings an operator has to quote in an audit.
 * Renders the "no certificate" placeholder when none is installed.
 */
export function HttpsCertificateDetails({ tls }: HttpsCertificateDetailsProps) {
  const { t } = useTranslation('security');

  if (!tls?.hasCert || !tls.certInfo) {
    return (
      <div className="rounded-lg border border-dashed border-border p-4 text-center">
        <IconCertificate className="mx-auto size-8 text-muted-foreground/50" />
        <p className="mt-2 text-sm text-muted-foreground">{t('https.noCertificate')}</p>
      </div>
    );
  }

  const expiryStatus = tls.certInfo.notAfter ? getCertExpiryStatus(tls.certInfo.notAfter) : null;

  const fingerprint = tls.certInfo.fingerprint;

  const handleCopyFingerprint = (value: string) => {
    void copyToClipboard(value).then((result) => {
      if (result.ok) {
        toast.success(t('https.fingerprintCopied'));
      }
    });
  };

  return (
    <div className="space-y-3 rounded-lg border border-border p-4">
      <div className="flex items-center gap-2">
        <IconCertificate className="size-5 text-primary" />
        <h4 className="font-medium text-foreground">{t('https.certificate')}</h4>
        {expiryStatus && <Badge variant={expiryStatus.variant}>{expiryStatus.label}</Badge>}
        {tls.certSource === 'self-signed' && <Badge variant="warning">{t('https.certSource.selfSigned')}</Badge>}
        {tls.certSource === 'user-uploaded' && <Badge variant="success">{t('https.certSource.userUploaded')}</Badge>}
      </div>
      {tls.certSource === 'self-signed' && (
        <div className="flex items-start gap-3 rounded-lg border border-warning/30 bg-warning/5 p-3">
          <IconAlertTriangle className="mt-0.5 size-4 shrink-0 text-warning" />
          <p className="text-sm text-muted-foreground">{t('https.warning.selfSigned')}</p>
        </div>
      )}
      {fingerprint && (
        <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-3">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <p className="text-sm text-muted-foreground">{t('https.fingerprint')}</p>
              <p className="font-mono text-xs break-all text-foreground">{fingerprint}</p>
            </div>
            <Button
              variant="ghost"
              size="icon"
              aria-label={t('https.fingerprintCopy')}
              onClick={() => handleCopyFingerprint(fingerprint)}
            >
              <IconCopy className="size-4" />
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">{t('https.fingerprintHint')}</p>
        </div>
      )}
      <div className="grid grid-cols-1 gap-x-6 gap-y-2 text-sm md:grid-cols-2">
        <div>
          <span className="text-muted-foreground">{t('https.subject')}</span>{' '}
          <span className="text-foreground">{tls.certInfo.subject || '-'}</span>
        </div>
        <div>
          <span className="text-muted-foreground">{t('https.issuer')}</span>{' '}
          <span className="text-foreground">{tls.certInfo.issuer || '-'}</span>
        </div>
        <div>
          <span className="text-muted-foreground">{t('https.validFrom')}</span>{' '}
          <span className="text-foreground">{formatDateOnly(tls.certInfo.notBefore)}</span>
        </div>
        <div>
          <span className="text-muted-foreground">{t('https.validUntil')}</span>{' '}
          <span className="text-foreground">{formatDateOnly(tls.certInfo.notAfter)}</span>
        </div>
        <div>
          <span className="text-muted-foreground">{t('https.serial')}</span>{' '}
          <span className="font-mono text-xs text-foreground">{tls.certInfo.serialNumber}</span>
        </div>
        {tls.caFingerprint && (
          <div className="md:col-span-2">
            <span className="text-muted-foreground">{t('https.ca.fingerprint')}</span>{' '}
            <span className="font-mono text-xs break-all text-foreground">{tls.caFingerprint}</span>
          </div>
        )}
        {tls.caNotAfter && (
          <div>
            <span className="text-muted-foreground">{t('https.ca.expires')}</span>{' '}
            <span className="text-foreground">{formatDateOnly(tls.caNotAfter)}</span>
          </div>
        )}
        {tls.certInfo.dnsNames && tls.certInfo.dnsNames.length > 0 && (
          <div>
            <span className="text-muted-foreground">{t('https.dnsNames')}</span>{' '}
            <span className="text-foreground">{tls.certInfo.dnsNames.join(', ')}</span>
          </div>
        )}
        {typeof tls.tlsMinVersion === 'number' && (
          <div>
            <span className="text-muted-foreground">{t('https.settings.tlsMinVersion')}</span>{' '}
            <span className="font-mono text-xs text-foreground">{tlsVersionLabel(tls.tlsMinVersion ?? 0)}</span>
          </div>
        )}
        {typeof tls.hstsMaxAge === 'number' && (
          <div>
            <span className="text-muted-foreground">{t('https.settings.hstsMaxAge')}</span>{' '}
            <span className="font-mono text-xs text-foreground">
              {tls.hstsMaxAge === 0 ? t('https.settings.hstsOff') : `${tls.hstsMaxAge}s`}
            </span>
          </div>
        )}
        {typeof tls.transferPort === 'number' && (
          <div>
            <span className="text-muted-foreground">{t('https.settings.transferPort')}</span>{' '}
            <span className="font-mono text-xs text-foreground">{tls.transferPort}</span>
          </div>
        )}
      </div>
    </div>
  );
}
