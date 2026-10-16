// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { RemoveImageDialog } from './RemoveImageDialog';
import { describeDestructiveDialog } from '../../../test/destructive';

describeDestructiveDialog({
  name: 'RemoveImageDialog',
  method: 'delete',
  path: '*/api/images/sha256%3Aabc',
  permission: 'images.delete',
  confirmLabel: 'Remove',
  render: ({ onOpenChange, onSuccess }) => (
    <RemoveImageDialog imageId="sha256:abc" onOpenChange={onOpenChange} onSuccess={onSuccess} />
  ),
});
