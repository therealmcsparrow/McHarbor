// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
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
import { cn } from '@resources/utils/cn';
import { Spinner } from '@resources/components/ui/Spinner';
import { VM_PERM } from '../permissions';
import { useCreateVMForm } from '../hooks/useCreateVMForm';
import { CreateSourceStep } from './create-vm/CreateSourceStep';
import { CreateResourcesStep } from './create-vm/CreateResourcesStep';
import { CreateStorageStep } from './create-vm/CreateStorageStep';
import { CreateNetworkStep } from './create-vm/CreateNetworkStep';
import { CreateSetupStep } from './create-vm/CreateSetupStep';
import { CreateSummaryStep } from './create-vm/CreateSummaryStep';

/**
 * Create a virtual machine.
 *
 * Six steps, and the first is the one that matters: an image, an installer
 * disc, or nothing at all. The image path is preselected because it is both the
 * common case and the one that reaches a login prompt in a couple of minutes;
 * the disc path exists for Windows and for distributions with no cloud image;
 * the blank path exists for a disk that already holds a system and for machines
 * that install over the network.
 *
 * Pools, networks and images come from the Compute Foundation plugin's API —
 * this plugin cannot read its tables, so the values it needs (a pool's backend
 * and target, a network's bridge) are read here and sent with the create.
 *
 * A host with no pool at all is not a dead end. The storage step then asks for
 * a data location instead of a pool, and submitting provisions a directory pool
 * there through compute's ensure route — every disk is still addressed through
 * a pool, but nobody has to go and make one first.
 */
export function CreateVMDialog({
  environments,
  onClose,
  onCreated,
}: {
  environments: { id: string; name: string }[];
  onClose: () => void;
  onCreated: (id: string) => void;
}) {
  const { t } = useTranslation('vm');
  const form = useCreateVMForm(environments, onCreated);
  const { step, steps, index } = form;
  const {
    setStep,
    source,
    template,
    dataLocation,
    ensurePool,
    pool,
    network,
    adopting,
    volume,
    canAdvance,
    create,
    cloneVM,
    submit,
  } = form;

  return (
    <Dialog open onOpenChange={(isOpen) => !isOpen && onClose()}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('create.title')}</DialogTitle>
          <DialogDescription>{t('description')}</DialogDescription>
        </DialogHeader>

        <ol className="flex flex-wrap gap-2" aria-label={t('create.title')}>
          {steps.map((entry, position) => (
            <li
              key={entry}
              aria-current={entry === step ? 'step' : undefined}
              className={cn(
                'flex items-center gap-1.5 rounded-full px-3 py-1 text-xs',
                position < index
                  ? 'bg-success/10 text-success'
                  : entry === step
                    ? 'bg-primary/10 text-primary'
                    : 'bg-muted text-muted-foreground',
              )}
            >
              {position < index && <IconCheck className="h-3 w-3" aria-hidden="true" />}
              {t(`create.steps.${entry}`)}
            </li>
          ))}
        </ol>

        <div className="min-h-[18rem] space-y-4 py-2">
          {step === 'source' && <CreateSourceStep form={form} environments={environments} />}
          {step === 'resources' && <CreateResourcesStep form={form} />}
          {step === 'storage' && <CreateStorageStep form={form} />}
          {step === 'network' && <CreateNetworkStep form={form} />}
          {step === 'setup' && <CreateSetupStep form={form} />}
          {step === 'summary' && <CreateSummaryStep form={form} />}
        </div>

        <DialogFooter>
          {index > 0 && (
            <Button variant="outline" onClick={() => setStep(steps[index - 1] ?? 'source')}>
              {t('actions.back')}
            </Button>
          )}
          {step === 'summary' ? (
            <Button
              permission={VM_PERM.MANAGE}
              disabled={
                source === 'template'
                  ? cloneVM.isPending || !template
                  : create.isPending ||
                    ensurePool.isPending ||
                    !network ||
                    !(pool || dataLocation.trim()) ||
                    (adopting && !volume)
              }
              onClick={() => void submit()}
            >
              {(create.isPending || ensurePool.isPending || cloneVM.isPending) && <Spinner className="mr-2 h-4 w-4" />}
              {t('create.submit')}
            </Button>
          ) : (
            <Button
              disabled={!canAdvance}
              onClick={() => setStep(steps[Math.min(index + 1, steps.length - 1)] ?? 'summary')}
            >
              {t('actions.next')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
