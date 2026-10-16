// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { RemoveVolumeDialog } from './RemoveVolumeDialog';
import { describeDestructiveDialog } from '../../../test/destructive';

describeDestructiveDialog({
  name: 'RemoveVolumeDialog',
  method: 'delete',
  path: '*/api/volumes/pgdata',
  permission: 'volumes.delete',
  confirmLabel: 'Remove',
  render: ({ onOpenChange, onSuccess }) => (
    <RemoveVolumeDialog volumeName="pgdata" onOpenChange={onOpenChange} onSuccess={onSuccess} />
  ),
});
