// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * Mirrors `modules/secrets/model.go`. Note what is absent: there is no value
 * field on `Secret`, because there is none on the Go type either. A list or
 * detail response structurally cannot carry a credential, and this type says
 * so on the client side too.
 */
export type Secret = {
  id: string;
  name: string;
  description?: string;
  kind: SecretKind;
  group?: string;
  scope: SecretScope;
  environmentId?: string;
  stackName?: string;
  version: number;
  generated: boolean;
  system: boolean;
  originModule?: string;
  lastUsedAt?: string;
  lastRevealedAt?: string;
  createdBy?: string;
  createdByName?: string;
  environmentName?: string;
  createdAt: string;
  updatedAt: string;
  referenceCount: number;
  autoApply: boolean;
  staleCount: number;
};

export type SecretScope = 'global' | 'environment' | 'stack' | 'application';

export type PatchSecretInput = {
  id: string;
  description?: string;
  kind?: SecretKind;
  group?: string;
  autoApply?: boolean;
};

export const SECRET_SCOPES: SecretScope[] = ['global', 'environment', 'stack', 'application'];

export const SECRET_ELEVATED_SCOPES: SecretScope[] = ['global', 'application'];

export type SecretKind = 'env' | 'password' | 'token' | 'cert' | 'private_key' | 'kubeconfig' | 'ssh_key' | 'json';

export const SECRET_KINDS: SecretKind[] = [
  'env',
  'password',
  'token',
  'cert',
  'private_key',
  'kubeconfig',
  'ssh_key',
  'json',
];

export type SecretReference = {
  id: string;
  secretId: string;
  resourceType: string;
  resourceId: string;
  resourceName?: string;
  environmentId?: string;
  field?: string;
  appliedVersion: number;
  createdAt: string;
};

export type SecretImpactResource = {
  resourceType: string;
  resourceId: string;
  resourceName: string;
  environmentId?: string;
  fields: string[];
  appliedVersion: number;
  stale: boolean;
  missing: boolean;
  supersededIds?: string[];
  supported: boolean;
};

export type SecretImpact = {
  secretId: string;
  secretName: string;
  version: number;
  autoApply: boolean;
  resources: SecretImpactResource[];
  staleCount: number;
  missingCount: number;
  prunableCount: number;
};

export type SecretApplyResult = {
  resourceType: string;
  resourceId: string;
  resourceName: string;
  status: SecretApplyStatus;
};

export type SecretApplyStatus = 'applied' | 'failed' | 'unsupported';

/** The response shape of the four endpoints that return a plaintext value. */
export type SecretValueResponse = {
  secret?: Secret;
  value: string;
};

export type CreateSecretInput = {
  name: string;
  description?: string;
  kind: SecretKind;
  group?: string;
  scope: SecretScope;
  environmentId?: string | null;
  stackName?: string | null;
  value?: string;
  generate?: boolean;
  generateLength?: number;
  autoApply?: boolean;
};

export type ApplySecretInput = {
  id: string;
  resources?: { resourceType: string; resourceId: string }[];
};

export type RotateSecretInput = {
  id: string;
  value?: string;
  generate?: boolean;
  generateLength?: number;
};

/** Builds the `${secret:NAME}` reference text for a secret. */
export function secretReference(itemName: string): string {
  return `\${secret:${itemName}}`;
}
