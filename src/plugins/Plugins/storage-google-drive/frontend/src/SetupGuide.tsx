// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// storage-google-drive — setup guide for Google Drive.
//
// Strings come from the host-supplied `t` (namespace = plugin slug).
//
// The host's OAuth consent flow handles the refresh-token round
// trip: the operator fills in client_id + client_secret, then
// clicks "Save and connect" to grant the drive.file scope. The
// returned refresh token is stored encrypted in the
// destination's cfg map and never shown again.

import { Stack } from '@resources/components/ui/Layout';
import { Heading } from '@resources/components/ui/Typography';
import { RichText } from './RichText';

export const STEP_KEYS: string[] = [
  'setup.steps.enableApi',
  'setup.steps.createClient',
  'setup.steps.copyCredentials',
  'setup.steps.folderId',
  'setup.steps.connect',
  'setup.steps.test',
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
        {STEP_KEYS.map((k) => (
          <li key={k}>
            <RichText text={t(k)} />
          </li>
        ))}
      </ol>
    </Stack>
  );
}
