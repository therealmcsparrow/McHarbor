// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Stack } from '@resources/components/ui/Layout';
import { Heading } from '@resources/components/ui/Typography';
import type { Translate } from './ConfigForm';
import { RichText } from './RichText';

const STEP_KEYS = [
  'service',
  'integration',
  'routingKey',
  'region',
  'restKey',
  'inbound',
  'webhook',
  'save',
  'testEvent',
] as const;

export function SetupGuide({ t }: { t: Translate }) {
  return (
    <Stack gap={3}>
      <Heading level={2}>{t('guide.title')}</Heading>
      <ol className="ml-5 list-decimal space-y-2 text-sm text-muted-foreground marker:text-foreground">
        {STEP_KEYS.map((key) => (
          <li key={key}>
            <RichText text={t(`guide.steps.${key}`)} />
          </li>
        ))}
      </ol>
      <p className="text-xs text-muted-foreground">{t('guide.note')}</p>
    </Stack>
  );
}
