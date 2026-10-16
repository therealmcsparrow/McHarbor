// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
//
// comm-signal SetupGuide — ordered steps the user can follow for
// each of the four connection methods. Matches the host's setup
// guide slot pattern (read-only documentation; no form fields).

import { useTranslation } from 'react-i18next';
import { RichText } from './RichText';

export function SetupGuide() {
  const { t } = useTranslation('comm-signal');

  // Each step is a (title, body) pair. Numbering is implicit — the
  // rendering loop adds the leading number so the user sees
  // "1.", "2.", ... even if the locale reorders the steps.
  const steps: Array<[string, string]> = [
    [t('guide.step1.title'), t('guide.step1.body')],
    [t('guide.step2.title'), t('guide.step2.body')],
    [t('guide.step3.title'), t('guide.step3.body')],
    [t('guide.step4.title'), t('guide.step4.body')],
    [t('guide.step5.title'), t('guide.step5.body')],
    [t('guide.step6.title'), t('guide.step6.body')],
    [t('guide.step7.title'), t('guide.step7.body')],
    [t('guide.step8.title'), t('guide.step8.body')],
    [t('guide.step9.title'), t('guide.step9.body')],
    [t('guide.step10.title'), t('guide.step10.body')],
    [t('guide.step11.title'), t('guide.step11.body')],
    [t('guide.step12.title'), t('guide.step12.body')],
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
            <p className="mt-2 pl-9 text-sm whitespace-pre-line text-muted-foreground">
              <RichText text={body} />
            </p>
          </li>
        ))}
      </ol>
    </article>
  );
}
