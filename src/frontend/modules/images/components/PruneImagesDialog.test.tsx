// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { PruneImagesDialog } from './PruneImagesDialog';
import { describeDestructiveDialog } from '../../../test/destructive';

describeDestructiveDialog({
  name: 'PruneImagesDialog',
  method: 'post',
  path: '*/api/images/prune',
  permission: 'images.manage',
  confirmLabel: 'Prune Unused',
  render: ({ onOpenChange, onSuccess }) => <PruneImagesDialog open onOpenChange={onOpenChange} onSuccess={onSuccess} />,
});
