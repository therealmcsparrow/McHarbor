// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const sshExec: NodeDefinition = {
  "key": "ssh-exec",
  "label": "SSH Exec",
  "category": "integration",
  "description": "Run a remote command over SSH with password or key authentication",
  "icon": "IconTerminal2",
  "configSchema": [
    {
      "key": "host",
      "label": "Host",
      "type": "text",
      "required": true
    },
    {
      "key": "port",
      "label": "Port",
      "type": "number",
      "required": false,
      "default": 22
    },
    {
      "key": "user",
      "label": "User",
      "type": "text",
      "required": false
    },
    {
      "key": "password",
      "label": "Password",
      "type": "text",
      "secret": true,
      "required": false
    },
    {
      "key": "private_key",
      "label": "Private Key",
      "type": "textarea",
      "secret": true,
      "required": false
    },
    {
      "key": "command",
      "label": "Command",
      "type": "textarea",
      "required": true
    }
  ],
  "inputPorts": [
    "input"
  ],
  "outputPorts": [
    "output",
    "error"
  ]
};
