// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { RunbookCatalogEntry } from '@resources/types/runbookCatalog';

/**
 * A plugin entry carries a key into its own plugin's i18n namespace, because the plugin
 * owns the wording; a host entry still carries its English text.
 */
export function useEntryDescription(): (entry: RunbookCatalogEntry) => string {
  const { t } = useTranslation('runbooks');
  return (entry) => (entry.descriptionKey ? t(entry.descriptionKey, { ns: entry.descriptionNs }) : entry.description);
}
