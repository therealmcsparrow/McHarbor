// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { IconChevronLeft, IconChevronRight, IconX, IconCheck, IconServer, IconApps, IconLayoutDashboard, IconBell } from '@tabler/icons-react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { useOnboardingStore, WIZARD_STEPS, type WizardStepId } from '@resources/stores/onboarding';
import { cn } from '@resources/utils/cn';

type StepContentProps = {
  stepId: WizardStepId;
  onComplete: () => void;
};

function StepContent({ stepId, onComplete }: StepContentProps) {
  const { t } = useTranslation();
  const navigate = useNavigate();

  switch (stepId) {
    case 'welcome':
      return (
        <div className="space-y-4 text-sm text-muted-foreground">
          <p>{t('wizard.welcome.body1', { defaultValue: 'Welcome to McHarbor — your self-hosted container management platform.' })}</p>
          <p>{t('wizard.welcome.body2', { defaultValue: 'This quick walkthrough will set up your first environment, install an app, and build a dashboard.' })}</p>
          <p className="rounded-md border border-border bg-muted/40 px-3 py-2 text-xs">
            {t('wizard.welcome.body3', { defaultValue: 'Takes about 5 minutes. You can skip any step and come back later.' })}
          </p>
          <Button
            className="w-full"
            onClick={() => {
              onComplete();
            }}
          >
            {t('actions.getStarted', { defaultValue: 'Get started' })}
          </Button>
        </div>
      );

    case 'environment':
      return (
        <div className="space-y-4 text-sm text-muted-foreground">
          <p>{t('wizard.environment.body1', { defaultValue: 'An environment is a Docker host or Kubernetes cluster McHarbor manages.' })}</p>
          <ul className="list-inside list-disc space-y-1">
            <li>{t('wizard.environment.local', { defaultValue: 'Local: use the host Docker socket (most common for homelabs).' })}</li>
            <li>{t('wizard.environment.agent', { defaultValue: 'Remote agent: install mcharbor-agent on a remote host (works behind NAT).' })}</li>
            <li>{t('wizard.environment.kubernetes', { defaultValue: 'Kubernetes: paste a kubeconfig or service account token.' })}</li>
          </ul>
          <Button
            variant="outline"
            className="w-full"
            onClick={() => {
              navigate('/environments');
              onComplete();
            }}
          >
            <IconServer className="mr-2 size-4" />
            {t('wizard.environment.cta', { defaultValue: 'Add your first environment' })}
          </Button>
        </div>
      );

    case 'app':
      return (
        <div className="space-y-4 text-sm text-muted-foreground">
          <p>{t('wizard.app.body1', { defaultValue: 'The App Store has 630+ one-click apps — homepages, dashboards, databases, and more.' })}</p>
          <p className="rounded-md border border-border bg-muted/40 px-3 py-2 text-xs">
            {t('wizard.app.recommended', { defaultValue: 'Recommended for first installs: Homepage (heimdall or homepage) — gives you a beautiful dashboard for all your apps.' })}
          </p>
          <Button
            variant="outline"
            className="w-full"
            onClick={() => {
              navigate('/appstore');
              onComplete();
            }}
          >
            <IconApps className="mr-2 size-4" />
            {t('wizard.app.cta', { defaultValue: 'Browse the App Store' })}
          </Button>
        </div>
      );

    case 'dashboard':
      return (
        <div className="space-y-4 text-sm text-muted-foreground">
          <p>{t('wizard.dashboard.body1', { defaultValue: 'The dashboard shows live container, host, and resource metrics. Add widgets by drag-and-drop.' })}</p>
          <ul className="list-inside list-disc space-y-1">
            <li>{t('wizard.dashboard.widget1', { defaultValue: 'Container List — see what is running and where.' })}</li>
            <li>{t('wizard.dashboard.widget2', { defaultValue: 'Host Info — CPU, RAM, disk, and Docker version.' })}</li>
            <li>{t('wizard.dashboard.widget3', { defaultValue: 'Resource Summary — at-a-glance totals across all environments.' })}</li>
          </ul>
          <Button
            variant="outline"
            className="w-full"
            onClick={() => {
              navigate('/dashboard');
              onComplete();
            }}
          >
            <IconLayoutDashboard className="mr-2 size-4" />
            {t('wizard.dashboard.cta', { defaultValue: 'Customize your dashboard' })}
          </Button>
        </div>
      );

    case 'notifications':
      return (
        <div className="space-y-4 text-sm text-muted-foreground">
          <p>{t('wizard.notifications.body1', { defaultValue: 'McHarbor can alert you when containers stop, disk fills up, or images have updates. Pick a channel:' })}</p>
          <ul className="list-inside list-disc space-y-1">
            <li>{t('wizard.notifications.slack', { defaultValue: 'Slack, Discord, Teams — webhook URLs' })}</li>
            <li>{t('wizard.notifications.email', { defaultValue: 'Email (SMTP)' })}</li>
            <li>{t('wizard.notifications.telegram', { defaultValue: 'Telegram, Gotify, Ntfy, Signal, WhatsApp' })}</li>
          </ul>
          <p className="text-xs text-muted-foreground">
            {t('wizard.notifications.optional', { defaultValue: 'You can configure channels later in Settings → Communications.' })}
          </p>
          <Button
            variant="outline"
            className="w-full"
            onClick={() => {
              navigate('/settings');
              onComplete();
            }}
          >
            <IconBell className="mr-2 size-4" />
            {t('wizard.notifications.cta', { defaultValue: 'Set up notifications (optional)' })}
          </Button>
        </div>
      );

    case 'finish':
      return (
        <div className="space-y-4 text-center text-sm text-muted-foreground">
          <div className="mx-auto flex size-14 items-center justify-center rounded-full bg-green-500/10 text-green-600">
            <IconCheck className="size-7" />
          </div>
          <p className="font-medium text-foreground">
            {t('wizard.finish.heading', { defaultValue: "You're all set!" })}
          </p>
          <p>
            {t('wizard.finish.body1', {
              defaultValue: 'McHarbor is ready to use. Press ? any time to see keyboard shortcuts, or click the help button in the top right of any page.',
            })}
          </p>
        </div>
      );

    default:
      return null;
  }
}

