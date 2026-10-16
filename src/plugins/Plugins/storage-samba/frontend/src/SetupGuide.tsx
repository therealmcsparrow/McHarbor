// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// SetupGuide — operator-facing walk-through for wiring the Samba
// plugin to a server share. Renders when the host shows the
// "Setup guide" tab on a storage destination. Mirrors the same
// shape as the ConfigForm: copy + numbered list + external link.
//
// The three auth methods get their own track: the steps walk
// through creating a Samba share, then branch into
// guest / basic / domain configuration depending on which
// method the operator picked.

import React from 'react';
import { useTranslation } from 'react-i18next';
import type { PluginMountCtx } from './index';

interface SetupGuideProps {
  ctx: PluginMountCtx;
}

interface Step {
  title: string;
  body: string;
  href?: string;
}

export function SetupGuide({ ctx }: SetupGuideProps) {
  const { t } = useTranslation('storage-samba');
  const authMethod = ctx.cfg['auth_method'] || 'basic';
  void ctx;

  // Common steps: every auth_method needs a working share first.
  const common: Step[] = [
    {
      title: t('setup.common.step1.title'),
      body: t('setup.common.step1.body'),
    },
    {
      title: t('setup.common.step2.title'),
      body: t('setup.common.step2.body'),
    },
    {
      title: t('setup.common.step3.title'),
      body: t('setup.common.step3.body'),
    },
  ];

  // Auth-method-specific tail: one branch per supported method.
  const tail: Step[] = (() => {
    switch (authMethod) {
      case 'guest':
        return [
          {
            title: t('setup.guest.step1.title'),
            body: t('setup.guest.step1.body'),
          },
          {
            title: t('setup.guest.step2.title'),
            body: t('setup.guest.step2.body'),
          },
        ];
      case 'domain':
        return [
          {
            title: t('setup.domain.step1.title'),
            body: t('setup.domain.step1.body'),
          },
          {
            title: t('setup.domain.step2.title'),
            body: t('setup.domain.step2.body'),
          },
        ];
      case 'basic':
      default:
        return [
          {
            title: t('setup.basic.step1.title'),
            body: t('setup.basic.step1.body'),
          },
          {
            title: t('setup.basic.step2.title'),
            body: t('setup.basic.step2.body'),
          },
        ];
    }
  })();

  const steps = [...common, ...tail];

  return (
    <div className="space-y-6">
      <header className="space-y-1">
        <h2 className="text-lg font-semibold text-foreground">{t('setup.title')}</h2>
        <p className="text-sm text-muted-foreground">{t('setup.subtitle')}</p>
      </header>

      <ol className="space-y-4">
        {steps.map((step, i) => (
          <li key={step.title} className="space-y-1.5 rounded-lg border border-border bg-card p-4">
            <div className="flex items-center gap-3">
              <span className="flex h-6 w-6 items-center justify-center rounded-full border border-border bg-muted font-mono text-xs text-muted-foreground">
                {i + 1}
              </span>
              <h3 className="text-sm font-medium text-foreground">{step.title}</h3>
            </div>
            <p className="pl-9 text-sm text-muted-foreground">{step.body}</p>
            {step.href && (
              <p className="pl-9">
                <a
                  href={step.href}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-xs text-primary underline-offset-4 hover:underline"
                >
                  {step.href}
                </a>
              </p>
            )}
          </li>
        ))}
      </ol>

      <div className="rounded-lg border border-border bg-card p-4 text-xs text-muted-foreground">
        {t('setup.footer')}
      </div>
    </div>
  );
}
