// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Stack } from '@resources/components/ui/Layout';
import { Heading } from '@resources/components/ui/Typography';
import { ScripSnippet } from './ScripSnippet';
import { SCRIP_COMMIT, SCRIP_PREPARE, SITE_CONFIG } from './scrip';
import type { Translate } from './useRtValidation';
import { RichText } from './RichText';

const STEPS = ['step1', 'step2', 'step3', 'step4', 'step5', 'step6'] as const;

export function SetupGuide({ t }: { t: Translate }) {
  return (
    <Stack gap={3}>
      <Heading level={2}>{t('guide.title')}</Heading>
      <ol className="ml-5 list-decimal space-y-2 text-sm text-muted-foreground marker:text-foreground">
        {STEPS.map((step) => (
          <li key={step}>{t(`guide.${step}`)}</li>
        ))}
      </ol>
      <ScripSnippet title={t('snippet.siteConfig')} code={SITE_CONFIG} t={t} />
      <ScripSnippet title={t('snippet.prepare')} code={SCRIP_PREPARE} t={t} />
      <ScripSnippet title={t('snippet.commit')} code={SCRIP_COMMIT} t={t} />
      <p className="text-sm text-muted-foreground">
        <RichText text={t('guide.signing')} />
      </p>
    </Stack>
  );
}
