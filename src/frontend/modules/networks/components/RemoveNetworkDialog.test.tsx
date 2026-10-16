// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { RemoveNetworkDialog } from './RemoveNetworkDialog';
import { describeDestructiveDialog } from '../../../test/destructive';

describeDestructiveDialog({
  name: 'RemoveNetworkDialog',
  method: 'delete',
  path: '*/api/networks/net1',
  permission: 'networks.delete',
  confirmLabel: 'Remove',
  render: ({ onOpenChange, onSuccess }) => (
    <RemoveNetworkDialog networkId="net1" onOpenChange={onOpenChange} onSuccess={onSuccess} />
  ),
});
