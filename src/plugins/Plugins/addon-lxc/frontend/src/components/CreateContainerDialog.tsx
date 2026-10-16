// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { CT_PERM } from '../permissions';
import { NetworkStep } from './create-container/NetworkStep';
import { ProfileStep } from './create-container/ProfileStep';
import { ResourcesStep } from './create-container/ResourcesStep';
import { SetupStep } from './create-container/SetupStep';
import { SourceStep } from './create-container/SourceStep';
import { SummaryStep } from './create-container/SummaryStep';
import { STEPS, useCreateContainerForm } from './create-container/useCreateContainerForm';

/**
 * Create a system container.
 *
 * Five steps rather than one long form, because the decisions are of different
 * kinds and mixing them makes each one worse: what to run, how isolated it is,
 * what it is allowed, how it reaches the network, and how somebody gets into it.
 *
 * The isolation step is the one that matters. Unprivileged is the default and
 * the recommendation, privileged is behind the permission and a warning, and
 * the two are described in terms of what they mean rather than what they are
 * called — "root inside is root on the host" tells an operator something that
 * "no user namespace" does not.
 */

interface CreateContainerDialogProps {
  environments: { id: string; name: string }[];
  onClose: () => void;
  onCreated: (id: string) => void;
}

export function CreateContainerDialog({ environments, onClose, onCreated }: CreateContainerDialogProps) {
  const { t } = useTranslation('ct');
  const form = useCreateContainerForm(environments, onCreated);
  const { step, setStep, index, last, create, ctName, poolId, submit } = form;

  return (
    <Dialog open onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('create.title')}</DialogTitle>
        </DialogHeader>

        <nav className="flex flex-wrap gap-2" aria-label={t('create.title')}>
          {STEPS.map((key, position) => (
            <span
              key={key}
              className={
                position === index
                  ? 'rounded-full bg-primary px-2.5 py-0.5 text-xs text-primary-foreground'
                  : 'rounded-full bg-muted px-2.5 py-0.5 text-xs text-muted-foreground'
              }
            >
              {t(`create.steps.${key}`)}
            </span>
          ))}
        </nav>

        <div className="min-h-[18rem] space-y-4">
          {step === 'source' && <SourceStep form={form} />}
          {step === 'profile' && <ProfileStep form={form} />}
          {step === 'resources' && <ResourcesStep form={form} />}
          {step === 'network' && <NetworkStep form={form} />}
          {step === 'setup' && <SetupStep form={form} />}
          {step === 'summary' && <SummaryStep form={form} />}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t('actions.cancel')}
          </Button>
          {index > 0 && (
            <Button variant="outline" onClick={() => setStep(STEPS[index - 1] ?? 'source')}>
              {t('actions.back')}
            </Button>
          )}
          {last ? (
            <Button permission={CT_PERM.MANAGE} disabled={!ctName || !poolId || create.isPending} onClick={submit}>
              {create.isPending && <Spinner className="mr-2 h-4 w-4" />}
              {t('actions.create')}
            </Button>
          ) : (
            <Button onClick={() => setStep(STEPS[index + 1] ?? 'summary')}>{t('actions.next')}</Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
