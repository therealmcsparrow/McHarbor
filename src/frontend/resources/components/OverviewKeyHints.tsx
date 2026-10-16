// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';

const KEY_CLASS = 'rounded border border-border bg-muted px-1.5 py-0.5 font-mono text-[10px]';
const KEY_NAVIGATE = '↑↓';
const KEY_OPEN = '↵';
const KEY_CLOSE = 'esc';

export function OverviewKeyHints() {
  const { t } = useTranslation('common');
  return (
    <div className="flex items-center gap-4 border-t border-hairline px-4 py-2 text-xs text-muted-foreground">
      <span className="flex items-center gap-1.5">
        <kbd className={KEY_CLASS}>{KEY_NAVIGATE}</kbd>
        {t('containerOverview.navigate')}
      </span>
      <span className="flex items-center gap-1.5">
        <kbd className={KEY_CLASS}>{KEY_OPEN}</kbd>
        {t('containerOverview.open')}
      </span>
      <span className="flex items-center gap-1.5">
        <kbd className={KEY_CLASS}>{KEY_CLOSE}</kbd>
        {t('containerOverview.close')}
      </span>
    </div>
  );
}
