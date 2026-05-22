// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { ct } from '../canvas-theme';

export function CanvasEmptyState() {
  const { t } = useTranslation('common');

  return (
    <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
      <div className="text-center">
        <p className={`text-sm ${ct.text20}`}>{t('workflows.dragHint')}</p>
        <p className={`mt-1 text-xs ${ct.text10}`}>{t('workflows.dragHintSub')}</p>
      </div>
    </div>
  );
}

