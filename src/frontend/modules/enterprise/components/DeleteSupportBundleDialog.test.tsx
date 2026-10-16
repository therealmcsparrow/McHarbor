// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { DeleteSupportBundleDialog } from './DeleteSupportBundleDialog';
import { describeDestructiveDialog } from '../../../test/destructive';

describeDestructiveDialog({
  name: 'DeleteSupportBundleDialog',
  method: 'delete',
  path: '*/api/support/bundles/abcdefghij0123456789',
  permission: 'support.manage',
  confirmLabel: 'Delete',
  render: ({ onOpenChange, onSuccess }) => (
    <DeleteSupportBundleDialog bundleId="abcdefghij0123456789" onOpenChange={onOpenChange} onSuccess={onSuccess} />
  ),
});
