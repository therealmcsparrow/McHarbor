// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconInfoCircle } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import type { ChangeClassification } from '../types/edit-form';

type SaveBarProps = {
  changes: ChangeClassification;
  onSave: () => void;
  onCancel: () => void;
  isSaving: boolean;
  isComposeManaged: boolean;
};

export function SaveBar({ changes, onSave, onCancel, isSaving, isComposeManaged }: SaveBarProps) {
  const { t } = useTranslation('containers');
  const hasChanges = changes.hasResourceChanges || changes.hasConfigChanges;

  return (
    <div className="sticky bottom-0 z-10 flex items-center justify-between border-t border-border bg-card px-5 py-3">
      <div className="flex items-center gap-3">
        {changes.hasConfigChanges && (
          <Badge variant="warning" className="gap-1">
            <IconAlertTriangle className="size-3" />
            {t('edit.recreateWarning')}
          </Badge>
        )}
        {changes.hasResourceChanges && !changes.hasConfigChanges && (
          <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <IconInfoCircle className="size-3.5" />
            {t('edit.liveUpdateInfo')}
          </div>
        )}
        {isComposeManaged && (
          <div className="flex items-center gap-1.5 text-xs text-yellow-500">
            <IconAlertTriangle className="size-3.5" />
            {t('edit.composeWarning')}
          </div>
        )}
      </div>
      <div className="flex items-center gap-2">
        <Button variant="outline" size="sm" onClick={onCancel} disabled={isSaving}>
          {t('edit.cancelChanges')}
        </Button>
        <Button
          size="sm"
          onClick={onSave}
          disabled={!hasChanges || isSaving}
        >
          {isSaving ? t('edit.saving') : t('edit.saveChanges')}
        </Button>
      </div>
    </div>
  );
}
