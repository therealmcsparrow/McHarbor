// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { QRCodeSVG } from 'qrcode.react';
import { IconCheck, IconCopy, IconX } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Spinner } from '@resources/components/ui/Spinner';
import { useMFAConfirmEnrollment } from '@resources/hooks/useMFA';

type Props = {
  /** The `otpauth://` URI the authenticator app consumes. */
  otpauthUri: string;
  /** Base32 secret, offered as the manual-entry fallback. */
  secret: string;
  recoveryCodes: string[];
  onCancel: () => void;
  onConfirmed: () => void;
};

/**
 * The three-step enrollment wizard: scan, save recovery codes, verify.
 *
 * Split out of MFASettingsPanel so each stays inside the 200-line
 * component budget, and so the panel only deals with MFA *state* while
 * this component deals with the one-time secret material.
 */
export function MFAEnrollmentSteps({ otpauthUri, secret, recoveryCodes, onCancel, onConfirmed }: Props) {
  const { t } = useTranslation('common');
  const confirmAction = useMFAConfirmEnrollment();

  const [verifyCode, setVerifyCode] = useState('');
  const [copiedCodes, setCopiedCodes] = useState(false);
  const [copiedSecret, setCopiedSecret] = useState(false);

  function handleConfirm() {
    if (verifyCode.length !== 6) return;
    confirmAction.mutate({ code: verifyCode }, { onSuccess: onConfirmed });
  }

  async function copyText(value: string, onCopied: (v: boolean) => void) {
    try {
      await navigator.clipboard.writeText(value);
      onCopied(true);
      window.setTimeout(() => onCopied(false), 2000);
    } catch {
      // Clipboard access is denied in some browser contexts; the
      // value stays selectable on screen, so there is nothing to
      // report to the user here.
    }
  }

  return (
    <div className="mc-glass space-y-5 rounded-md p-4">
      <div>
        <p className="text-sm font-medium">{t('mfa.step1Title')}</p>
        <p className="text-xs text-muted-foreground">{t('mfa.step1Help')}</p>
        <div className="mt-3 flex flex-col gap-3 sm:flex-row sm:items-start">
          {/* The QR paints its own white ground through component
              props rather than a Tailwind colour, so it keeps the
              contrast a scanner needs in either theme. */}
          <div className="w-fit rounded-md border border-border p-3">
            <QRCodeSVG
              value={otpauthUri}
              size={160}
              bgColor="#ffffff"
              fgColor="#000000"
              level="M"
              title={t('mfa.qrAlt')}
            />
          </div>
          <div className="min-w-0 flex-1">
            <p className="text-xs font-medium text-muted-foreground">{t('mfa.manualEntry')}</p>
            <div className="mt-1 flex items-start gap-2">
              <code className="min-w-0 flex-1 rounded-md bg-background/60 p-2 font-mono text-xs break-all">
                {secret}
              </code>
              <Button size="sm" variant="outline" onClick={() => void copyText(secret, setCopiedSecret)}>
                {copiedSecret ? <IconCheck className="size-3.5" /> : <IconCopy className="size-3.5" />}
                {copiedSecret ? t('mfa.copied') : t('mfa.copy')}
              </Button>
            </div>
          </div>
        </div>
      </div>

      <div>
        <p className="text-sm font-medium">{t('mfa.step2Title')}</p>
        <p className="text-xs text-muted-foreground">{t('mfa.step2Help')}</p>
        <div className="mt-2 grid grid-cols-2 gap-2 sm:grid-cols-4">
          {recoveryCodes.map((code) => (
            <code
              key={code}
              className="rounded-md border border-border bg-background/60 px-2 py-1 text-center font-mono text-xs"
            >
              {code}
            </code>
          ))}
        </div>
        <Button
          size="sm"
          variant="ghost"
          onClick={() => void copyText(recoveryCodes.join('\n'), setCopiedCodes)}
          className="mt-2"
        >
          {copiedCodes ? <IconCheck className="size-3.5" /> : <IconCopy className="size-3.5" />}
          {copiedCodes ? t('mfa.copied') : t('mfa.copyCodes')}
        </Button>
      </div>

      <div>
        <p className="text-sm font-medium">{t('mfa.step3Title')}</p>
        <p className="text-xs text-muted-foreground">{t('mfa.step3Help')}</p>
        <div className="mt-2 flex flex-wrap items-center gap-2">
          <Input
            value={verifyCode}
            onChange={(e) => setVerifyCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
            placeholder="123456"
            inputMode="numeric"
            autoComplete="one-time-code"
            maxLength={6}
            aria-label={t('mfa.step3Title')}
            className="w-32 font-mono"
          />
          <Button onClick={handleConfirm} disabled={verifyCode.length !== 6 || confirmAction.isPending}>
            {confirmAction.isPending ? <Spinner size="sm" /> : t('mfa.confirm')}
          </Button>
          <Button variant="ghost" size="sm" onClick={onCancel}>
            <IconX className="size-3.5" /> {t('mfa.cancel')}
          </Button>
        </div>
        {confirmAction.isError && (
          <div role="alert" className="mt-2 text-xs text-danger">
            {t('mfa.confirmFailed')}
          </div>
        )}
      </div>

      <p className="text-[10px] text-muted-foreground">{t('mfa.footerHelp')}</p>
    </div>
  );
}
