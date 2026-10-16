// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import type { BatchAction } from '@resources/layout/table-page/types';

type DataGridBatchActionsProps = {
  pendingAction: BatchAction | null;
  selectedCount: number;
  onCancel: () => void;
  onConfirm: () => void;
};

// DataGridBatchActions renders the destructive-action confirmation dialog shown
// when a user clicks a batch action marked `confirm: true`. Lives outside the
// main table component so the orchestrator can stay focused on layout.
//
// Open state is derived from `pendingAction` rather than held locally: a local
// `useState(true)` survived the first dismissal and left the dialog permanently
// shut for every subsequent action.
export function DataGridBatchActions({ pendingAction, selectedCount, onCancel, onConfirm }: DataGridBatchActionsProps) {
  const { t } = useTranslation('common');

  if (!pendingAction) return null;

  return (
    <ConfirmDialog
      open
      onOpenChange={(next) => {
        if (!next) onCancel();
      }}
      title={t('batch.confirmTitle')}
      description={t('batch.confirmDescription', {
        action: pendingAction.label.toLowerCase(),
        count: selectedCount,
      })}
      onConfirm={onConfirm}
      confirmLabel={pendingAction.label}
      variant={pendingAction.variant === 'destructive' ? 'destructive' : 'caution'}
    />
  );
}
