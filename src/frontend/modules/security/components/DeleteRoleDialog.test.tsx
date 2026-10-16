// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { DeleteRoleDialog } from './DeleteRoleDialog';
import { describeDestructiveDialog } from '../../../test/destructive';

describeDestructiveDialog({
  name: 'DeleteRoleDialog',
  method: 'delete',
  path: '*/api/roles/r1',
  permission: 'roles.manage',
  confirmLabel: 'Remove',
  render: ({ onOpenChange, onSuccess }) => (
    <DeleteRoleDialog roleId="r1" onOpenChange={onOpenChange} onSuccess={onSuccess} />
  ),
});
