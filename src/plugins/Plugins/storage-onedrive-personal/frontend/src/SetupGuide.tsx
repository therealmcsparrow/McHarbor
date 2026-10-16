// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// storage-onedrive-personal — setup guide for OneDrive Personal.
//
// The plugin ships an embedded McSparrow-owned Azure AD app
// registration, so the operator only needs to:
//   1. Save the destination (no fields required)
//   2. Click "Sign in with Microsoft"
//   3. Sign in with their live.com / hotmail.com / outlook.com account
//   4. Grant the requested Files.ReadWrite + User.Read + offline_access scopes
// The host's OAuth consent flow then stores the refresh token
// encrypted in the destination's cfg map.
//
// Operators who want their own app registration can fill in
// client_id (Azure AD app, Mobile and desktop applications
// public client under "Personal Microsoft accounts only") and
// optionally client_secret. See the plugin's README for details.

import { Stack } from '@resources/components/ui/Layout';
import { Heading } from '@resources/components/ui/Typography';
import { RichText } from './RichText';

type Props = {
  t?: (key: string, vars?: Record<string, unknown>) => string;
};

export type GuideStep = string;

export const STEPS: GuideStep[] = ['setup.steps.save', 'setup.steps.signIn', 'setup.steps.grant', 'setup.steps.test'];

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
