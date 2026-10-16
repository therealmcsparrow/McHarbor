// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// storage-sharepoint-online — setup guide for SharePoint Online.
// Strings are keys resolved by the host-supplied `t` (namespace = plugin slug).
//
// The host's OAuth consent flow handles the refresh-token round
// trip: the operator fills in tenant_id + client_id + client_secret,
// then clicks "Save and connect" to grant Files.ReadWrite.All,
// Sites.ReadWrite.All, User.Read, and offline_access. The returned
// refresh token is stored encrypted in the destination's cfg map
// and never shown again.
//
// OneDrive for Business setup lives in storage-onedrive-business.

import { Stack } from '@resources/components/ui/Layout';
import { Heading } from '@resources/components/ui/Typography';
import { RichText } from './RichText';

const GUIDE_TITLE_KEY = 'setup.title';

export type GuideStep = string;

type Props = {
  t?: (key: string, vars?: Record<string, unknown>) => string;
};

export const STEPS: GuideStep[] = [
  'setup.step1',
  'setup.step2',
  'setup.step3',
  'setup.step4',
  'setup.step5',
  'setup.step6',
  'setup.step7',
];

export function SetupGuide({ t: hostT }: Props) {
  const t = (key: string, vars?: Record<string, unknown>) => hostT?.(key, vars) ?? key;

  return (
    <Stack gap={3}>
      <Heading level={2}>{t(GUIDE_TITLE_KEY)}</Heading>
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
