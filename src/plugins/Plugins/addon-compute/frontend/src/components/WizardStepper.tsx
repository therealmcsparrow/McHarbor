// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCheck } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';

interface WizardStepperProps {
  steps: readonly string[];
  step: string;
  index: number;
}

export function WizardStepper({ steps, step, index }: WizardStepperProps) {
  const { t } = useTranslation('compute');

  return (
    <ol className="flex flex-wrap gap-2" aria-label={t('wizard.title')}>
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
          {t(`wizard.steps.${entry}`)}
        </li>
      ))}
    </ol>
  );
}
