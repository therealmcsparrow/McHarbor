// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';
import { predicateConfigFields } from '@modules/workflows/node-predicates';

export const filter: NodeDefinition = {
  key: 'filter',
  label: 'Filter',
  category: 'logic',
  description: 'Allow or block a message using a condition',
  icon: 'IconFilter',
  configSchema: predicateConfigFields(),
  inputPorts: ['input'],
  outputPorts: ['pass', 'block'],
};
