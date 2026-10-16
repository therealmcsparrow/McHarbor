// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { VM_PERM } from '../permissions';
import { VM_FEATURE } from '../hooks/useQEMUCapability';
import { useHostEnvironmentNames } from '../hooks/useHostEnvironments';
import { useCancelMigration, useMigrateVM, useMigrationStatus } from '../hooks/useMigration';
import { useMigrateForm } from '../hooks/useMigrateForm';
import type { VM } from '../types';
import { useHostFeature } from './HostFeatureGate';
import { MigrateProgress } from './MigrateProgress';
import { MigrateTargetStep, hostName } from './MigrateTargetStep';

/**
 * Move a guest to another host.
 *
 * Two steps, and only the second sends anything: choosing a target is
 * reversible, the move is not — a cold move shuts the guest down, and either
 * kind hands its disks to another machine. Once started the dialog shows the
 * server's progress, and closing it leaves the move running on the hosts.
 */
export function MigrateVMDialog({ vm, onClose }: { vm: VM; onClose: () => void }) {
  const { t } = useTranslation('vm');
  const cold = useHostFeature(vm.environmentId, VM_FEATURE.MIGRATE_COLD);
  const liveFeature = useHostFeature(vm.environmentId, VM_FEATURE.MIGRATE_LIVE);
  const running = vm.state === 'running';
  const liveAllowed = liveFeature.supported && running;
  const liveReason = liveFeature.supported ? t('migrate.liveNeedsRunning') : liveFeature.reason;

  const form = useMigrateForm(vm, true, liveAllowed);
  const names = useHostEnvironmentNames();
  const migrate = useMigrateVM();
  const cancel = useCancelMigration();
  const [step, setStep] = useState<'target' | 'confirm'>('target');
  const [started, setStarted] = useState(false);
  // The accepted request's own answer stands in until the first poll lands, so a
  // poll that races the orchestrator cannot blank the progress it just showed.
  const polled = useMigrationStatus(vm.id, true).data;
  const current = polled ?? (started ? migrate.data : undefined);
  // The pending flag reaches the button a render late; a second click inside
  // that window must not become a second move.
  const sending = useRef(false);

  const progress = (started || current?.state === 'running') && current ? current : null;
  const targetName = current ? hostName(form, current.targetEnvironmentId, names) : '';

  const submit = () => {
    if (!form.body || sending.current) return;
    sending.current = true;
    migrate.mutate(
      { id: vm.id, body: form.body },
      {
        onSuccess: () => setStarted(true),
        onSettled: () => {
          sending.current = false;
        },
      },
    );
  };

  return (
    <Dialog open onOpenChange={(isOpen) => !isOpen && onClose()}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('migrate.title', { name: vm.name })}</DialogTitle>
          <DialogDescription>{t('migrate.description')}</DialogDescription>
        </DialogHeader>

        {progress ? (
          <MigrateProgress status={progress} targetName={targetName} />
        ) : !cold.supported ? (
          <Alert variant="warning" description={cold.reason} />
        ) : step === 'target' ? (
          <MigrateTargetStep vm={vm} form={form} names={names} liveAllowed={liveAllowed} liveReason={liveReason} />
        ) : (
          <div className="space-y-3">
            <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
              <dt className="text-muted-foreground">{t('migrate.summary.from')}</dt>
              <dd className="text-foreground">{names.get(vm.environmentId) || vm.environmentId}</dd>
              <dt className="text-muted-foreground">{t('migrate.summary.to')}</dt>
              <dd className="text-foreground">{hostName(form, form.target, names)}</dd>
              <dt className="text-muted-foreground">{t('migrate.pool')}</dt>
              <dd className="text-foreground">{form.pool?.name}</dd>
              <dt className="text-muted-foreground">{t('migrate.summary.mode')}</dt>
              <dd className="text-foreground">{form.live ? t('migrate.summary.live') : t('migrate.summary.cold')}</dd>
            </dl>
            <Alert variant="warning" description={form.live ? t('migrate.confirmLive') : t('migrate.confirmCold')} />
          </div>
        )}

        {migrate.isError && !progress && <Alert variant="destructive" description={migrate.error.message} />}

        <DialogFooter>
          {progress ? (
            <>
              {progress.state === 'running' && (
                <Button
                  variant="destructive"
                  permission={VM_PERM.MIGRATE}
                  disabled={cancel.isPending}
                  onClick={() => cancel.mutate(vm.id)}
                >
                  {t('migrate.cancelMigration')}
                </Button>
              )}
              <Button variant="outline" onClick={onClose}>
                {t('actions.close', { ns: 'common' })}
              </Button>
            </>
          ) : step === 'target' ? (
            <>
              <Button variant="outline" onClick={onClose}>
                {t('actions.cancel')}
              </Button>
              <Button disabled={!form.ready || !cold.supported} onClick={() => setStep('confirm')}>
                {t('migrate.review')}
              </Button>
            </>
          ) : (
            <>
              <Button variant="outline" disabled={migrate.isPending} onClick={() => setStep('target')}>
                {t('actions.back')}
              </Button>
              <Button permission={VM_PERM.MIGRATE} disabled={migrate.isPending || !form.body} onClick={submit}>
                {migrate.isPending && <Spinner className="mr-2 h-4 w-4" />}
                {t('migrate.submit')}
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
