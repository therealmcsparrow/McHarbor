// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// SetupGuide — the operator-facing walkthrough for adding an
// SFTP storage destination. The host's storage location page
// mounts this component in the `setup_guide` slot alongside
// the per-field config form. We render a numbered list of
// steps that adapts to the auth_method currently selected so
// operators don't see instructions for keys they aren't using.
//
// UI primitives are inlined (native HTML + Tailwind) because
// the host's `@resources` alias does not resolve from a
// plugin's frontend build (see mcharbor-plugin-frontend-fix).

import * as React from 'react';
import { IconCircleCheck, IconKey, IconPlugConnected, IconShieldLock, IconUser } from '@tabler/icons-react';
import { useTranslation } from 'react-i18next';

type AuthMethod = 'password' | 'publickey' | 'keyboard_interactive' | 'agent';

type Step = {
  icon: React.ReactNode;
  titleKey: string;
  bodyKey: string;
};

// Step list — the first three are always visible; the auth-
// specific step swaps based on the selected auth_method.
function buildSteps(auth: AuthMethod): Step[] {
  const common: Step[] = [
    {
      icon: <IconShieldLock className="h-4 w-4" />,
      titleKey: 'guide.step.1.title',
      bodyKey: 'guide.step.1.body',
    },
    {
      icon: <IconCircleCheck className="h-4 w-4" />,
      titleKey: 'guide.step.2.title',
      bodyKey: 'guide.step.2.body',
    },
    {
      icon: <IconUser className="h-4 w-4" />,
      titleKey: 'guide.step.3.title',
      bodyKey: 'guide.step.3.body',
    },
  ];

  let authStep: Step;
  switch (auth) {
    case 'publickey':
      authStep = {
        icon: <IconKey className="h-4 w-4" />,
        titleKey: 'guide.step.auth.publickey.title',
        bodyKey: 'guide.step.auth.publickey.body',
      };
      break;
    case 'keyboard_interactive':
      authStep = {
        icon: <IconShieldLock className="h-4 w-4" />,
        titleKey: 'guide.step.auth.keyboard_interactive.title',
        bodyKey: 'guide.step.auth.keyboard_interactive.body',
      };
      break;
    case 'agent':
      authStep = {
        icon: <IconPlugConnected className="h-4 w-4" />,
        titleKey: 'guide.step.auth.agent.title',
        bodyKey: 'guide.step.auth.agent.body',
      };
      break;
    case 'password':
    default:
      authStep = {
        icon: <IconUser className="h-4 w-4" />,
        titleKey: 'guide.step.auth.password.title',
        bodyKey: 'guide.step.auth.password.body',
      };
  }

  const final: Step[] = [
    ...common,
    authStep,
    {
      icon: <IconCircleCheck className="h-4 w-4" />,
      titleKey: 'guide.step.5.title',
      bodyKey: 'guide.step.5.body',
    },
  ];
  return final;
}

export type SetupGuideProps = {
  config: Record<string, string>;
};

export function SetupGuide({ config }: SetupGuideProps): React.ReactElement {
  const { t } = useTranslation('storage-sftp');
  const auth = (config.auth_method as AuthMethod) || 'password';
  const steps = buildSteps(auth);

  return (
    <div className="space-y-6">
      <div>
        <p className="text-sm font-medium text-foreground">{t('guide.heading')}</p>
        <p className="text-sm text-muted-foreground">{t('guide.intro')}</p>
      </div>

      <ol className="space-y-4">
        {steps.map((step, idx) => (
          <li key={step.titleKey} className="flex items-start gap-3 rounded-lg border border-border bg-card p-4">
            <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary">
              {idx + 1}
            </div>
            <div className="flex-1 space-y-1">
              <div className="flex items-center gap-2 text-sm font-medium text-foreground">
                {step.icon}
                {t(step.titleKey)}
              </div>
              <p className="text-sm text-muted-foreground">{t(step.bodyKey)}</p>
            </div>
          </li>
        ))}
      </ol>

      <div className="rounded-lg border border-border bg-muted/30 p-4">
        <p className="text-sm text-muted-foreground">{t('guide.footer')}</p>
      </div>
    </div>
  );
}

export default SetupGuide;
