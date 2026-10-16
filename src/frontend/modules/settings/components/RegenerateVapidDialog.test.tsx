// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { RegenerateVapidDialog } from './RegenerateVapidDialog';
import { describeDestructiveDialog } from '../../../test/destructive';

describeDestructiveDialog({
  name: 'RegenerateVapidDialog',
  method: 'post',
  path: '*/api/push/vapid/regenerate',
  permission: 'settings.manage',
  confirmLabel: 'Regenerate key',
  render: ({ onOpenChange, onSuccess }) => (
    <RegenerateVapidDialog open onOpenChange={onOpenChange} onSuccess={onSuccess} />
  ),
});
