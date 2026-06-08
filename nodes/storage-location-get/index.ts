// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const storageLocationGet: NodeDefinition = {
  key: 'storage-location-get',
  label: 'Get Storage Location',
  category: 'action',
  description: 'Read one configured storage location without secret values',
  icon: 'IconDatabaseSearch',
  configSchema: [
    { key: 'storage_location_id', label: 'Storage Location', type: 'storage-location-select', required: true },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
