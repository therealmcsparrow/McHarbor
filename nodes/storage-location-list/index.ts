// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const storageLocationList: NodeDefinition = {
  key: 'storage-location-list',
  label: 'List Storage Locations',
  category: 'action',
  description: 'Return configured storage locations without secret values',
  icon: 'IconDatabaseSearch',
  configSchema: [
    { key: 'enabled_only', label: 'Enabled Only', type: 'toggle', required: false, default: true },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
