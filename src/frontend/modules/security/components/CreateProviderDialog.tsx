// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
  DialogBody,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { useIdentityAdapters } from '@resources/hooks/useIdentityAdapters';
import { PERM } from '@core/auth/permissionKeys';
import { WIZARD_STEPS, useProviderWizard, type WizardStep } from './identity/useProviderWizard';
import { WizardConfigureStep, WizardTypeStep } from './identity/WizardSteps';
import { WizardAccessStep, WizardReviewStep } from './identity/WizardAccessSteps';
import { WizardDoneStep } from './identity/WizardDoneStep';

type CreateProviderDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

const PREVIOUS: Partial<Record<WizardStep, WizardStep>> = { configure: 'type', access: 'configure', review: 'access' };
const NEXT: Partial<Record<WizardStep, WizardStep>> = { configure: 'access', access: 'review' };

export function CreateProviderDialog({ open: isOpen, onOpenChange }: CreateProviderDialogProps) {
  const { t } = useTranslation('security');
  const wizard = useProviderWizard();
  const { data: adapters, isLoading } = useIdentityAdapters();
  const spec = wizard.providerType ? (adapters?.[wizard.providerType] ?? null) : null;
  const step = wizard.step;

  const handleOpenChange = (value: boolean) => {
    if (!value) wizard.reset();
    onOpenChange(value);
  };

  const previous = PREVIOUS[step];
  const next = NEXT[step];

  return (
    <Dialog open={isOpen} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t(`wizard.steps.${step}`, { ns: 'access' })}</DialogTitle>
          <DialogDescription>
            {step === 'type' ? t('identity.selectTypeDescription') : (spec?.DisplayName ?? wizard.providerType ?? '')}
          </DialogDescription>
          <ol className="mt-2 flex flex-wrap gap-2 text-xs" aria-label={t('wizard.progress', { ns: 'access' })}>
            {WIZARD_STEPS.map((id, index) => (
              <li
                key={id}
                aria-current={id === step ? 'step' : undefined}
                className={id === step ? 'font-semibold text-foreground' : 'text-muted-foreground'}
              >
                {`${index + 1}. ${t(`wizard.steps.${id}`, { ns: 'access' })}`}
              </li>
            ))}
          </ol>
        </DialogHeader>

        <DialogBody className="max-h-[65vh] overflow-y-auto">
          {step === 'type' && (
            <WizardTypeStep
              adapters={Object.values(adapters ?? {})}
              isLoading={isLoading}
              onSelect={wizard.selectType}
            />
          )}
          {step === 'configure' && <WizardConfigureStep wizard={wizard} spec={spec} />}
          {step === 'access' && <WizardAccessStep wizard={wizard} spec={spec} />}
          {step === 'review' && <WizardReviewStep wizard={wizard} spec={spec} />}
          {step === 'done' && wizard.created && (
            <WizardDoneStep provider={wizard.created} spec={spec} mapping={wizard.mapping} />
          )}
          {wizard.create.isError && (
            <p
              role="alert"
              className="mt-3 rounded-md border border-destructive/40 bg-destructive/10 p-3 text-xs break-words whitespace-pre-wrap text-destructive"
            >
              {wizard.create.error.message}
            </p>
          )}
        </DialogBody>

        {step !== 'type' && (
          <DialogFooter className="flex flex-wrap items-center justify-end gap-2">
            {previous && (
              <Button variant="outline" onClick={() => wizard.setStep(previous)}>
                {t('common:actions.back')}
              </Button>
            )}
            {next && (
              <Button onClick={() => wizard.setStep(next)} disabled={!wizard.nameValid}>
                {t('wizard.next', { ns: 'access' })}
              </Button>
            )}
            {step === 'review' && (
              <Button
                permission={PERM.IDENTITY_PROVIDERS_MANAGE}
                onClick={wizard.submit}
                disabled={!wizard.nameValid || wizard.create.isPending}
              >
                {wizard.create.isPending ? t('common:actions.processing') : t('identity.addProvider')}
              </Button>
            )}
            {step === 'done' && (
              <Button onClick={() => handleOpenChange(false)}>{t('wizard.finish', { ns: 'access' })}</Button>
            )}
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}
