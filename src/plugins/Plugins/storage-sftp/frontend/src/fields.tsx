// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import * as React from 'react';
import {
  IconKey,
  IconLock,
  IconNetwork,
  IconPlugConnected,
  IconRoute,
  IconServer,
  IconShieldLock,
  IconUser,
} from '@tabler/icons-react';

// ─────────────────────────────────────────────────────────────────
// Field descriptors — render order, type, and conditional visibility
// ─────────────────────────────────────────────────────────────────

export type AuthMethod = 'password' | 'publickey' | 'keyboard_interactive' | 'agent';

export type FieldDescriptor = {
  key: string;
  labelKey: string;
  helpKey: string;
  placeholderKey?: string;
  type: 'text' | 'secret' | 'select';
  visible?: (auth: AuthMethod) => boolean;
  options?: Array<{ value: AuthMethod; labelKey: string }>;
};

const AUTH_METHODS: Array<{ value: AuthMethod; labelKey: string }> = [
  { value: 'password', labelKey: 'config.field.auth_method.option.password' },
  { value: 'publickey', labelKey: 'config.field.auth_method.option.publickey' },
  { value: 'keyboard_interactive', labelKey: 'config.field.auth_method.option.keyboard_interactive' },
  { value: 'agent', labelKey: 'config.field.auth_method.option.agent' },
];

// Mirror of backend/main.go ConfigSchema. Order is the visual order.
export const FIELDS: FieldDescriptor[] = [
  {
    key: 'server',
    labelKey: 'config.field.server.label',
    helpKey: 'config.field.server.help',
    placeholderKey: 'config.field.server.placeholder',
    type: 'text',
  },
  {
    key: 'port',
    labelKey: 'config.field.port.label',
    helpKey: 'config.field.port.help',
    placeholderKey: 'config.field.port.placeholder',
    type: 'text',
  },
  {
    key: 'auth_method',
    labelKey: 'config.field.auth_method.label',
    helpKey: 'config.field.auth_method.help',
    type: 'select',
    options: AUTH_METHODS,
  },
  {
    key: 'username',
    labelKey: 'config.field.username.label',
    helpKey: 'config.field.username.help',
    placeholderKey: 'config.field.username.placeholder',
    type: 'text',
  },
  {
    key: 'password',
    labelKey: 'config.field.password.label',
    helpKey: 'config.field.password.help',
    placeholderKey: 'config.field.password.placeholder',
    type: 'secret',
    visible: (a) => a === 'password' || a === 'keyboard_interactive',
  },
  {
    key: 'private_key_path',
    labelKey: 'config.field.private_key_path.label',
    helpKey: 'config.field.private_key_path.help',
    placeholderKey: 'config.field.private_key_path.placeholder',
    type: 'text',
    visible: (a) => a === 'publickey',
  },
  {
    key: 'private_key_passphrase',
    labelKey: 'config.field.private_key_passphrase.label',
    helpKey: 'config.field.private_key_passphrase.help',
    placeholderKey: 'config.field.private_key_passphrase.placeholder',
    type: 'secret',
    visible: (a) => a === 'publickey',
  },
  {
    key: 'agent_socket',
    labelKey: 'config.field.agent_socket.label',
    helpKey: 'config.field.agent_socket.help',
    placeholderKey: 'config.field.agent_socket.placeholder',
    type: 'text',
    visible: (a) => a === 'agent',
  },
  {
    key: 'host_key_fingerprint',
    labelKey: 'config.field.host_key_fingerprint.label',
    helpKey: 'config.field.host_key_fingerprint.help',
    placeholderKey: 'config.field.host_key_fingerprint.placeholder',
    type: 'text',
  },
  {
    key: 'path',
    labelKey: 'config.field.path.label',
    helpKey: 'config.field.path.help',
    placeholderKey: 'config.field.path.placeholder',
    type: 'text',
  },
  {
    key: 'connect_timeout',
    labelKey: 'config.field.connect_timeout.label',
    helpKey: 'config.field.connect_timeout.help',
    placeholderKey: 'config.field.connect_timeout.placeholder',
    type: 'text',
  },
];

// ─────────────────────────────────────────────────────────────────
// Field icons — picked by key, mapped to a Tabler icon
// ─────────────────────────────────────────────────────────────────

export function fieldIcon(key: string): React.ReactNode {
  switch (key) {
    case 'server':
      return <IconServer className="h-4 w-4" />;
    case 'port':
      return <IconNetwork className="h-4 w-4" />;
    case 'auth_method':
      return <IconShieldLock className="h-4 w-4" />;
    case 'username':
      return <IconUser className="h-4 w-4" />;
    case 'password':
      return <IconLock className="h-4 w-4" />;
    case 'private_key_path':
    case 'private_key_passphrase':
      return <IconKey className="h-4 w-4" />;
    case 'agent_socket':
      return <IconPlugConnected className="h-4 w-4" />;
    case 'host_key_fingerprint':
      return <IconShieldLock className="h-4 w-4" />;
    case 'path':
      return <IconRoute className="h-4 w-4" />;
    case 'connect_timeout':
      return <IconNetwork className="h-4 w-4" />;
    default:
      return null;
  }
}
