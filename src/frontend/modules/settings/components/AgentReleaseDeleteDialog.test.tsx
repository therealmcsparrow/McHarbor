// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { AgentReleaseDeleteDialog } from './AgentReleaseDeleteDialog';
import { describeDestructiveDialog } from '../../../test/destructive';

describeDestructiveDialog({
  name: 'AgentReleaseDeleteDialog',
  method: 'delete',
  path: '*/api/updates/agents/releases/2026.09.01',
  permission: 'updates.manage',
  confirmLabel: 'Delete',
  render: ({ onOpenChange, onSuccess }) => (
    <AgentReleaseDeleteDialog release={{ version: '2026.09.01' }} onOpenChange={onOpenChange} onSuccess={onSuccess} />
  ),
});
