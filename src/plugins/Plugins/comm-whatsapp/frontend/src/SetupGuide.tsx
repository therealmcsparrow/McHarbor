// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// comm-whatsapp — setup guide for the WhatsApp notification channel.
//
// The guide walks an operator through picking a connection method
// and getting the credentials in place. Each method has its own
// prerequisites; we surface them as a single ordered list with
// method-specific sub-steps called out inline. The plugin handles
// Test (cheap probe per method) and Send (real message to the
// recipient) — there is no OAuth callback because WhatsApp
// providers use long-lived bearer tokens.

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
  'guide.step10.body',
  'guide.step11.body',
  'guide.step12.body',
  'guide.step13.body',
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
