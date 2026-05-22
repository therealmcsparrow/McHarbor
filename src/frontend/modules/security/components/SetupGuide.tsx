// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconChevronDown } from '@tabler/icons-react';

type Step = {
  titleKey: string;
  descriptionKey: string;
};

type SetupGuideProps = {
  titleKey: string;
  steps: Step[];
  callbackUrl: string;
};

const STEP_NUMBERS = ['①', '②', '③', '④', '⑤'];

export function SetupGuide({ titleKey, steps, callbackUrl }: SetupGuideProps) {
  const { t } = useTranslation('security');

  return (
    <details className="group rounded-lg border border-border bg-muted/20">
      <summary className="flex cursor-pointer items-center gap-2 px-4 py-3 text-sm font-medium text-foreground select-none list-none">
        <IconChevronDown className="size-4 text-muted-foreground transition-transform group-open:rotate-180" />
        {t(titleKey)}
      </summary>
      <div className="space-y-3 border-t border-border px-4 py-3">
        {steps.map((step, i) => (
          <div key={step.titleKey} className="flex gap-3">
            <span className="mt-0.5 text-base leading-none text-primary">
              {STEP_NUMBERS[i]}
            </span>
            <div>
              <p className="text-sm font-medium text-foreground">
                {t(step.titleKey)}
              </p>
              <p className="mt-0.5 text-xs text-muted-foreground">
                {t(step.descriptionKey)}
              </p>
            </div>
          </div>
        ))}
        <div className="rounded-md border border-dashed border-border bg-muted/30 px-3 py-2">
          <p className="text-xs text-muted-foreground">
            {t('identity.callbackUrl')}
          </p>
          <code className="mt-1 block text-xs text-foreground select-all">
            {callbackUrl}
          </code>
        </div>
      </div>
    </details>
  );
}
