// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
//
// identity-entra-id SetupGuide — ordered Azure App Registration
// steps. Mirrors the host's setup-guide slot (read-only docs; no
// form fields).

import { useTranslation } from 'react-i18next';

export function SetupGuide() {
  const { t } = useTranslation('identity-entra-id');

  // Localized steps. Numbering is implicit (added by the renderer)
  // so a locale can reorder without breaking visuals.
  const steps: Array<[string, string]> = [
    [t('guide.step1.title'), t('guide.step1.body')],
    [t('guide.step2.title'), t('guide.step2.body')],
    [t('guide.step3.title'), t('guide.step3.body')],
    [t('guide.step4.title'), t('guide.step4.body')],
    [t('guide.step5.title'), t('guide.step5.body')],
    [t('guide.step6.title'), t('guide.step6.body')],
    [t('guide.step7.title'), t('guide.step7.body')],
  ];

  return (
    <article className="space-y-6">
      <header className="space-y-2">
        <h3 className="text-lg font-semibold text-foreground">{t('guide.title')}</h3>
        <p className="text-sm text-muted-foreground">{t('guide.subtitle')}</p>
      </header>

      <ol className="space-y-4">
        {steps.map(([title, body], i) => (
          <li key={title} className="rounded-lg border border-border bg-card p-4">
            <div className="flex items-baseline gap-3">
              <span className="font-mono text-xs text-muted-foreground">{String(i + 1).padStart(2, '0')}</span>
              <h4 className="text-sm font-semibold text-foreground">{title}</h4>
            </div>
            <p className="mt-2 pl-9 text-sm whitespace-pre-line text-muted-foreground">{body}</p>
          </li>
        ))}
      </ol>
    </article>
  );
}
