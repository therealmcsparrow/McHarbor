// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { ContainerActionDialog } from './ContainerActionDialog';
import { describeDestructiveDialog } from '../../../test/destructive';

for (const action of ['stop', 'kill'] as const) {
  describeDestructiveDialog({
    name: `ContainerActionDialog (${action})`,
    method: 'post',
    path: `*/api/containers/c1/${action}`,
    permission: 'containers.manage',
    confirmLabel: action === 'stop' ? 'Stop' : 'Kill',
    render: ({ onOpenChange, onSuccess }) => (
      <ContainerActionDialog containerId="c1" action={action} open onOpenChange={onOpenChange} onSuccess={onSuccess} />
    ),
  });
}
