// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { IconCheck, IconCopy } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Flex, Stack } from '@resources/components/ui/Layout';
import { Heading } from '@resources/components/ui/Typography';
import znunyYaml from '../../webservice/McHarborConnectorREST.yml?raw';
import otoboYaml from '../../webservice/McHarborConnectorREST-otobo.yml?raw';
import type { Translate } from './fields';
import { RichText } from './RichText';

const STEP_KEYS = ['step1', 'step2', 'step3', 'step4', 'step5', 'step6', 'step7', 'step8', 'step9'] as const;

type Variant = 'znuny' | 'otobo';

const VARIANTS: readonly Variant[] = ['znuny', 'otobo'];

const YAML: Record<Variant, string> = { znuny: znunyYaml, otobo: otoboYaml };

export function SetupGuide({ t }: { t: Translate }) {
  const [copied, setCopied] = useState<Variant | null>(null);

  const copy = async (variant: Variant) => {
    try {
      await navigator.clipboard.writeText(YAML[variant]);
      setCopied(variant);
    } catch {
      setCopied(null);
    }
  };

  const label = (variant: Variant) =>
    copied === variant ? t('setup.copied') : t(variant === 'znuny' ? 'setup.copyYaml' : 'setup.copyYamlOtobo');

  return (
    <Stack gap={3}>
      <Heading level={2}>{t('setup.title')}</Heading>
      <ol className="ml-5 list-decimal space-y-2 text-sm text-muted-foreground marker:text-foreground">
        {STEP_KEYS.map((k) => (
          <li key={k}>
            <RichText text={t(`setup.${k}`)} />
          </li>
        ))}
      </ol>
      <Flex gap={2} className="flex-wrap">
        {VARIANTS.map((variant) => (
          <Button key={variant} type="button" variant="outline" onClick={() => void copy(variant)}>
            {copied === variant ? (
              <IconCheck className="size-4" aria-hidden />
            ) : (
              <IconCopy className="size-4" aria-hidden />
            )}
            {label(variant)}
          </Button>
        ))}
      </Flex>
    </Stack>
  );
}
