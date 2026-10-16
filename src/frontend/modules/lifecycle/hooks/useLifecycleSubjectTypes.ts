// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { usePluginCapabilities } from '@core/plugins/capabilities';
import { SUBJECT_TYPES } from '../components/lifecycle-filter-options';

export type LifecycleSubjectOption = { value: string; label: string };

/**
 * The subject filter's options: the host's own subjects, then every type a loaded
 * plugin declares as a `lifecycle-subject`, so a plugin's subjects are offered only
 * while the plugin that writes them is installed.
 */
export function useLifecycleSubjectTypes(): LifecycleSubjectOption[] {
  const { t } = useTranslation('settings');
  const caps = usePluginCapabilities('lifecycle-subject');
  return useMemo(() => {
    const options: LifecycleSubjectOption[] = SUBJECT_TYPES.map((o) => ({ value: o.value, label: t(o.labelKey) }));
    const seen = new Set(options.map((o) => o.value));
    for (const c of caps) {
      const type = c.subjectType;
      if (!type || seen.has(type)) continue;
      seen.add(type);
      // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: plugins declare their own subject types, so one the host has no label for shows as the type itself
      options.push({ value: type, label: t(`lifecycle.subjectType.${type}`, { defaultValue: type }) });
    }
    return options;
  }, [caps, t]);
}
