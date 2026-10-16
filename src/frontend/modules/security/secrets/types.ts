// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type {
  ApplySecretInput,
  CreateSecretInput,
  RotateSecretInput,
  Secret,
  SecretApplyResult,
  SecretApplyStatus,
  SecretImpact,
  SecretImpactResource,
  SecretKind,
  SecretReference,
  SecretScope,
  SecretValueResponse,
} from '@core/types/secrets';
export { SECRET_ELEVATED_SCOPES, SECRET_KINDS, SECRET_SCOPES, secretReference } from '@core/types/secrets';

export { SECRET_NAME_PATTERN } from '@resources/utils/secret-name';

/** The reveal reason floor, mirrored from `revealReasonMinLength`. */
export const REVEAL_REASON_MIN_LENGTH = 10;

/** How long a revealed value stays on screen before it hides itself. */
export const REVEAL_AUTO_HIDE_SECONDS = 30;
