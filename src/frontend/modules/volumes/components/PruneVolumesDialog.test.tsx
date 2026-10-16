// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { PruneVolumesDialog } from './PruneVolumesDialog';
import { describeDestructiveDialog } from '../../../test/destructive';

describeDestructiveDialog({
  name: 'PruneVolumesDialog',
  method: 'post',
  path: '*/api/volumes/prune',
  permission: 'volumes.manage',
  confirmLabel: 'Prune Unused',
  render: ({ onOpenChange, onSuccess }) => (
    <PruneVolumesDialog open onOpenChange={onOpenChange} onSuccess={onSuccess} />
  ),
});
