// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { IconChevronRight } from '@tabler/icons-react';
import { PlainButton } from '@resources/components/ui/PlainButton';
import { useHeaderSlot } from '@resources/hooks/useHeaderSlot';

export function VMBreadcrumb({ rootLabel, leaf, onRoot }: { rootLabel: string; leaf: string; onRoot: () => void }) {
  const { t } = useTranslation('common');
  const slot = useHeaderSlot();
  if (!slot) return null;

  return createPortal(
    <nav aria-label={t('breadcrumb.label')} className="flex min-w-0 flex-1 items-center gap-1 text-sm">
      <PlainButton
        type="button"
        onClick={onRoot}
        className="shrink-0 text-muted-foreground transition-colors hover:text-foreground"
      >
        {rootLabel}
      </PlainButton>
      <IconChevronRight className="size-3.5 shrink-0 text-muted-foreground/50" aria-hidden="true" />
      <span className="truncate font-semibold text-foreground">{leaf}</span>
    </nav>,
    slot,
  );
}
