// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// storage-nextcloud — setup guide for Nextcloud. Strings come from the
// host-supplied `t` (namespace = plugin slug).
//
// The plugin's setup guide uses the host's OAuth consent flow via
// the `app_password` route (no OAuth client required) AND offers
// the full OAuth 2.0 flow for operators who want delegated user
// identity. The simpler flow is app passwords — generate one in
// Nextcloud Personal settings → Security → App passwords, paste
// the username + app password into the form, save. That's it.

import { Stack } from '@resources/components/ui/Layout';
import { Heading } from '@resources/components/ui/Typography';
import { RichText } from './RichText';

export type GuideStep = string;

export const STEPS: GuideStep[] = [
  'setup.steps.s1',
  'setup.steps.s2',
  'setup.steps.s3',
  'setup.steps.s4',
  'setup.steps.s5',
  'setup.steps.s6',
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
