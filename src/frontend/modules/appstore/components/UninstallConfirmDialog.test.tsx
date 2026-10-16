// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { http, HttpResponse } from 'msw';
import { UninstallConfirmDialog } from './UninstallConfirmDialog';
import type { InstalledPlugin } from '../types/plugins';
import { describeDestructiveDialog } from '../../../test/destructive';

const plugin: InstalledPlugin = {
  id: 'p1',
  slug: 'comm-slack',
  version: '2026.09.01',
  enabled: true,
  installed_at: '2026-09-25T00:00:00Z',
  updated_at: '2026-09-25T00:00:00Z',
  loaded: true,
  signed: true,
  permissions_referenced: 0,
  roles_referencing: [],
  built_for: '2026.09.01',
  repair_error: '',
};

describeDestructiveDialog({
  name: 'UninstallConfirmDialog',
  method: 'post',
  path: '*/api/appstore/plugins/uninstall',
  permission: 'plugins.manage',
  confirmLabel: 'Uninstall',
  body: { slug: 'comm-slack', purge_data: false },
  handlers: [
    http.get('*/api/plugins/loader/:kind', () => HttpResponse.json({ success: true, data: [] })),
    http.get('*/api/appstore/plugins/installed', () => HttpResponse.json({ success: true, data: [plugin] })),
  ],
  render: ({ onOpenChange, onSuccess }) => (
    <UninstallConfirmDialog
      plugin={plugin}
      catalogEntry={null}
      open
      onOpenChange={onOpenChange}
      onSuccess={onSuccess}
    />
  ),
});
