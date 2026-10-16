// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { RotateTokenDialog } from './RotateTokenDialog';
import { describeDestructiveDialog } from '../../../test/destructive';

describeDestructiveDialog({
  name: 'RotateTokenDialog',
  method: 'post',
  path: '*/api/scim/endpoints/e1/rotate-token',
  permission: 'scim.manage',
  confirmLabel: 'Rotate token',
  render: ({ onOpenChange, onSuccess }) => (
    <RotateTokenDialog endpoint={{ id: 'e1', name: 'Corp IdP' }} onOpenChange={onOpenChange} onSuccess={onSuccess} />
  ),
});
