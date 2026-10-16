// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { DeleteEndpointDialog } from './DeleteEndpointDialog';
import { describeDestructiveDialog } from '../../../test/destructive';

describeDestructiveDialog({
  name: 'DeleteEndpointDialog',
  method: 'delete',
  path: '*/api/scim/endpoints/e1',
  permission: 'scim.manage',
  confirmLabel: 'Delete endpoint',
  render: ({ onOpenChange, onSuccess }) => (
    <DeleteEndpointDialog endpoint={{ id: 'e1', name: 'Corp IdP' }} onOpenChange={onOpenChange} onSuccess={onSuccess} />
  ),
});
