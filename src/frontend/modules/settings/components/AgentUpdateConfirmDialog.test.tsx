// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { AgentUpdateConfirmDialog } from './AgentUpdateConfirmDialog';
import { describeDestructiveDialog } from '../../../test/destructive';

describeDestructiveDialog({
  name: 'AgentUpdateConfirmDialog',
  method: 'post',
  path: '*/api/updates/agents/update',
  permission: 'environments.manage',
  confirmLabel: 'Update',
  body: { envIds: ['env-1'] },
  render: ({ onOpenChange, onSuccess }) => (
    <AgentUpdateConfirmDialog
      target={{ envIds: ['env-1'], name: 'edge-01' }}
      onOpenChange={onOpenChange}
      onSuccess={onSuccess}
    />
  ),
});
