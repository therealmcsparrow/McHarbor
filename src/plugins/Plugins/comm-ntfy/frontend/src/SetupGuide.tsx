// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// comm-ntfy — setup guide for the ntfy notification channel.
//
// The guide walks an operator through creating an ntfy topic,
// picking a server (cloud or self-hosted), issuing a token, and
// confirming subscribers can receive the channel before
// activating alert rules. The plugin handles Test (publishes a
// real "McHarbor test notification" message) and Send (publishes
// Title + Message as the body) — there's no OAuth flow because
// ntfy uses static tokens or HTTP Basic auth.

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
