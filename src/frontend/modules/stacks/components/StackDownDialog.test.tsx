// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { StackDownDialog } from './StackDownDialog';
import type { StackInfo } from '@resources/hooks/useStacks';
import { describeDestructiveDialog } from '../../../test/destructive';

const stack: StackInfo = {
  id: 'st1',
  name: 'web',
  status: 'running',
  services: [],
  type: 'managed',
  targetsSelf: false,
};

describeDestructiveDialog({
  name: 'StackDownDialog',
  method: 'post',
  path: '*/api/stacks/web/down',
  permission: 'stacks.manage',
  confirmLabel: 'Down',
  render: ({ onOpenChange, onSuccess }) => (
    <StackDownDialog stack={stack} onOpenChange={onOpenChange} onSuccess={onSuccess} />
  ),
});
