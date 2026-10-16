// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { DebugShellDialog } from './DebugShellDialog';
import { describeDestructiveDialog } from '../../../test/destructive';

describeDestructiveDialog({
  name: 'DebugShellDialog',
  method: 'post',
  path: '*/api/containers/c1/debug-shell',
  permission: 'containers.debug_shell',
  confirmLabel: 'Install and connect',
  render: ({ onOpenChange, onSuccess }) => (
    <DebugShellDialog containerId="c1" open onOpenChange={onOpenChange} onSuccess={onSuccess} />
  ),
});
