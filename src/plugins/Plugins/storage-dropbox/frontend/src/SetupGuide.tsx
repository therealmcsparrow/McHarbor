// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// storage-dropbox — setup guide for Dropbox.
//
// Each McHarbor installation uses its own Dropbox app; McSparrow
// does not host a shared app. The operator creates one app, picks
// the appropriate permission level (App folder for namespace
// isolation, or Full Dropbox for unrestricted access), generates
// a long-lived access token, and pastes it into the Access Token
// field on the left. Text comes from the host-supplied `t`
// (namespace = plugin slug).

import { Stack } from '@resources/components/ui/Layout';
import { Heading } from '@resources/components/ui/Typography';
import { RichText } from './RichText';

export type GuideStep = string;

export const STEPS: GuideStep[] = [
  'setup.step1',
  'setup.step2',
  'setup.step3',
  'setup.step4',
  'setup.step5',
  'setup.step6',
];

type Props = {
  t?: (key: string, vars?: Record<string, unknown>) => string;
};

export function SetupGuide({ t: hostT }: Props) {
  const t = (key: string, vars?: Record<string, unknown>) => hostT?.(key, vars) ?? key;

  return (
    <Stack gap={3}>
      <Heading level={2}>{t('setup.title')}</Heading>
      <ol className="ml-5 list-decimal space-y-2 text-sm text-muted-foreground marker:text-foreground">
        {STEPS.map((s) => (
          <li key={s}>
            <RichText text={t(s)} />
          </li>
        ))}
      </ol>
    </Stack>
  );
}
