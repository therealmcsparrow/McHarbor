// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The "Reduce motion" preference.
//
// `prefers-reduced-motion` is the right default and the wrong ceiling.
// McHarbor is an app people leave open on a second monitor all day, so a
// status dot pulsing in peripheral vision is a different proposition here
// than the same animation on a page someone reads once — and someone who
// set the OS flag for a different app entirely may still want the theme
// reveal here.
//
// Hence three states rather than a switch: follow the system, force full
// motion, or force reduced.

import { useTranslation } from 'react-i18next';
import { IconWaveSine, IconWaveSquare, IconDeviceDesktop } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Flex } from '@resources/components/ui/Layout';
import {
  useMotionStore,
  resolveReduced,
  type MotionPreference as MotionPreferenceValue,
} from '@resources/stores/motion';

const OPTIONS: {
  value: MotionPreferenceValue;
  icon: typeof IconWaveSine;
  labelKey: string;
}[] = [
  { value: 'system', icon: IconDeviceDesktop, labelKey: 'motion.system' },
  { value: 'full', icon: IconWaveSine, labelKey: 'motion.full' },
  { value: 'reduced', icon: IconWaveSquare, labelKey: 'motion.reduced' },
];

export function MotionPreference() {
  const { t } = useTranslation('settings');
  const preference = useMotionStore((s) => s.preference);
  const setPreference = useMotionStore((s) => s.setPreference);

  const effective = resolveReduced(preference) ? t('motion.effectiveReduced') : t('motion.effectiveFull');

  return (
    <div className="flex flex-col gap-2">
      <Flex gap={2} wrap>
        {OPTIONS.map((option) => {
          const Icon = option.icon;
          const active = preference === option.value;
          return (
            <Button
              key={option.value}
              type="button"
              variant={active ? 'default' : 'outline'}
              size="sm"
              aria-pressed={active}
              onClick={() => setPreference(option.value)}
              className="flex items-center gap-2"
            >
              <Icon className="size-4" />
              {t(option.labelKey)}
            </Button>
          );
        })}
      </Flex>
      <p className="text-xs text-muted-foreground">
        {t('motion.hint')} — {effective}
      </p>
    </div>
  );
}
