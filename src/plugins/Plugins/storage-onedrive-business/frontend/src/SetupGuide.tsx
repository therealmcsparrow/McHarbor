// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// storage-onedrive-business — setup guide for OneDrive for Business.
//
// The host's OAuth consent flow handles the refresh-token round
// trip: the operator fills in tenant_id + client_id + client_secret,
// then clicks "Save and connect" to grant Files.ReadWrite.All,
// User.Read, and offline_access. The returned refresh token is
// stored encrypted in the destination's cfg map and never shown
// again.
//
// Strings come from the host-supplied t (namespace = plugin slug).
//
// SharePoint setup lives in storage-sharepoint-online.

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
  'setup.step7',
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
