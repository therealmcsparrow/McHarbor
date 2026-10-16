// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import { formatDate } from '@resources/utils/format';
import { useQueryClient } from '@tanstack/react-query';
import { IconPower } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { StatusError } from '@resources/components/ui/StatusError';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { TooltipFocusTarget } from '@resources/components/ui/TooltipFocusTarget';
import { hostTargetKey, type HostTarget } from '@resources/hooks/useHostUpdates';
import {
  isRebootReason,
  rebootInFlight,
  useCancelHostReboot,
  useHostRebootState,
  type HostReboot,
  type HostRebootStatus,
} from '@resources/hooks/useHostReboot';
import { HostRebootDialog } from './HostRebootDialog';
import { hostPermission } from '@resources/hooks/useHostUpdates';

type HostRebootCardProps = {
  target: HostTarget;
  rebootRequired: boolean;
  disabled: boolean;
};

const STATUS_VARIANT: Record<HostRebootStatus, 'info' | 'warning' | 'success' | 'destructive'> = {
  scheduled: 'info',
  running: 'warning',
  stopping_workloads: 'warning',
  rebooting: 'warning',
  done: 'success',
  failed: 'destructive',
};

const PHASES = ['detect', 'swarm', 'guests', 'containers', 'reboot'] as const;

function isPhase(value: string | undefined): value is (typeof PHASES)[number] {
  return PHASES.some((phase) => phase === value);
}

function useRebootDoneRefresh(target: HostTarget, currentStatus: HostRebootStatus | undefined) {
  const queryClient = useQueryClient();
  const previous = useRef(currentStatus);
  useEffect(() => {
    if (currentStatus === 'done' && previous.current && previous.current !== 'done') {
      void queryClient.invalidateQueries({ queryKey: ['host-os-info', hostTargetKey(target)] });
    }
    previous.current = currentStatus;
  }, [currentStatus, target, queryClient]);
}

function RebootStatusLine({ reboot }: { reboot: HostReboot }) {
  const { t } = useTranslation('environments');
  const formatPrefs = useLocaleFormat();
  const reason = isRebootReason(reboot.failureReason) ? t(`detail.hostReboot.reason.${reboot.failureReason}`) : '';
  const error = [reason, reboot.lastError ?? ''].filter(Boolean).join('\n\n');
  const whenAt = formatDate(reboot.scheduledAt, formatPrefs);

  return (
    <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
      <Badge variant={STATUS_VARIANT[reboot.status]}>
        {rebootInFlight(reboot.status) ? <Spinner className="mr-1.5 h-3 w-3" /> : null}
        {t(`detail.hostReboot.status.${reboot.status}`)}
      </Badge>
      <StatusError error={error} title={t('detail.hostReboot.errorTitle')} />
      {rebootInFlight(reboot.status) && isPhase(reboot.phase) ? (
        <span>{t(`detail.hostReboot.phase.${reboot.phase}`)}</span>
      ) : null}
      <span>{t('detail.hostReboot.scheduledFor', { when: whenAt })}</span>
      {reboot.requestedBy ? <span>{t('detail.hostReboot.requestedBy', { user: reboot.requestedBy })}</span> : null}
      {reboot.finishedAt ? (
        <span>{t('detail.hostReboot.finishedAt', { when: formatDate(reboot.finishedAt, formatPrefs) })}</span>
      ) : null}
    </div>
  );
}

export function HostRebootCard({ target, rebootRequired, disabled }: HostRebootCardProps) {
  const { t } = useTranslation('environments');
  const [isOpen, setOpen] = useState(false);
  const state = useHostRebootState(target, true);
  const cancel = useCancelHostReboot(target);
  const reboot = state.data?.reboot;
  useRebootDoneRefresh(target, reboot?.status);

  if (!rebootRequired && !reboot) return null;

  const demo = state.data?.blockedReason === 'demo_mode';
  const inFlight = rebootInFlight(reboot?.status);
  const rebootButton = (
    <Button
      size="sm"
      variant="destructive"
      permission={hostPermission(target, 'reboot')}
      disabled={disabled || demo || inFlight || state.isPending}
      onClick={() => setOpen(true)}
    >
      <IconPower className="mr-1.5 h-4 w-4" aria-hidden="true" />
      {t('detail.hostReboot.rebootMachine')}
    </Button>
  );

  return (
    <div className="mc-glass rounded-lg p-6">
      <div className="mb-3 flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="text-sm font-medium text-foreground">{t('detail.hostReboot.title')}</h3>
          <p className="mt-0.5 text-xs text-muted-foreground">
            {rebootRequired ? t('detail.hostReboot.requiredDescription') : t('detail.hostReboot.idleDescription')}
          </p>
        </div>
        <div className="flex items-center gap-2">
          {reboot?.status === 'scheduled' ? (
            <Button
              size="sm"
              variant="outline"
              permission={hostPermission(target, 'reboot')}
              disabled={cancel.isPending}
              onClick={() => cancel.mutate()}
            >
              {cancel.isPending ? <Spinner className="mr-1.5 h-4 w-4" /> : null}
              {t('detail.hostReboot.cancel')}
            </Button>
          ) : null}
          {rebootRequired && demo ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <TooltipFocusTarget>{rebootButton}</TooltipFocusTarget>
              </TooltipTrigger>
              <TooltipContent>{t('detail.hostReboot.demoBlocked')}</TooltipContent>
            </Tooltip>
          ) : rebootRequired ? (
            rebootButton
          ) : null}
        </div>
      </div>

      {reboot ? <RebootStatusLine reboot={reboot} /> : null}
      {cancel.isError ? <Alert variant="destructive" className="mt-3" description={cancel.error.message} /> : null}

      <HostRebootDialog target={target} open={isOpen} blocked={demo} onOpenChange={setOpen} />
    </div>
  );
}
