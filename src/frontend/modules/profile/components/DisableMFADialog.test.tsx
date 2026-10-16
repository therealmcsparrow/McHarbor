// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { DisableMFADialog } from './DisableMFADialog';
import { describeDestructiveDialog } from '../../../test/destructive';

describeDestructiveDialog({
  name: 'DisableMFADialog',
  method: 'delete',
  path: '*/api/auth/mfa',
  permission: 'account.self.manage',
  confirmLabel: 'Turn off',
  render: ({ onOpenChange, onSuccess }) => <DisableMFADialog open onOpenChange={onOpenChange} onSuccess={onSuccess} />,
});
