// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const manualTrigger: NodeDefinition = {
  "key": "manual-trigger",
  "label": "Manual Trigger",
  "category": "trigger",
  "description": "Start a workflow manually from the editor or run panel",
  "icon": "IconHandClick",
  "configSchema": [],
  "inputPorts": [],
  "outputPorts": [
    "output"
  ]
};
