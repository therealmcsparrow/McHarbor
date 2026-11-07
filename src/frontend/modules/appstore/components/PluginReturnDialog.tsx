// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Checkbox } from '@resources/components/ui/Checkbox';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Spinner } from '@resources/components/ui/Spinner';
import { StatusError } from '@resources/components/ui/StatusError';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { usePluginReturn, usePluginReturnCheck } from '../hooks/usePluginReturn';

type PluginReturnDialogProps = {
  slug: string;
  name: string;
  installedVersion: string;
  onOpenChange: (open: boolean) => void;
};

/** Mount it only while open: each mount runs one check, because the server downloads the package to answer. */
export function PluginReturnDialog({
  slug,
  name: pluginName,
  installedVersion,
  onOpenChange,
}: PluginReturnDialogProps) {
  const { t } = useTranslation('plugins');
  const statusId = useId();
  const alertId = useId();
  const consentId = useId();
  const cancelRef = useRef<HTMLButtonElement>(null);
  const alertRef = useRef<HTMLDivElement>(null);
  const hadAlert = useRef(false);
  const canManage = usePermission(PERM.PLUGINS_MANAGE);
  const [consent, setConsent] = useState(false);
  const { check, isPending, error, recheck } = usePluginReturnCheck(slug, canManage);
  const move = usePluginReturn(slug, pluginName);

  const refusal = move.error?.message ?? '';
  const outcome = check?.outcome;
  const unsigned = outcome === 'unsigned';
  const consentOn = consent && refusal === '';
  const ready = outcome === 'clear' || (unsigned && consentOn);
  const settled = !isPending && !error && check !== undefined;
  const dead = !canManage || (settled && !unsigned && outcome !== 'clear') || error !== null;
  const canRetry = canManage && (error !== null || refusal !== '');
  const confirmDisabled = !ready || !settled || refusal !== '' || move.isPending;

  let text = '';
  let isAlert = false;
  if (!canManage) {
    text = t('channels.needs', { permission: PERM.PLUGINS_MANAGE });
    isAlert = true;
  } else if (isPending) {
    text = t('return.checking', { name: pluginName });
  } else if (refusal !== '') {
    text = refusal;
    isAlert = true;
  } else if (error) {
    text = error.message;
    isAlert = true;
  } else if (outcome === 'clear') {
    text = t('return.ready');
  } else if (outcome === 'no-stable') {
    text = t('return.noStable', { name: pluginName });
    isAlert = true;
  } else if (outcome === 'prerelease-host') {
    text = t('return.prereleaseHost');
    isAlert = true;
  } else if (unsigned) {
    text = consentOn ? t('return.consentGiven') : t('return.unsigned');
    isAlert = !consentOn;
  } else if (outcome === 'blocked') {
    text =
      check?.blockingMigration && !check.unrecognised
        ? t('return.blocked', { migration: check.blockingMigration })
        : t('return.blockedGeneric');
    isAlert = true;
  }

  const statusText = isAlert ? '' : text;
  const alertText = isAlert ? text : '';

  // A blocking message appears while focus sits on Cancel; moving focus to the alert is what tells a keyboard user it did.
  useEffect(() => {
    if (alertText !== '' && !hadAlert.current) alertRef.current?.focus();
    hadAlert.current = alertText !== '';
  }, [alertText]);

  function submit() {
    if (confirmDisabled || move.isPending) return;
    move.mutate({ allowUnsigned: unsigned && consentOn }, { onSuccess: () => onOpenChange(false) });
  }

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent
        className="max-w-md"
        aria-describedby={`${statusId} ${alertId}`}
        onOpenAutoFocus={(evt) => {
          evt.preventDefault();
          cancelRef.current?.focus();
        }}
      >
        <DialogHeader>
          <DialogTitle>{t('return.title', { name: pluginName })}</DialogTitle>
        </DialogHeader>
        <DialogBody className="space-y-3 p-4">
          <p className="text-sm text-foreground">
            {check?.targetVersion
              ? t('return.summary', { from: installedVersion, to: check.targetVersion })
              : t('return.summaryFrom', { from: installedVersion })}
          </p>
          <div className="space-y-2 text-sm text-muted-foreground">
            <output id={statusId} className="flex items-start gap-2">
              {isPending && <Spinner className="mt-0.5 size-4" />}
              <span>{statusText}</span>
            </output>
            <div
              id={alertId}
              ref={alertRef}
              role="alert"
              tabIndex={-1}
              className="rounded-sm focus:ring-2 focus:ring-ring focus:outline-none"
            >
              {alertText}
            </div>
            {error && !isPending && <StatusError error={error.message} title={t('return.checkFailed')} />}
          </div>
          {unsigned && canManage && (
            <label htmlFor={consentId} className="flex items-start gap-2 text-sm text-foreground">
              <Checkbox
                id={consentId}
                checked={consentOn}
                disabled={move.isPending}
                onCheckedChange={(next) => setConsent(next === true)}
              />
              <span>{t('return.consent', { name: pluginName })}</span>
            </label>
          )}
        </DialogBody>
        <DialogFooter>
          {canRetry && (
            <Button
              variant="outline"
              onClick={() => {
                move.reset();
                setConsent(false);
                recheck();
              }}
            >
              {t('return.checkAgain')}
            </Button>
          )}
          <Button ref={cancelRef} variant="outline" onClick={() => onOpenChange(false)}>
            {dead || refusal !== '' ? t('actions.close', { ns: 'common' }) : t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button
            className="aria-disabled:cursor-not-allowed aria-disabled:opacity-50"
            onClick={submit}
            aria-disabled={confirmDisabled ? true : undefined}
            aria-describedby={`${alertId} ${statusId}`}
            permission={PERM.PLUGINS_MANAGE}
          >
            {move.isPending && <Spinner className="size-4" />}
            {t('return.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
