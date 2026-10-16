// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Checkbox } from '@resources/components/ui/Checkbox';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import type { HostTarget } from '@resources/hooks/useHostUpdates';
import { useHostRebootInventory, useScheduleHostReboot, type HostRebootRequest } from '@resources/hooks/useHostReboot';
import { HostRebootDisclaimers } from './HostRebootDisclaimers';
import { hostPermission } from '@resources/hooks/useHostUpdates';

type HostRebootDialogProps = {
  target: HostTarget;
  open: boolean;
  blocked?: boolean;
  onOpenChange: (open: boolean) => void;
};

function toLocalInput(date: Date): string {
  const shifted = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return shifted.toISOString().slice(0, 16);
}

// Read when the dialog opens, so the suggested time is an hour from that moment rather than from mount.
function defaultRebootTime(): string {
  return toLocalInput(new Date(Date.now() + 60 * 60_000));
}

export function HostRebootDialog({ target, open: isOpen, blocked = false, onOpenChange }: HostRebootDialogProps) {
  const { t } = useTranslation('environments');
  const schedule = useScheduleHostReboot(target);
  const inventory = useHostRebootInventory(target, isOpen);
  const baseId = useId();
  const [now, setNow] = useState(true);
  const [whenAt, setWhen] = useState('');
  const [drained, setDrained] = useState(false);

  const [wasOpen, setWasOpen] = useState(false);
  if (wasOpen !== isOpen) {
    setWasOpen(isOpen);
    if (isOpen) {
      setNow(true);
      setWhen(defaultRebootTime());
      setDrained(false);
    }
  }

  const scheduledAt = whenAt ? new Date(whenAt) : null;
  const validTime = scheduledAt !== null && !Number.isNaN(scheduledAt.getTime());
  const inv = inventory.data;
  const needsDrain = Boolean(inv?.kubernetes?.length) || (inv?.swarm === 'worker' && inv.swarmTasks > 0);

  const variables: HostRebootRequest = {
    confirm: true,
    now,
    drainedExternally: drained,
    ...(now || !scheduledAt || !validTime ? {} : { scheduledAt: scheduledAt.toISOString() }),
  };

  return (
    <MutationConfirmDialog
      open={isOpen}
      onOpenChange={onOpenChange}
      variant="destructive"
      title={t('detail.hostReboot.dialog.title')}
      description={t('detail.hostReboot.dialog.description')}
      confirmLabel={t('detail.hostReboot.rebootMachine')}
      permission={hostPermission(target, 'reboot')}
      mutation={schedule}
      variables={variables}
      disabled={blocked || (!now && !validTime) || (needsDrain && !drained)}
    >
      <div className="space-y-4">
        <div className="space-y-2">
          <p className="text-sm font-medium text-foreground">{t('detail.hostReboot.dialog.when')}</p>
          <div className="flex flex-wrap gap-2">
            <Button
              type="button"
              size="sm"
              variant={now ? 'default' : 'outline'}
              aria-pressed={now}
              onClick={() => setNow(true)}
            >
              {t('detail.hostReboot.dialog.now')}
            </Button>
            <Button
              type="button"
              size="sm"
              variant={now ? 'outline' : 'default'}
              aria-pressed={!now}
              onClick={() => setNow(false)}
            >
              {t('detail.hostReboot.dialog.later')}
            </Button>
          </div>
          {now ? null : (
            <div className="space-y-1">
              <Label htmlFor={`${baseId}-when`}>{t('detail.hostReboot.dialog.dateTime')}</Label>
              <Input
                id={`${baseId}-when`}
                type="datetime-local"
                value={whenAt}
                onChange={(evt) => setWhen(evt.target.value)}
              />
              <p className="text-xs text-muted-foreground">{t('detail.hostReboot.dialog.dateTimeHint')}</p>
            </div>
          )}
        </div>

        <HostRebootDisclaimers inventory={inv} loading={inventory.isPending && isOpen} self={target.kind === 'self'} />

        {needsDrain ? (
          <div className="flex items-start gap-3">
            <Checkbox
              id={`${baseId}-drained`}
              checked={drained}
              onCheckedChange={(next) => setDrained(next === true)}
            />
            <div className="min-w-0">
              <Label htmlFor={`${baseId}-drained`} className="text-sm font-normal">
                {t('detail.hostReboot.dialog.drainedLabel')}
              </Label>
              <p className="mt-0.5 text-xs text-muted-foreground">{t('detail.hostReboot.dialog.drainedDescription')}</p>
            </div>
          </div>
        ) : null}
      </div>
    </MutationConfirmDialog>
  );
}
