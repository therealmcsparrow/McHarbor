// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Stack } from '@resources/components/ui/Layout';
import { Heading } from '@resources/components/ui/Typography';
import { RichText } from './RichText';

export const STEPS = [
  'setup.step1',
  'setup.step2',
  'setup.step3',
  'setup.step4',
  'setup.step5',
  'setup.step6',
] as const;

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
