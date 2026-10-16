// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useHostEnvironment } from '../hooks/useHostEnvironment';
import { IconCheck } from '@tabler/icons-react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Select } from '@resources/components/ui/Select';
import { COMPUTE_PERM } from '../permissions';
import { useCapability, useNetworks, usePools, useProbe, useUpdateEnvironment } from '../hooks/useCompute';
import { CapabilityPanel } from './CapabilityPanel';
import { WizardStepper } from './WizardStepper';
import { CreatePoolDialog } from './CreatePoolDialog';
import { CreateNetworkDialog } from './CreateNetworkDialog';

/**
 * The compute setup wizard.
 *
 * Five steps, in the order the dependencies actually run: pick a host, see what
 * it can do, give it somewhere to keep disks, give it a network, done.
 *
 * The capability step is not skippable and not decorative. It is the point at
 * which an operator learns that their host needs one package or one firmware
 * setting — and the step exists precisely so they learn it here, in thirty
 * seconds, rather than at the first failed guest create.
 */

type Step = 'host' | 'capability' | 'pool' | 'network' | 'summary';

const STEPS: Step[] = ['host', 'capability', 'pool', 'network', 'summary'];

interface SetupWizardProps {
  environments: { id: string; name: string }[];
  defaultEnvironmentId?: string;
  onClose: () => void;
}

