// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ComponentType } from 'react';
import {
  IconServer,
  IconLock,
  IconKey,
  IconShieldCheck,
  IconCertificate,
  IconFolder,
  IconClock,
} from '@tabler/icons-react';

export interface FieldDef {
  key: string;
  type: 'text' | 'password' | 'select' | 'boolean';
  required: boolean;
  // Predicate: when defined, only render the field if it
  // returns true. Used to hide username/password for
  // anonymous auth and to swap tls_skip_verify / cert paths
  // depending on the auth method.
  visible?: (cfg: Record<string, string>) => boolean;
  options?: string[];
  icon?: ComponentType<{ className?: string }>;
  labelKey: string;
  placeholderKey?: string;
  helpKey?: string;
}

// List of all auth methods. Match the keys in main.go's
// ftpAuthMethod constant block.
const AUTH_METHODS = ['anonymous', 'password', 'ftps_explicit', 'ftps_implicit', 'client_cert'];

export const FIELDS: FieldDef[] = [
  {
    key: 'server',
    type: 'text',
    required: true,
    icon: IconServer,
    labelKey: 'config.field.server.label',
    placeholderKey: 'config.field.server.placeholder',
    helpKey: 'config.field.server.help',
  },
  {
    key: 'port',
    type: 'text',
    required: false,
    icon: IconServer,
    labelKey: 'config.field.port.label',
    placeholderKey: 'config.field.port.placeholder',
    helpKey: 'config.field.port.help',
  },
  {
    key: 'auth_method',
    type: 'select',
    required: true,
    options: AUTH_METHODS,
    icon: IconLock,
    labelKey: 'config.field.auth_method.label',
    helpKey: 'config.field.auth_method.help',
  },
  {
    key: 'username',
    type: 'text',
    required: false,
    // Anonymous uses built-in USER/PASS; client_cert may
    // need USER (server-dependent) but usually doesn't.
    visible: (cfg) => cfg.auth_method !== 'anonymous' && cfg.auth_method !== undefined && cfg.auth_method !== '',
    icon: IconKey,
    labelKey: 'config.field.username.label',
    placeholderKey: 'config.field.username.placeholder',
    helpKey: 'config.field.username.help',
  },
  {
    key: 'password',
    type: 'password',
    required: false,
    visible: (cfg) =>
      cfg.auth_method === 'password' || cfg.auth_method === 'ftps_explicit' || cfg.auth_method === 'ftps_implicit',
    icon: IconKey,
    labelKey: 'config.field.password.label',
    placeholderKey: 'config.field.password.placeholder',
    helpKey: 'config.field.password.help',
  },
  {
    key: 'tls_skip_verify',
    type: 'boolean',
    required: false,
    visible: (cfg) =>
      cfg.auth_method === 'ftps_explicit' || cfg.auth_method === 'ftps_implicit' || cfg.auth_method === 'client_cert',
    icon: IconShieldCheck,
    labelKey: 'config.field.tls_skip_verify.label',
    helpKey: 'config.field.tls_skip_verify.help',
  },
  {
    key: 'client_cert_path',
    type: 'text',
    required: false,
    visible: (cfg) => cfg.auth_method === 'client_cert',
    icon: IconCertificate,
    labelKey: 'config.field.client_cert_path.label',
    placeholderKey: 'config.field.client_cert_path.placeholder',
    helpKey: 'config.field.client_cert_path.help',
  },
  {
    key: 'client_key_path',
    type: 'text',
    required: false,
    visible: (cfg) => cfg.auth_method === 'client_cert',
    icon: IconCertificate,
    labelKey: 'config.field.client_key_path.label',
    placeholderKey: 'config.field.client_key_path.placeholder',
    helpKey: 'config.field.client_key_path.help',
  },
  {
    key: 'path',
    type: 'text',
    required: false,
    icon: IconFolder,
    labelKey: 'config.field.path.label',
    placeholderKey: 'config.field.path.placeholder',
    helpKey: 'config.field.path.help',
  },
  {
    key: 'connect_timeout',
    type: 'text',
    required: false,
    icon: IconClock,
    labelKey: 'config.field.connect_timeout.label',
    placeholderKey: 'config.field.connect_timeout.placeholder',
    helpKey: 'config.field.connect_timeout.help',
  },
];
