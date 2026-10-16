// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { RemoveDownloadedWidgetDialog } from './RemoveDownloadedWidgetDialog';
import { describeDestructiveDialog } from '../../../test/destructive';

describeDestructiveDialog({
  name: 'RemoveDownloadedWidgetDialog',
  method: 'delete',
  path: '*/api/widgets/clock',
  permission: 'dashboard_widgets.install',
  confirmLabel: 'Remove',
  render: ({ onOpenChange, onSuccess }) => (
    <RemoveDownloadedWidgetDialog
      widgetKey="clock"
      widgetName="Clock"
      open
      onOpenChange={onOpenChange}
      onSuccess={onSuccess}
    />
  ),
});
