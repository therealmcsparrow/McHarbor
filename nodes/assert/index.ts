// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';
import { predicateConfigFields } from '@modules/workflows/node-predicates';

export const assert: NodeDefinition = {
  key: 'assert',
  label: 'Assert',
  category: 'logic',
  description: 'Assert that a condition is true and branch on the result',
  icon: 'IconShieldCheckFilled',
  configSchema: [
    ...predicateConfigFields(),
    {
      key: 'message',
      label: 'Failure Message',
      type: 'text',
      required: false,
    },
  ],
  inputPorts: ['input'],
  outputPorts: ['pass', 'fail'],
};
