// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Stack } from '@resources/components/ui/Layout';
import { Heading } from '@resources/components/ui/Typography';
import type { Translate } from './useZammadValidation';
import { RichText } from './RichText';

const STEPS = ['step1', 'step2', 'step3', 'step4', 'step5', 'step6', 'step7'] as const;

export function SetupGuide({ t }: { t: Translate }) {
  return (
    <Stack gap={3}>
      <Heading as="h3" size="base" weight="semibold">
        {t('guide.title')}
      </Heading>
      <ol className="ml-5 list-decimal space-y-2 text-sm text-muted-foreground marker:text-foreground">
        {STEPS.map((step) => (
          <li key={step}>
            <RichText text={t(`guide.${step}`)} />
          </li>
        ))}
      </ol>
    </Stack>
  );
}
