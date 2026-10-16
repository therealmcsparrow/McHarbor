// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Stack } from '@resources/components/ui/Layout';
import { Heading, Text as TextBlock } from '@resources/components/ui/Typography';
import type { Translate } from './fields';
import { RichText } from './RichText';

const STEPS = ['step1', 'step2', 'step3', 'step4', 'step5', 'step6', 'step7'] as const;

export const WEBHOOK_PAYLOAD =
  '{"id":"$this->id$","ref":"$this->ref$","class":"$this->finalclass$","title":"$this->title$","status":"$this->status$","agent":"$this->agent_id_friendlyname$","last_update":"$this->last_update$"}';

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
      <TextBlock weight="medium">{t('guide.payloadTitle')}</TextBlock>
      <pre className="overflow-x-auto rounded-md bg-muted p-3 font-mono text-xs break-all whitespace-pre-wrap text-foreground">
        {WEBHOOK_PAYLOAD}
      </pre>
    </Stack>
  );
}
