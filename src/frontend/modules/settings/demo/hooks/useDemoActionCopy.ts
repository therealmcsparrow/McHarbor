// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { DemoAction, DemoSizesMode } from '@resources/types/demo';

export type DemoActionCopy = {
  label: string;
  shortLabel: string;
  title: string;
  description: string;
};

export function useDemoActionCopy(action: DemoAction, active: boolean): DemoActionCopy {
  const { t } = useTranslation('demo');
  switch (action) {
    case 'enter':
      return {
        label: t('actions.enter'),
        shortLabel: t('actions.enter'),
        title: t('confirm.enterTitle'),
        description: t('confirm.enterDescription'),
      };
    case 'exit':
      return {
        label: t('actions.exit'),
        shortLabel: t('actions.exit'),
        title: t('confirm.exitTitle'),
        description: t('confirm.exitDescription'),
      };
    case 'reset':
      return {
        label: t('actions.reset'),
        shortLabel: t('actions.resetShort'),
        title: t('confirm.resetTitle'),
        description: active ? t('confirm.resetActiveDescription') : t('confirm.resetDescription'),
      };
  }
}

export function useDemoSizesCopy(mode: DemoSizesMode): Omit<DemoActionCopy, 'shortLabel'> {
  const { t } = useTranslation('demo');
  if (mode === 'enter') {
    return {
      label: t('actions.enter'),
      title: t('confirm.enterTitle'),
      description: t('confirm.enterDescription'),
    };
  }
  return {
    label: t('actions.applySizes'),
    title: t('confirm.applySizesTitle'),
    description: t('confirm.applySizesDescription'),
  };
}