export function SetupWizard({ environments, defaultEnvironmentId, onClose }: SetupWizardProps) {
  const { t } = useTranslation('compute');

  const [step, setStep] = useState<Step>(defaultEnvironmentId ? 'capability' : 'host');
  // The host's topbar selection comes before whatever sorted first, which is
  // what this used to offer regardless of what the user had chosen.
  const hostEnvironmentId = useHostEnvironment();
  const [environmentId, setEnvironmentId] = useState(
    defaultEnvironmentId || hostEnvironmentId || environments[0]?.id || '',
  );
  const [computeTurnedOn, setComputeTurnedOn] = useState(false);
  const [creatingPool, setCreatingPool] = useState(false);
  const [creatingNetwork, setCreatingNetwork] = useState(false);

  const { data: env } = useCapability(environmentId || undefined);
  const { data: pools } = usePools(environmentId || undefined);
  const { data: networks } = useNetworks(environmentId || undefined);
  const probe = useProbe();
  const updateEnvironment = useUpdateEnvironment();

  const index = STEPS.indexOf(step);
  const hasPool = (pools ?? []).length > 0;
  const hasNetwork = (networks ?? []).length > 0;

  // A host that cannot run guests has nothing for the remaining steps to
  // configure: a pool and a network exist to carry virtual machines, and this
  // host will carry none. The wizard says so and offers the way out rather than
  // a disabled Next that leaves the operator looking for what they missed.
  const blocked = step === 'capability' && Boolean(env) && !(env?.available && env?.capability?.canRunVms);

  const canAdvance =
    step === 'host'
      ? Boolean(environmentId)
      : step === 'capability'
        ? Boolean(env?.available)
        : step === 'pool'
          ? hasPool
          : step === 'network'
            ? hasNetwork
            : true;

  const next = () => {
    if (step === 'host') {
      probe.mutate(environmentId);
    }
    // Turning compute on is what makes the host's surfaces appear at all, so it
    // happens at the first advance rather than at the end — a wizard abandoned
    // half way still leaves the host usable. It cannot hang off the host step:
    // a wizard opened for a host that is already chosen starts at the
    // capability step, and every green run then left compute switched off.
    if (!computeTurnedOn && environmentId) {
      updateEnvironment.mutate({ envId: environmentId, computeEnabled: true });
      setComputeTurnedOn(true);
    }
    const target = STEPS[Math.min(index + 1, STEPS.length - 1)] ?? 'summary';
    setStep(target);
  };

  return (
    <Dialog open onOpenChange={(isOpen) => !isOpen && onClose()}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('wizard.title')}</DialogTitle>
          <DialogDescription>{t('wizard.subtitle')}</DialogDescription>
        </DialogHeader>

        <WizardStepper steps={STEPS} step={step} index={index} />

        <div className="min-h-[16rem] py-2">
          {step === 'host' && (
            <div className="space-y-3">
              <h3 className="font-medium text-foreground">{t('wizard.host.title')}</h3>
              <p className="text-sm text-muted-foreground">{t('wizard.host.body')}</p>
              {environments.length === 0 ? (
                <p className="text-sm text-warning">{t('wizard.host.noAgentHosts')}</p>
              ) : (
                <Select
                  value={environmentId}
                  ariaLabel={t('wizard.steps.host')}
                  options={environments.map((entry) => ({ value: entry.id, label: entry.name }))}
                  onChange={setEnvironmentId}
                />
              )}
            </div>
          )}

          {step === 'capability' && (
            <div className="space-y-3">
              <h3 className="font-medium text-foreground">{t('wizard.capability.title')}</h3>
              <p className="text-sm text-muted-foreground">{t('wizard.capability.body')}</p>
              {/*
                The panel owns the probe from here on. This step used to swap
                itself for a spinner while `probe.isPending`, but that was this
                component's own `useProbe()` — a different mutation object from
                the one behind the panel's button, so the panel's button could
                never drive it, and pressing it produced no visible change at
                all. The panel now renders its own pending and failure states.
              */}
              <CapabilityPanel
                environmentId={environmentId}
                capability={env?.capability}
                probedAt={env?.probedAt}
                available={Boolean(env?.available)}
                unavailableReason={env?.unavailableReason}
                agentVersionCurrent={env?.capability?.agentVersion}
              />
            </div>
          )}

          {step === 'pool' && (
            <div className="space-y-3">
              <h3 className="font-medium text-foreground">{t('wizard.pool.title')}</h3>
              <p className="text-sm text-muted-foreground">{t('wizard.pool.body')}</p>
              {hasPool ? (
                <ul className="space-y-1 text-sm text-foreground">
                  {(pools ?? []).map((pool) => (
                    <li key={pool.id} className="flex items-center gap-2">
                      <IconCheck className="h-4 w-4 text-success" aria-hidden="true" />
                      {pool.name} · {t(`pools.backend.${pool.backend}`)}
                    </li>
                  ))}
                </ul>
              ) : (
                <Button permission={COMPUTE_PERM.MANAGE} onClick={() => setCreatingPool(true)}>
                  {t('actions.createPool')}
                </Button>
              )}
            </div>
          )}

          {step === 'network' && (
            <div className="space-y-3">
              <h3 className="font-medium text-foreground">{t('wizard.network.title')}</h3>
              <p className="text-sm text-muted-foreground">{t('wizard.network.body')}</p>
              {hasNetwork ? (
                <ul className="space-y-1 text-sm text-foreground">
                  {(networks ?? []).map((network) => (
                    <li key={network.id} className="flex items-center gap-2">
                      <IconCheck className="h-4 w-4 text-success" aria-hidden="true" />
                      {network.name} · {network.bridge}
                    </li>
                  ))}
                </ul>
              ) : (
                <Button permission={COMPUTE_PERM.MANAGE} onClick={() => setCreatingNetwork(true)}>
                  {t('actions.createNetwork')}
                </Button>
              )}
            </div>
          )}

          {step === 'summary' && (
            <div className="space-y-3">
              <h3 className="font-medium text-foreground">{t('wizard.summary.title')}</h3>
              <p className="text-sm text-muted-foreground">{t('wizard.summary.body')}</p>
            </div>
          )}
        </div>

        <DialogFooter>
          {index > 0 && (
            <Button variant="outline" onClick={() => setStep(STEPS[index - 1] ?? 'host')}>
              {t('wizard.back')}
            </Button>
          )}
          {step === 'summary' || blocked ? (
            <Button onClick={onClose}>{blocked ? t('wizard.close') : t('wizard.finish')}</Button>
          ) : (
            <Button onClick={next} disabled={!canAdvance}>
              {t('wizard.next')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>

      {creatingPool && (
        <CreatePoolDialog
          environments={environments}
          defaultEnvironmentId={environmentId}
          onClose={() => setCreatingPool(false)}
        />
      )}
      {creatingNetwork && (
        <CreateNetworkDialog
          environments={environments}
          defaultEnvironmentId={environmentId}
          onClose={() => setCreatingNetwork(false)}
        />
      )}
    </Dialog>
  );
}
