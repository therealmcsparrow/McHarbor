// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// SetupGuide — operator-facing walk-through for wiring the Box
// plugin to a Box account. Renders when the host shows the
// "Setup guide" tab on a storage destination.
//
// Strings come from the host-supplied `t` (namespace = plugin slug); the
// bundle has no i18next instance of its own, so useTranslation is unavailable.

export interface SetupGuideProps {
  value?: Record<string, string>;
  t?: (key: string, vars?: Record<string, unknown>) => string;
}

interface Step {
  titleKey: string;
  bodyKey: string;
  href?: string;
}

const STEPS: Step[] = [
  {
    titleKey: 'setup.step1.title',
    bodyKey: 'setup.step1.body',
    href: 'https://developer.box.com/console',
  },
  {
    titleKey: 'setup.step2.title',
    bodyKey: 'setup.step2.body',
  },
  {
    titleKey: 'setup.step3.title',
    bodyKey: 'setup.step3.body',
  },
  {
    titleKey: 'setup.step4.title',
    bodyKey: 'setup.step4.body',
  },
  {
    titleKey: 'setup.step5.title',
    bodyKey: 'setup.step5.body',
  },
  {
    titleKey: 'setup.step6.title',
    bodyKey: 'setup.step6.body',
  },
];

export function SetupGuide({ value: _value, t: hostT }: SetupGuideProps) {
  void _value;
  const t = (key: string, vars?: Record<string, unknown>) => hostT?.(key, vars) ?? key;

  return (
    <div className="space-y-6">
      <header className="space-y-1">
        <h2 className="text-lg font-semibold text-foreground">{t('setup.title')}</h2>
        <p className="text-sm text-muted-foreground">{t('setup.subtitle')}</p>
      </header>

      <ol className="space-y-4">
        {STEPS.map((step, i) => (
          <li key={step.titleKey} className="space-y-1.5 rounded-lg border border-border bg-card p-4">
            <div className="flex items-center gap-3">
              <span className="flex h-6 w-6 items-center justify-center rounded-full border border-border bg-muted font-mono text-xs text-muted-foreground">
                {i + 1}
              </span>
              <h3 className="text-sm font-medium text-foreground">{t(step.titleKey)}</h3>
            </div>
            <p className="pl-9 text-sm text-muted-foreground">{t(step.bodyKey)}</p>
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

export default SetupGuide;
