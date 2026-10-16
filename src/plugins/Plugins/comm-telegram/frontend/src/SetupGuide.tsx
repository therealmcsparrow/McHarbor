// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// comm-telegram — setup guide for the Telegram notification channel.
//
// The guide walks an operator through creating a bot with
// @BotFather, finding the chat_id, and granting the bot permission
// to post in the target chat. The plugin handles Test (getMe liveness
// check) and Send (sendMessage) — there is no OAuth or callback
// flow because Telegram bots use a long-lived bearer token.

import { Stack } from '@resources/components/ui/Layout';
import { Heading } from '@resources/components/ui/Typography';
import { RichText } from './RichText';

export type GuideStep = string;

export const STEP_KEYS: GuideStep[] = [
  'guide.step1',
  'guide.step2',
  'guide.step3',
  'guide.step4',
  'guide.step5',
  'guide.step6',
  'guide.step7',
  'guide.step8',
];

const echoKey = (key: string) => key;

export function SetupGuide({ t: hostT }: { t?: (key: string) => string }) {
  const t = hostT ?? echoKey;
  return (
    <Stack gap={3}>
      <Heading level={2}>{t('guide.title')}</Heading>
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
