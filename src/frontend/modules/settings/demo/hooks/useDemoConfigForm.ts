// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import {
  DEMO_CONFIG_FIELDS,
  checkDemoConfig,
  draftOf,
  type DemoConfigDraft,
  type DemoConfigProblems,
} from '@resources/constants/demoConfig';
import type { DemoConfig, DemoConfigField, DemoSizes } from '@resources/types/demo';

export type DemoConfigForm = {
  draft: DemoConfigDraft;
  problems: DemoConfigProblems;
  config?: DemoConfig;
  isDefault: boolean;
  setField: (field: DemoConfigField, value: string) => void;
  restoreDefaults: () => void;
};

export function useDemoConfigForm(sizes: DemoSizes, onEdit?: () => void): DemoConfigForm {
  const [draft, setDraft] = useState<DemoConfigDraft>(() => draftOf(sizes.initial));
  const checked = useMemo(() => checkDemoConfig(draft, sizes.limits), [draft, sizes.limits]);
  const defaults = useMemo(() => draftOf(sizes.defaults), [sizes.defaults]);
  const isDefault = DEMO_CONFIG_FIELDS.every((field) => draft[field] === defaults[field]);

  return {
    draft,
    problems: checked.problems,
    config: checked.config,
    isDefault,
    setField: (field, value) => {
      setDraft((current) => ({ ...current, [field]: value }));
      onEdit?.();
    },
    restoreDefaults: () => {
      setDraft(defaults);
      onEdit?.();
    },
  };
}
