// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { PERM } from '@core/auth/permissionKeys';
import { useRemoveDownloadedWidget } from '@resources/hooks/useWidgetCatalog';

type RemoveDownloadedWidgetDialogProps = {
  widgetKey: string;
  widgetName: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
};

export function RemoveDownloadedWidgetDialog({
  widgetKey,
  widgetName,
  open: isOpen,
  onOpenChange,
  onSuccess,
}: RemoveDownloadedWidgetDialogProps) {
  const { t } = useTranslation('common');
  const remove = useRemoveDownloadedWidget();

  return (
    <MutationConfirmDialog
      open={isOpen}
      onOpenChange={onOpenChange}
      title={t('store.widgetRemoveTitle', { name: widgetName })}
      description={t('store.widgetRemoveDescription', { name: widgetName })}
      confirmLabel={t('actions.remove')}
      permission={PERM.DASHBOARD_WIDGETS_INSTALL}
      mutation={remove}
      variables={widgetKey}
      onSuccess={onSuccess}
    />
  );
}
