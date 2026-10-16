// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { usePluginCapabilities } from '@core/plugins/capabilities';
import { SOURCE_CONTAINER } from '../pages/backup-run-rows';

export type SourceTagged = { sourceKind: string; sourceSlug?: string; sourceLabel?: string };

/**
 * A plugin source is titled by its own `backup-row-actions` declaration. Log rows carry no
 * slug, so the declaring plugin is only checked when the row names one.
 */
export function useSourceLabel() {
  const { t } = useTranslation('containers');
  const declared = usePluginCapabilities('backup-row-actions');
  return (row: SourceTagged) => {
    if (row.sourceKind === SOURCE_CONTAINER) return t('backups.source.container');
    const match = declared.find(
      (c) => c.sourceKind === row.sourceKind && (!row.sourceSlug || c.plugin === row.sourceSlug),
    );
    if (match?.labelKey && match.labelNs) return t(match.labelKey, { ns: match.labelNs });
    return row.sourceLabel || row.sourceKind;
  };
}
