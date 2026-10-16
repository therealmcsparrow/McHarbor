// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { TFunction } from 'i18next';
import { SECRET_KINDS, type SecretKind } from '@core/types/secrets';

export function isBuiltInKind(kind: string): kind is SecretKind {
  return (SECRET_KINDS as string[]).includes(kind);
}

export function kindLabel(t: TFunction<'secrets'>, kind: string): string {
  return isBuiltInKind(kind) ? t(`kind.${kind}`) : kind;
}
