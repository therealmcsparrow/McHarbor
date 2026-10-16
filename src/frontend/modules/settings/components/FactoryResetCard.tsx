// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconRefresh, IconAlertTriangle } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { useResetSystem } from '../hooks/useUpdates';

// FactoryResetCard renders the "Danger Zone" with the factory reset
// action. The flow is intentionally heavy: clicking the button opens a
// confirm dialog that requires the operator to type the literal
// string "RESET" before the destructive call is dispatched. The
// backend enforces the same confirmation string so an unattended
// browser cannot trigger the wipe.
export function FactoryResetCard() {
  const { t } = useTranslation('settings');
  const [isOpen, setOpen] = useState(false);
  const [confirmText, setConfirmText] = useState('');
  const reset = useResetSystem();
  const focusOnMount = useCallback((el: HTMLInputElement | null) => el?.focus(), []);
  const canReset = confirmText.trim() === 'RESET' && !reset.isPending;

  function startReset() {
    if (!canReset) return;
    reset.mutate('RESET', {
      onSuccess: () => {
        // The toast is dispatched by the mutation's `meta.success`
        // before the API process exits, so the user sees the
        // confirmation toast before the websocket dies.
        setOpen(false);
        setConfirmText('');
      },
    });
  }

  return (
    <div className="rounded-lg border border-destructive/40 bg-destructive/5 p-5">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <IconAlertTriangle className="mt-0.5 size-5 shrink-0 text-destructive" />
          <div>
            <h3 className="text-sm font-semibold text-foreground">{t('about.reset.title')}</h3>
            <p className="mt-1 text-sm text-muted-foreground">{t('about.reset.description')}</p>
          </div>
        </div>
        <Button variant="destructive" onClick={() => setOpen(true)} data-testid="reset-mcharbor-button">
          <IconRefresh className="size-4" />
          {t('about.reset.button')}
        </Button>
      </div>

      <ConfirmDialog
        open={isOpen}
        onOpenChange={(next) => {
          if (reset.isPending) return;
          setOpen(next);
          if (!next) setConfirmText('');
        }}
        title={t('about.reset.confirmTitle')}
        description={
          <div className="space-y-3">
            <p>{t('about.reset.confirmDescription')}</p>
            <p className="text-sm font-medium text-foreground">{t('about.reset.typePrompt', { value: 'RESET' })}</p>
            <Input
              ref={focusOnMount}
              value={confirmText}
              onChange={(e) => setConfirmText(e.target.value)}
              placeholder={t('placeholders.resetConfirm')}
              className="font-mono"
              disabled={reset.isPending}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && canReset) {
                  e.preventDefault();
                  startReset();
                }
              }}
            />
            {reset.isError && (
              <div className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive">
                {(reset.error as Error)?.message ?? t('about.toast.resetFailed')}
              </div>
            )}
          </div>
        }
        confirmLabel={reset.isPending ? t('about.reset.resetting') : t('about.reset.confirmButton')}
        variant="destructive"
        loading={reset.isPending}
        disabled={!canReset}
        onConfirm={startReset}
      />
    </div>
  );
}
