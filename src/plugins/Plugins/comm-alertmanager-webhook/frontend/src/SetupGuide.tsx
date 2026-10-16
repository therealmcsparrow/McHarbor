// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Stack } from '@resources/components/ui/Layout';
import { Heading, Subheading } from '@resources/components/ui/Typography';
import type { Translate } from './options';
import { RichText } from './RichText';

const GENERAL = ['oneWay', 'profile', 'test'] as const;

const PROFILE_STEPS = [
  { profile: 'grafana_irm', steps: ['create', 'copy', 'templates'] },
  { profile: 'squadcast', steps: ['create', 'resolve'] },
  { profile: 'zenduty', steps: ['create', 'grouping'] },
  { profile: 'keep', steps: ['create', 'private'] },
] as const;

export function SetupGuide({ t }: { t: Translate }) {
  return (
    <Stack gap={4}>
      <Heading size="base" weight="semibold">
        {t('guide.title')}
      </Heading>
      <ul className="ml-5 list-disc space-y-2 text-sm text-muted-foreground marker:text-foreground">
        {GENERAL.map((key) => (
          <li key={key}>
            <RichText text={t(`guide.general.${key}`)} />
          </li>
        ))}
      </ul>
      {PROFILE_STEPS.map(({ profile, steps }) => (
        <section key={profile} className="space-y-2">
          <Subheading weight="semibold">{t(`profiles.${profile}.label`)}</Subheading>
          <ol className="ml-5 list-decimal space-y-2 text-sm text-muted-foreground marker:text-foreground">
            {steps.map((step) => (
              <li key={step}>
                <RichText text={t(`guide.${profile}.${step}`)} />
              </li>
            ))}
          </ol>
        </section>
      ))}
    </Stack>
  );
}
