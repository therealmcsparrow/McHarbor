// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import { hostPermission, type HostPermissionNeed } from './useHostUpdates';

describe('hostPermission', () => {
  it('gates McHarbor’s own host on host_self.* and any other host on the per-environment keys', () => {
    const expected: Record<HostPermissionNeed, [string, string]> = {
      servicesView: ['host_self.view', 'host_services.view'],
      servicesManage: ['host_self.manage', 'host_services.manage'],
      updatesView: ['host_self.view', 'host_updates.view'],
      updatesManage: ['host_self.manage', 'host_updates.manage'],
      reboot: ['host_self.reboot', 'host_updates.reboot'],
    };
    for (const [need, [self, env]] of Object.entries(expected) as [HostPermissionNeed, [string, string]][]) {
      expect(hostPermission({ kind: 'self' }, need)).toBe(self);
      expect(hostPermission({ kind: 'environment', envId: 'e1' }, need)).toBe(env);
    }
  });
});
