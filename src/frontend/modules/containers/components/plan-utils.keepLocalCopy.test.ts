// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import type { ContainerBackupPlan } from '@resources/hooks/useContainerBackups';
import { EMPTY_BACKUP_INPUT, planToInput } from './plan-utils';

const plan: ContainerBackupPlan = {
  id: 'p1',
  name: 'web backup',
  environmentId: 'env1',
  containerId: 'web',
  containerName: 'web',
  storageLocationIds: ['loc1'],
  includeConfig: true,
  includeLogs: false,
  includeFilesystem: true,
  includeImage: false,
  selectedMounts: [],
  enabled: true,
  retentionCount: 3,
  retentionDays: 0,
  createdAt: '2026-11-05T00:00:00Z',
  updatedAt: '2026-11-05T00:00:00Z',
};

// Saving an edit sends the whole input back, so a wrong default here would silently delete the
// server copy of every plan an operator merely opened and saved.
describe('keepLocalCopy in the plan form', () => {
  it('keeps the local copy for a new plan', () => {
    expect(EMPTY_BACKUP_INPUT.keepLocalCopy).toBe(true);
  });

  it('keeps the local copy for a plan that never stored the setting', () => {
    expect(planToInput(plan).keepLocalCopy).toBe(true);
  });

  it('carries an explicit off through an edit', () => {
    expect(planToInput({ ...plan, keepLocalCopy: false }).keepLocalCopy).toBe(false);
  });
});
