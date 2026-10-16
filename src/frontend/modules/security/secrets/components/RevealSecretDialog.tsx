// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconCopy } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { FormField } from '@resources/components/ui/FormField';
import { Textarea } from '@resources/components/ui/Textarea';
import { PasswordInput } from '@resources/components/ui/PasswordInput';
import { Input } from '@resources/components/ui/Input';
import { type Secret } from '../types';
import { useRevealFlow } from '../hooks/useRevealFlow';

type RevealSecretDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  secret: Secret | null;
};

/**
 * The reveal dialog.
 *
 * The plaintext lives in this component's state and nowhere else — not in the
 * Query cache (the hook is a mutation for exactly that reason), not in a
 * Zustand store, not in localStorage, not in the URL. It is cleared when the
 * dialog closes and auto-hidden after REVEAL_AUTO_HIDE_SECONDS even if the
 * dialog stays open, because the realistic exposure is an unattended screen.
 */
export function RevealSecretDialog({ open: isOpen, onOpenChange, secret }: RevealSecretDialogProps) {
  const { t } = useTranslation('secrets');
  const {
    reason,
    setReason,
    value,
    secondsLeft,
    password,
    setPassword,
    totpCode,
    setTotpCode,
    steppedUp,
    submit,
    submitDisabled,
  } = useRevealFlow(isOpen, secret);

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('reveal.title', { name: secret?.name ?? '' })}</DialogTitle>
          <DialogDescription>{t('reveal.description')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <Alert variant="warning" title={t('reveal.auditedTitle')}>
            {t('reveal.auditedBody')}
          </Alert>

          {value === '' ? (
            <>
              <FormField label={t('reveal.reason')} description={t('reveal.reasonHint')}>
                <Textarea
                  value={reason}
                  onChange={(entry) => setReason(entry.target.value)}
                  rows={3}
                  placeholder={t('reveal.reasonPlaceholder')}
                />
              </FormField>
              {!steppedUp && (
                <>
                  <FormField label={t('reveal.password')} description={t('reveal.stepUpHint')}>
                    <PasswordInput
                      value={password}
                      onChange={(eventItem) => setPassword(eventItem.target.value)}
                      placeholder={t('reveal.passwordPlaceholder')}
                    />
                  </FormField>
                  <FormField label={t('reveal.totp')}>
                    <Input
                      variant="outline"
                      value={totpCode}
                      onChange={(evt) => setTotpCode(evt.target.value)}
                      placeholder="123456"
                      inputMode="numeric"
                      autoComplete="one-time-code"
                    />
                  </FormField>
                </>
              )}
            </>
          ) : (
            <div className="space-y-1.5">
              <div className="flex items-center gap-2">
                <code className="flex-1 rounded bg-muted px-2 py-1.5 font-mono text-xs break-all">{value}</code>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t('clipboard.copy', { ns: 'common' })}
                  onClick={() => void navigator.clipboard.writeText(value)}
                >
                  <IconCopy className="size-4" />
                </Button>
              </div>
              <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <IconAlertTriangle className="size-3.5" />
                {t('reveal.autoHide', { seconds: secondsLeft })}
              </p>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {value === '' ? t('actions.cancel', { ns: 'common' }) : t('actions.close', { ns: 'common' })}
          </Button>
          {value === '' && (
            <Button onClick={submit} disabled={submitDisabled}>
              {t('reveal.submit')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
