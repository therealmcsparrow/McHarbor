// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// comm-teams — setup guide for the Microsoft Teams notification
// channel.
//
// The guide walks an operator through creating an Incoming
// Webhook connector in Teams, copying the connector URL, and
// confirming the channel receives a test card before activating
// alert rules. The plugin handles Test (posts a real "McHarbor
// test notification" Adaptive Card) and Send (posts Title +
// Message as the Adaptive Card body) — there is no OAuth flow
// because Incoming Webhook connectors are URL-only credentials.

import { Stack } from '@resources/components/ui/Layout';
import { Heading } from '@resources/components/ui/Typography';
import { RichText } from './RichText';

export const STEP_KEYS = [
  'guide.step1.body',
  'guide.step2.body',
  'guide.step3.body',
  'guide.step4.body',
  'guide.step5.body',
  'guide.step6.body',
  'guide.step7.body',
  'guide.step8.body',
  'guide.step9.body',
] as const;

type Props = { t?: (key: string, vars?: Record<string, unknown>) => string };

export function SetupGuide({ t: hostT }: Props) {
  const t = (key: string) => hostT?.(key) ?? key;
  return (
    <Stack gap={3}>
      <Heading level={2}>{t('setupGuide.title')}</Heading>
      <ol className="ml-5 list-decimal space-y-2 text-sm text-muted-foreground marker:text-foreground">
        {STEP_KEYS.map((key) => (
          <li key={key}>
            <RichText text={t(key)} />
          </li>
        ))}
      </ol>
    </Stack>
  );
}
