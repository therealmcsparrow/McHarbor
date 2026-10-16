// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';
import { predicateConfigFields } from '@modules/workflows/node-predicates';

export const condition: NodeDefinition = {
  key: 'condition',
  label: 'Condition',
  category: 'logic',
  description: 'Route a message based on a condition',
  icon: 'IconGitBranch',
  configSchema: predicateConfigFields(),
  inputPorts: ['input'],
  outputPorts: ['true', 'false'],
};