type FirstRunWizardProps = {
  forceOpen?: boolean;
  onClose?: () => void;
};

export function FirstRunWizard({ forceOpen, onClose }: FirstRunWizardProps) {
  const { t } = useTranslation();
  const { hasCompletedWizard, hasSkippedWizard, setStepCompleted, setHasCompletedWizard, setHasSkippedWizard } = useOnboardingStore();
  const [open, setOpen] = useState(false);
  const [stepIndex, setStepIndex] = useState(0);

  useEffect(() => {
    if (forceOpen) {
      setOpen(true);
      setStepIndex(0);
      return;
    }
    if (!hasCompletedWizard && !hasSkippedWizard) {
      setOpen(true);
    }
  }, [forceOpen, hasCompletedWizard, hasSkippedWizard]);

  const currentStep = WIZARD_STEPS[stepIndex];
  const isLastStep = stepIndex === WIZARD_STEPS.length - 1;
  const isFirstStep = stepIndex === 0;

  const handleNext = () => {
    if (currentStep) setStepCompleted(currentStep.id);
    if (isLastStep) {
      setHasCompletedWizard(true);
      setOpen(false);
      onClose?.();
      return;
    }
    setStepIndex((i) => i + 1);
  };

  const handleBack = () => setStepIndex((i) => Math.max(0, i - 1));

  const handleSkipAll = () => {
    setHasSkippedWizard(true);
    setOpen(false);
    onClose?.();
  };

  if (!currentStep) return null;

  const totalSteps = WIZARD_STEPS.length;
  const isFinish = currentStep.id === 'finish';

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) handleSkipAll();
        else setOpen(true);
      }}
    >
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <div className="flex items-start justify-between gap-4">
            <div>
              <DialogTitle>
                {t(currentStep.titleKey, { defaultValue: currentStep.id })}
              </DialogTitle>
              <DialogDescription>
                {t(currentStep.descriptionKey, { defaultValue: '' })}
              </DialogDescription>
            </div>
            <Button
              variant="ghost"
              size="icon"
              onClick={handleSkipAll}
              aria-label={t('wizard.skip', { defaultValue: 'Skip setup' })}
            >
              <IconX className="size-4" />
            </Button>
          </div>
        </DialogHeader>

        <div className="mb-2 flex items-center gap-1.5">
          {WIZARD_STEPS.map((step, i) => (
            <div
              key={step.id}
              className={cn(
                'h-1.5 flex-1 rounded-full transition-colors',
                i < stepIndex
                  ? 'bg-primary'
                  : i === stepIndex
                    ? 'bg-primary/60'
                    : 'bg-muted',
              )}
              aria-hidden
            />
          ))}
        </div>

        <div className="min-h-[200px]">
          <StepContent
            stepId={currentStep.id}
            onComplete={() => {
              handleNext();
            }}
          />
        </div>

        {!isFinish && (
          <DialogFooter>
            <span className="text-xs text-muted-foreground">
              {t('wizard.step', {
                defaultValue: `Step ${stepIndex + 1} of ${totalSteps}`,
                current: stepIndex + 1,
                total: totalSteps,
              })}
            </span>
            <div className="ml-auto flex gap-2">
              <Button variant="ghost" onClick={isFirstStep ? handleSkipAll : handleBack}>
                {isFirstStep ? (
                  t('wizard.skip', { defaultValue: 'Skip' })
                ) : (
                  <>
                    <IconChevronLeft className="mr-1 size-4" />
                    {t('actions.back', { defaultValue: 'Back' })}
                  </>
                )}
              </Button>
              <Button onClick={handleNext}>
                {isLastStep ? (
                  t('actions.finish', { defaultValue: 'Finish' })
                ) : (
                  <>
                    {t('actions.next', { defaultValue: 'Next' })}
                    <IconChevronRight className="ml-1 size-4" />
                  </>
                )}
              </Button>
            </div>
          </DialogFooter>
        )}

        {isFinish && (
          <DialogFooter>
            <Button onClick={handleNext} className="ml-auto">
              {t('actions.finish', { defaultValue: 'Finish' })}
              <IconCheck className="ml-1 size-4" />
            </Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}

type RestartWizardButtonProps = {
  className?: string;
};

export function RestartWizardButton({ className }: RestartWizardButtonProps) {
  const { t } = useTranslation();
  const { reset, setHasCompletedWizard, setHasSkippedWizard } = useOnboardingStore();
  const handleRestart = () => {
    reset();
    setHasCompletedWizard(false);
    setHasSkippedWizard(false);
  };
  return (
    <Button variant="ghost" onClick={handleRestart} className={className}>
      {t('wizard.restart', { defaultValue: 'Run setup again' })}
    </Button>
  );
}
