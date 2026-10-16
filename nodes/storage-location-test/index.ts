// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const storageLocationTest: NodeDefinition = {
  key: 'storage-location-test',
  label: 'Storage Location Test',
  category: 'action',
  description: 'Run the connection test for a storage location and optionally read its capacity',
  icon: 'IconPlugConnected',
  configSchema: [
    { key: 'storage_location_id', label: 'Storage Location', type: 'storage-location-select', required: true },
    { key: 'include_usage', label: 'Include Usage', type: 'toggle', required: false, default: false },
    { key: 'fail_on_warn', label: 'Fail On Warning', type: 'toggle', required: false, default: false },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
